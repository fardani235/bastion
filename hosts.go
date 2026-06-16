package main

import (
	"fmt"

	"bastion/internal/store"
	"bastion/internal/vault"
)

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("bastion: port %d out of range (1-65535)", port)
	}
	return nil
}

func validateHostname(h string) error {
	if h == "" {
		return fmt.Errorf("bastion: hostname must not be empty")
	}
	for i := 0; i < len(h); i++ {
		if h[i] <= ' ' || h[i] == 0x7f {
			return fmt.Errorf("bastion: hostname contains control characters")
		}
	}
	return nil
}

// HostDTO is the frontend-facing view of a host. It deliberately carries NO
// plaintext credential — only enough for the UI to render correct form state.
// has_password lets the edit form show "password set" without ever shipping
// the secret to the renderer.
type HostDTO struct {
	ID          string  `json:"id"`
	GroupID     *string `json:"groupId"`
	Label       string  `json:"label"`
	Hostname    string  `json:"hostname"`
	Port        int     `json:"port"`
	Username    string  `json:"username"`
	AuthKind    string  `json:"authKind"`
	KeyPath     string  `json:"keyPath"`
	HasPassword bool    `json:"hasPassword"`
	HasKeyPass  bool    `json:"hasKeyPassphrase"`
	SortOrder   int     `json:"sortOrder"`
	FontSize    *int    `json:"fontSize,omitempty"`
}

// HostInput is the writable surface the frontend sends. Unlike the DTO it DOES
// carry plaintext secrets (password, key passphrase) — they travel renderer ->
// Go only, are encrypted immediately, and are never sent back.
type HostInput struct {
	GroupID       *string `json:"groupId"`
	Label         string  `json:"label"`
	Hostname      string  `json:"hostname"`
	Port          int     `json:"port"`
	Username      string  `json:"username"`
	AuthKind      string  `json:"authKind"`
	Password      string  `json:"password"`
	KeyPath       string  `json:"keyPath"`
	KeyPassphrase string  `json:"keyPassphrase"`
	FontSize      *int    `json:"fontSize,omitempty"`
}

func toHostDTO(h store.Host) HostDTO {
	return HostDTO{
		ID:          h.ID,
		GroupID:     h.GroupID,
		Label:       h.Label,
		Hostname:    h.Hostname,
		Port:        h.Port,
		Username:    h.Username,
		AuthKind:    h.AuthKind,
		KeyPath:     h.KeyPath,
		HasPassword: len(h.PasswordCiphertext) > 0,
		HasKeyPass:  len(h.KeyPassphraseCiphertext) > 0,
		SortOrder:   h.SortOrder,
		FontSize:    h.FontSize,
	}
}

// ListHosts returns all hosts as credential-free DTOs.
func (a *App) ListHosts() ([]HostDTO, error) {
	if !a.IsUnlocked() {
		return nil, errLocked
	}
	rows, err := a.store.ListHosts()
	if err != nil {
		return nil, err
	}
	out := make([]HostDTO, 0, len(rows))
	for _, h := range rows {
		out = append(out, toHostDTO(h))
	}
	return out, nil
}

// CreateHost encrypts any supplied secrets under the vault key and inserts the
// host, returning a credential-free DTO.
func (a *App) CreateHost(in HostInput) (HostDTO, error) {
	if err := validateHostname(in.Hostname); err != nil {
		return HostDTO{}, err
	}
	if err := validatePort(in.Port); err != nil {
		return HostDTO{}, err
	}
	key, err := a.keyCopy()
	if err != nil {
		return HostDTO{}, err
	}
	defer zero(key)

	si, err := a.toStoreInput(key, in, nil)
	if err != nil {
		return HostDTO{}, err
	}
	h, err := a.store.CreateHost(si)
	if err != nil {
		return HostDTO{}, err
	}
	return toHostDTO(h), nil
}

// UpdateHost re-encrypts changed secrets and overwrites the host. A blank
// Password/KeyPassphrase means "keep the existing ciphertext" — the UI never
// holds the secret, so it cannot resend an unchanged one.
func (a *App) UpdateHost(id string, in HostInput) (HostDTO, error) {
	if err := validateHostname(in.Hostname); err != nil {
		return HostDTO{}, err
	}
	if err := validatePort(in.Port); err != nil {
		return HostDTO{}, err
	}
	key, err := a.keyCopy()
	if err != nil {
		return HostDTO{}, err
	}
	defer zero(key)

	existing, err := a.store.GetHost(id)
	if err != nil {
		return HostDTO{}, err
	}
	si, err := a.toStoreInput(key, in, &existing)
	if err != nil {
		return HostDTO{}, err
	}
	h, err := a.store.UpdateHost(id, si)
	if err != nil {
		return HostDTO{}, err
	}
	return toHostDTO(h), nil
}

// DeleteHost removes a host.
func (a *App) DeleteHost(id string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	return a.store.DeleteHost(id)
}

// SetHostFontSize updates the terminal font size for a host. Pass nil to
// reset to the default.
func (a *App) SetHostFontSize(hostID string, fontSize *int) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	return a.store.SetHostFontSize(hostID, fontSize)
}

// toStoreInput converts a frontend HostInput into a store.HostInput, encrypting
// secrets under key. When prev is non-nil (update), blank secrets inherit the
// previous ciphertext; for key-auth hosts the password ciphertext is dropped.
func (a *App) toStoreInput(key []byte, in HostInput, prev *store.Host) (store.HostInput, error) {
	si := store.HostInput{
		GroupID:  in.GroupID,
		Label:    in.Label,
		Hostname: in.Hostname,
		Port:     in.Port,
		Username: in.Username,
		AuthKind: in.AuthKind,
		KeyPath:  in.KeyPath,
		FontSize: in.FontSize,
	}

	switch in.AuthKind {
	case "password":
		ct, err := a.encryptOrInherit(key, in.Password, prevPassword(prev))
		if err != nil {
			return store.HostInput{}, err
		}
		si.PasswordCiphertext = ct
	case "key":
		// Key auth may still carry an encrypted-key passphrase.
		ct, err := a.encryptOrInherit(key, in.KeyPassphrase, prevPassphrase(prev))
		if err != nil {
			return store.HostInput{}, err
		}
		si.KeyPassphraseCiphertext = ct
	default:
		return store.HostInput{}, fmt.Errorf("bastion: unknown auth kind %q", in.AuthKind)
	}
	return si, nil
}

// encryptOrInherit encrypts plaintext under key, or returns prev unchanged when
// plaintext is blank (the "leave as-is" path). A blank plaintext with no prev
// yields nil (no credential).
func (a *App) encryptOrInherit(key []byte, plaintext string, prev []byte) ([]byte, error) {
	if plaintext == "" {
		return prev, nil
	}
	return vault.Encrypt(key, []byte(plaintext))
}

func prevPassword(prev *store.Host) []byte {
	if prev == nil {
		return nil
	}
	return prev.PasswordCiphertext
}

func prevPassphrase(prev *store.Host) []byte {
	if prev == nil {
		return nil
	}
	return prev.KeyPassphraseCiphertext
}

// decryptHostPassword recovers a host's plaintext password for opening a
// session. Callers MUST zero the returned slice when done.
func (a *App) decryptHostPassword(id string) ([]byte, error) {
	key, err := a.keyCopy()
	if err != nil {
		return nil, err
	}
	defer zero(key)

	h, err := a.store.GetHost(id)
	if err != nil {
		return nil, err
	}
	if len(h.PasswordCiphertext) == 0 {
		return nil, fmt.Errorf("bastion: host %q has no stored password", id)
	}
	pt, err := vault.Decrypt(key, h.PasswordCiphertext)
	if err != nil {
		return nil, fmt.Errorf("bastion: decrypt password: %w", err)
	}
	return pt, nil
}

// decryptHostPassphrase recovers a host's plaintext key passphrase. Callers
// MUST zero the returned slice when done. Returns nil, nil if no passphrase.
func (a *App) decryptHostPassphrase(id string) ([]byte, error) {
	key, err := a.keyCopy()
	if err != nil {
		return nil, err
	}
	defer zero(key)

	h, err := a.store.GetHost(id)
	if err != nil {
		return nil, err
	}
	if len(h.KeyPassphraseCiphertext) == 0 {
		return nil, nil
	}
	pt, err := vault.Decrypt(key, h.KeyPassphraseCiphertext)
	if err != nil {
		return nil, fmt.Errorf("bastion: decrypt passphrase: %w", err)
	}
	return pt, nil
}
