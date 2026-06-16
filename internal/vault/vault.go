// Package vault implements the Bastion vault: KDF, AES-GCM encryption, and a
// verification blob used to confirm the master password without decrypting any
// stored credential.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

// KeyLen is the length in bytes of a derived vault key (AES-256).
const KeyLen = 32

// DeriveParams holds Argon2id parameters used for key derivation.
type DeriveParams struct {
	Time    uint32 // number of passes
	Memory  uint32 // KiB of memory
	Threads uint8  // parallelism
}

// DefaultParams is the production Argon2id parameter set.
var DefaultParams = DeriveParams{
	Time:    3,
	Memory:  64 * 1024, // 64 MiB
	Threads: 4,
}

// KDF parameter bounds enforced when reading parameters back from storage. The
// on-disk vault_meta is local but sensitive: bounding the parameters before
// deriving stops a tampered DB from forcing a weak KDF (downgrade) or an
// unbounded one (memory-exhaustion DoS at unlock time). DefaultParams sits
// inside these bounds, so every legitimately-created vault is unaffected.
const (
	MinTime    = 1
	MaxTime    = 30
	MinMemory  = 16 * 1024   // 16 MiB
	MaxMemory  = 2048 * 1024 // 2 GiB
	MinThreads = 1
	MaxThreads = 64
)

// Derive returns a KeyLen-byte key from the master password using Argon2id.
func Derive(password string, salt []byte, p DeriveParams) []byte {
	return argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, KeyLen)
}

// ClampParam returns v if it lies within [min, max]; otherwise it returns def.
// Used when reading individual KDF parameters from untrusted storage: an
// out-of-range stored value is treated as absent and falls back to the default
// rather than being rejected, so a legitimate vault never fails to unlock.
func ClampParam(v, min, max, def uint32) uint32 {
	if v < min || v > max {
		return def
	}
	return v
}

// DefaultIters is the legacy PBKDF2 iteration count, retained for backward
// compat when unlocking vaults created with schema version 1.
const DefaultIters = 600_000

// NonceLen is the length in bytes of the GCM nonce.
const NonceLen = 12

// ErrCiphertextTooShort is returned when Decrypt receives a blob shorter than
// the nonce length.
var ErrCiphertextTooShort = errors.New("vault: ciphertext too short")

// Encrypt seals plaintext under key using AES-256-GCM with a fresh random
// nonce. The returned blob is: nonce(12) || ciphertext || tag(16).
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Seal appends ciphertext+tag onto its first argument; we prefix the nonce.
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens a blob produced by Encrypt. Returns an error if the key is
// wrong, the blob has been tampered with, or the blob is malformed.
func Decrypt(key, blob []byte) ([]byte, error) {
	if len(blob) < NonceLen {
		return nil, ErrCiphertextTooShort
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, ct := blob[:NonceLen], blob[NonceLen:]
	return gcm.Open(nil, nonce, ct, nil)
}

// VerifyString is the fixed plaintext we encrypt at setup time and decrypt at
// unlock time. Its value is part of the on-disk format; never change it.
const VerifyString = "bastion-vault-v1"

// ErrWrongMasterPassword is returned by CheckVerifyBlob when the supplied key
// does not unseal the verification blob.
var ErrWrongMasterPassword = errors.New("vault: wrong master password")

// NewVerifyBlob encrypts the fixed VerifyString under key. The result is
// stored in the vault at setup time.
func NewVerifyBlob(key []byte) ([]byte, error) {
	return Encrypt(key, []byte(VerifyString))
}

// CheckVerifyBlob attempts to decrypt blob under key and verifies the plaintext
// matches VerifyString. Returns ErrWrongMasterPassword on any failure.
func CheckVerifyBlob(key, blob []byte) error {
	pt, err := Decrypt(key, blob)
	if err != nil {
		return ErrWrongMasterPassword
	}
	if string(pt) != VerifyString {
		return ErrWrongMasterPassword
	}
	return nil
}

// SaltLen is the length in bytes of a vault salt.
const SaltLen = 16

// NewSalt returns SaltLen bytes from crypto/rand.
func NewSalt() ([]byte, error) {
	s := make([]byte, SaltLen)
	if _, err := io.ReadFull(rand.Reader, s); err != nil {
		return nil, err
	}
	return s, nil
}
