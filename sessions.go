package main

import (
	"errors"
	"fmt"
	"os"

	appssh "bastion/internal/ssh"

	gossh "golang.org/x/crypto/ssh"
)

// UnknownHostKeyInfo is the trust-prompt payload sent to the frontend when a
// server's key is not yet in known_hosts.
type UnknownHostKeyInfo struct {
	Hostname          string `json:"hostname"`
	Port              int    `json:"port"`
	KeyType           string `json:"keyType"`
	FingerprintSHA256 string `json:"fingerprintSHA256"`
	Base64Key         string `json:"base64Key"`
}

// OpenSessionResult is the union returned by OpenSession: exactly one of
// SessionID (success) or UnknownHostKey (the UI must show a trust prompt and
// retry after TrustHostKey).
type OpenSessionResult struct {
	SessionID      string              `json:"sessionId,omitempty"`
	UnknownHostKey *UnknownHostKeyInfo `json:"unknownHostKey,omitempty"`
}

// OpenSession resolves the host's credentials, dials, and starts a PTY session.
// An unknown host key is reported as a structured prompt (not an error); a
// changed key is a hard error.
func (a *App) OpenSession(hostID string, cols, rows int) (OpenSessionResult, error) {
	defer a.touchAutoLock()
	if !a.IsUnlocked() {
		return OpenSessionResult{}, errLocked
	}

	h, err := a.store.GetHost(hostID)
	if err != nil {
		return OpenSessionResult{}, err
	}

	auth, err := a.authMethod(h.ID, h.AuthKind, h.KeyPath)
	if err != nil {
		return OpenSessionResult{}, err
	}

	kh, err := appssh.NewKnownHosts(a.knownHostsPath)
	if err != nil {
		return OpenSessionResult{}, err
	}

	// Load enabled port forwards for auto-start.
	pfRows, err := a.store.ListPortForwards(hostID)
	if err != nil {
		return OpenSessionResult{}, fmt.Errorf("bastion: list port forwards: %w", err)
	}
	var rules []appssh.ForwardRule
	for _, pf := range pfRows {
		if pf.Enabled {
			rules = append(rules, appssh.ForwardRule{
				ID:         pf.ID,
				LocalPort:  pf.LocalPort,
				RemoteHost: pf.RemoteHost,
				RemotePort: pf.RemotePort,
			})
		}
	}

	cfg := appssh.DialConfig{
		Hostname:     h.Hostname,
		Port:         h.Port,
		Username:     h.Username,
		Auth:         auth,
		HostKey:      kh.Callback(),
		ForwardRules: rules,
	}

	id, err := a.sessions.Open(cfg, cols, rows)
	if err != nil {
		var unknown *appssh.UnknownHostKeyError
		if errors.As(err, &unknown) {
			return OpenSessionResult{UnknownHostKey: &UnknownHostKeyInfo{
				Hostname:          unknown.Hostname,
				Port:              unknown.Port,
				KeyType:           unknown.KeyType,
				FingerprintSHA256: unknown.FingerprintSHA256,
				Base64Key:         unknown.Base64Key,
			}}, nil
		}
		// Host-key mismatch and all other dial errors propagate as errors.
		return OpenSessionResult{}, err
	}
	// Start session logging only when the user has explicitly enabled it. Logs
	// are plaintext and may capture secrets typed at the terminal, so this is
	// off by default (see SessionLoggingEnabled).
	if a.emitter != nil && a.SessionLoggingEnabled() {
		label := h.Label
		if label == "" {
			label = h.Hostname
		}
		_ = a.emitter.OpenLog(id, label)
	}
	return OpenSessionResult{SessionID: id}, nil
}

// SessionLoggingEnabled reports whether plaintext session logging is turned on.
// It is OFF unless the user has explicitly enabled it: session logs are written
// in the clear and can capture passwords and secrets typed at the terminal.
func (a *App) SessionLoggingEnabled() bool {
	raw, ok, err := a.store.GetMeta(metaSessionLog)
	if err != nil || !ok {
		return false
	}
	return string(raw) == "1"
}

// SetSessionLogging turns plaintext session logging on or off. The setting only
// affects sessions opened after the change; existing logs are untouched. The
// caller must be unlocked.
func (a *App) SetSessionLogging(enabled bool) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	val := "0"
	if enabled {
		val = "1"
	}
	return a.store.SetMeta(metaSessionLog, []byte(val))
}

// authMethod builds the SSH auth methods for a host, decrypting stored secrets.
func (a *App) authMethod(hostID, authKind, keyPath string) ([]gossh.AuthMethod, error) {
	switch authKind {
	case "password":
		pw, err := a.decryptHostPassword(hostID)
		if err != nil {
			return nil, err
		}
		defer zero(pw)
		return []gossh.AuthMethod{gossh.Password(string(pw))}, nil

	case "key":
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("bastion: read key %q: %w", keyPath, err)
		}
		signer, err := a.parseKey(hostID, keyBytes)
		if err != nil {
			return nil, err
		}
		return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil

	default:
		return nil, fmt.Errorf("bastion: unknown auth kind %q", authKind)
	}
}

// parseKey parses a private key, decrypting it with the stored passphrase when
// the key is encrypted.
func (a *App) parseKey(hostID string, keyBytes []byte) (gossh.Signer, error) {
	signer, err := gossh.ParsePrivateKey(keyBytes)
	if err == nil {
		return signer, nil
	}
	var passErr *gossh.PassphraseMissingError
	if !errors.As(err, &passErr) {
		return nil, fmt.Errorf("bastion: parse key: %w", err)
	}

	passphrase, perr := a.decryptHostPassphrase(hostID)
	if perr != nil {
		return nil, perr
	}
	if passphrase == nil {
		return nil, errors.New("bastion: key is encrypted but no passphrase is stored")
	}
	defer zero(passphrase)
	signer, err = gossh.ParsePrivateKeyWithPassphrase(keyBytes, passphrase)
	if err != nil {
		return nil, fmt.Errorf("bastion: parse encrypted key: %w", err)
	}
	return signer, nil
}

// TrustHostKey records a server key the user accepted at the trust prompt.
func (a *App) TrustHostKey(hostname string, port int, keyType, base64Key string) error {
	defer a.touchAutoLock()
	kh, err := appssh.NewKnownHosts(a.knownHostsPath)
	if err != nil {
		return err
	}
	return kh.TrustEncoded(hostname, port, keyType, base64Key)
}

// WriteToSession forwards keystrokes / pasted text to a session's stdin. It is
// rejected while the vault is locked: a locked vault must not accept input to
// any terminal, even one that has not yet finished tearing down.
func (a *App) WriteToSession(sessionID, data string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	defer a.touchAutoLock()
	return a.sessions.Write(sessionID, []byte(data))
}

// ResizeSession informs the remote PTY of a new terminal size.
func (a *App) ResizeSession(sessionID string, cols, rows int) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	defer a.touchAutoLock()
	return a.sessions.Resize(sessionID, cols, rows)
}

// CloseSession terminates a session at the user's request.
func (a *App) CloseSession(sessionID string) error {
	defer a.touchAutoLock()
	return a.sessions.Close(sessionID)
}
