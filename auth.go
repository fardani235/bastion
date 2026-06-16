package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"

	"bastion/internal/vault"
	"golang.org/x/crypto/pbkdf2"
)

// vault_meta keys.
const (
	metaSalt          = "salt"
	metaVerifyBlob    = "verify_blob"
	metaKDFIters      = "kdf_iters"
	metaKDFTime       = "kdf_time"
	metaKDFMemory     = "kdf_memory"
	metaKDFThreads    = "kdf_threads"
	metaSchemaVersion = "schema_version"
	metaSessionLog    = "session_logging" // "1" = enabled; absent/"0" = disabled
)

const schemaVersion = 2

// minMasterPasswordLen is the minimum length enforced when setting up a vault.
// The whole vault's security reduces to this password (there is no recovery),
// so we reject trivially short ones at the trust boundary — the frontend check
// is convenience only and can be bypassed via direct IPC. Following NIST
// SP 800-63B we favor length over composition rules and impose no other cost.
const minMasterPasswordLen = 8

// errLocked is returned by IPC methods that require an unlocked vault.
var errLocked = errors.New("bastion: vault is locked")

// IsFirstRun reports whether the vault has not yet been set up (no salt stored).
func (a *App) IsFirstRun() bool {
	_, ok, err := a.store.GetMeta(metaSalt)
	if err != nil {
		// Treat a read error conservatively as "not first run" so we don't
		// offer to overwrite a vault we simply failed to read.
		return false
	}
	return !ok
}

// IsUnlocked reports whether a derived key is currently held in memory.
func (a *App) IsUnlocked() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.key != nil
}

// Setup initializes a brand-new vault: it generates a salt, derives a key from
// password, stores the verification blob and KDF parameters, and leaves the
// vault unlocked. It fails if a vault already exists.
func (a *App) Setup(password string) error {
	if !a.IsFirstRun() {
		return errors.New("bastion: vault already set up")
	}
	if len(password) < minMasterPasswordLen {
		return fmt.Errorf("bastion: master password must be at least %d characters", minMasterPasswordLen)
	}

	salt, err := vault.NewSalt()
	if err != nil {
		return fmt.Errorf("bastion: setup: %w", err)
	}
	key := vault.Derive(password, salt, vault.DefaultParams)

	blob, err := vault.NewVerifyBlob(key)
	if err != nil {
		return fmt.Errorf("bastion: setup: %w", err)
	}

	if err := a.store.SetMeta(metaSalt, salt); err != nil {
		return err
	}
	if err := a.store.SetMeta(metaVerifyBlob, blob); err != nil {
		return err
	}
	if err := a.store.SetMeta(metaKDFTime, []byte(strconv.Itoa(int(vault.DefaultParams.Time)))); err != nil {
		return err
	}
	if err := a.store.SetMeta(metaKDFMemory, []byte(strconv.Itoa(int(vault.DefaultParams.Memory)))); err != nil {
		return err
	}
	if err := a.store.SetMeta(metaKDFThreads, []byte(strconv.Itoa(int(vault.DefaultParams.Threads)))); err != nil {
		return err
	}
	if err := a.store.SetMeta(metaSchemaVersion, []byte(strconv.Itoa(schemaVersion))); err != nil {
		return err
	}

	a.mu.Lock()
	a.key = key
	a.mu.Unlock()
	return nil
}

// Unlock derives a key from password and verifies it against the stored
// verification blob. On success the key is held in memory; on failure the
// vault stays locked and no credential is ever decrypted with a wrong key.
func (a *App) Unlock(password string) error {
	defer a.touchAutoLock()
	salt, ok, err := a.store.GetMeta(metaSalt)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("bastion: vault not set up")
	}
	blob, ok, err := a.store.GetMeta(metaVerifyBlob)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("bastion: vault missing verification blob")
	}

	ver := 1
	if raw, ok, _ := a.store.GetMeta(metaSchemaVersion); ok {
		if v, convErr := strconv.Atoi(string(raw)); convErr == nil {
			ver = v
		}
	}

	var key []byte
	if ver >= 2 {
		// Read KDF parameters from storage, but bound each one: a tampered DB
		// must not be able to downgrade the KDF to a weak value or inflate it
		// into an unlock-time memory-exhaustion DoS. Out-of-range values fall
		// back to the default for that parameter (see vault.ClampParam).
		params := vault.DefaultParams
		if raw, ok, _ := a.store.GetMeta(metaKDFTime); ok {
			if n, convErr := strconv.Atoi(string(raw)); convErr == nil && n > 0 {
				params.Time = vault.ClampParam(uint32(n), vault.MinTime, vault.MaxTime, vault.DefaultParams.Time)
			}
		}
		if raw, ok, _ := a.store.GetMeta(metaKDFMemory); ok {
			if n, convErr := strconv.Atoi(string(raw)); convErr == nil && n > 0 {
				params.Memory = vault.ClampParam(uint32(n), vault.MinMemory, vault.MaxMemory, vault.DefaultParams.Memory)
			}
		}
		if raw, ok, _ := a.store.GetMeta(metaKDFThreads); ok {
			if n, convErr := strconv.Atoi(string(raw)); convErr == nil && n > 0 {
				params.Threads = uint8(vault.ClampParam(uint32(n), vault.MinThreads, vault.MaxThreads, uint32(vault.DefaultParams.Threads)))
			}
		}
		key = vault.Derive(password, salt, params)
	} else {
		// Legacy PBKDF2 vault (schema version 1).
		iters := vault.DefaultIters
		if raw, ok, _ := a.store.GetMeta(metaKDFIters); ok {
			if n, convErr := strconv.Atoi(string(raw)); convErr == nil && n > 0 {
				iters = n
			}
		}
		key = pbkdf2.Key([]byte(password), salt, iters, vault.KeyLen, sha256.New)
	}

	if err := vault.CheckVerifyBlob(key, blob); err != nil {
		zero(key)
		return err // vault.ErrWrongMasterPassword
	}

	a.mu.Lock()
	a.key = key
	a.mu.Unlock()
	return nil
}

// Lock zeroes and drops the in-memory key and tears down every live SSH
// session, returning the app to the locked state. Sessions are closed because
// a locked vault must not leave fully-writable terminals open behind it.
func (a *App) Lock() error {
	a.mu.Lock()
	zero(a.key)
	a.key = nil
	if a.autoLockTimer != nil {
		a.autoLockTimer.Stop()
	}
	a.mu.Unlock()

	// Close sessions outside the lock: teardown does network I/O and emits
	// events, neither of which should run while holding a.mu.
	if a.sessions != nil {
		a.sessions.CloseAll()
	}
	return nil
}

// keyCopy returns a copy of the in-memory key, or an error if locked. Callers
// that need the key for encrypt/decrypt take a copy under the lock so the key
// can't be zeroed mid-operation by a concurrent Lock().
func (a *App) keyCopy() ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.key == nil {
		return nil, errLocked
	}
	k := make([]byte, len(a.key))
	copy(k, a.key)
	return k, nil
}
