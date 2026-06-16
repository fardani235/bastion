package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Host is one saved SSH host. Credential ciphertexts are opaque to this layer.
type Host struct {
	ID                      string  `json:"id"`
	GroupID                 *string `json:"groupId"`
	Label                   string  `json:"label"`
	Hostname                string  `json:"hostname"`
	Port                    int     `json:"port"`
	Username                string  `json:"username"`
	AuthKind                string  `json:"authKind"` // "password" | "key"
	PasswordCiphertext      []byte  `json:"-"`
	KeyPath                 string  `json:"keyPath"`
	KeyPassphraseCiphertext []byte  `json:"-"`
	SortOrder               int     `json:"sortOrder"`
	FontSize                *int    `json:"fontSize,omitempty"`
	CreatedAt               int64   `json:"createdAt"`
	UpdatedAt               int64   `json:"updatedAt"`
}

// HostInput is the writable surface — what callers supply on Create/Update.
// The store does not validate semantic correctness (e.g., that password_ciphertext
// is non-nil when auth_kind == "password"); that is the responsibility of the
// app-layer code.
type HostInput struct {
	GroupID                 *string
	Label                   string
	Hostname                string
	Port                    int
	Username                string
	AuthKind                string
	PasswordCiphertext      []byte
	KeyPath                 string
	KeyPassphraseCiphertext []byte
	FontSize                *int
}

// CreateHost inserts a new host with a fresh UUID.
func (s *Store) CreateHost(in HostInput) (Host, error) {
	now := time.Now().Unix()
	h := Host{
		ID:                      uuid.NewString(),
		GroupID:                 in.GroupID,
		Label:                   in.Label,
		Hostname:                in.Hostname,
		Port:                    in.Port,
		Username:                in.Username,
		AuthKind:                in.AuthKind,
		PasswordCiphertext:      in.PasswordCiphertext,
		KeyPath:                 in.KeyPath,
		KeyPassphraseCiphertext: in.KeyPassphraseCiphertext,
		FontSize:                in.FontSize,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	_, err := s.db.Exec(
		`INSERT INTO hosts(
			id, group_id, label, hostname, port, username, auth_kind,
			password_ciphertext, key_path, key_passphrase_ciphertext,
			sort_order, font_size, created_at, updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.ID, nullableString(h.GroupID), h.Label, h.Hostname, h.Port, h.Username, h.AuthKind,
		nullableBytes(h.PasswordCiphertext), nullableString(strPtr(h.KeyPath)),
		nullableBytes(h.KeyPassphraseCiphertext),
		h.SortOrder, h.FontSize, h.CreatedAt, h.UpdatedAt,
	)
	if err != nil {
		return Host{}, fmt.Errorf("store: create host: %w", err)
	}
	return h, nil
}

// ListHosts returns all hosts, ordered by sort_order then label.
func (s *Store) ListHosts() ([]Host, error) {
	rows, err := s.db.Query(`
		SELECT id, group_id, label, hostname, port, username, auth_kind,
		       password_ciphertext, key_path, key_passphrase_ciphertext,
		       sort_order, font_size, created_at, updated_at
		FROM hosts
		ORDER BY sort_order, label
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list hosts: %w", err)
	}
	defer rows.Close()

	var out []Host
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// GetHost returns one host by id. Returns an error if not found.
func (s *Store) GetHost(id string) (Host, error) {
	row := s.db.QueryRow(`
		SELECT id, group_id, label, hostname, port, username, auth_kind,
		       password_ciphertext, key_path, key_passphrase_ciphertext,
		       sort_order, font_size, created_at, updated_at
		FROM hosts WHERE id = ?`, id)
	h, err := scanHost(row)
	if err != nil {
		return Host{}, fmt.Errorf("store: get host %q: %w", id, err)
	}
	return h, nil
}

// UpdateHost overwrites all writable fields. Returns the updated host.
func (s *Store) UpdateHost(id string, in HostInput) (Host, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(`
		UPDATE hosts SET
			group_id = ?, label = ?, hostname = ?, port = ?, username = ?,
			auth_kind = ?, password_ciphertext = ?, key_path = ?,
			key_passphrase_ciphertext = ?, font_size = ?, updated_at = ?
		WHERE id = ?`,
		nullableString(in.GroupID), in.Label, in.Hostname, in.Port, in.Username,
		in.AuthKind, nullableBytes(in.PasswordCiphertext),
		nullableString(strPtr(in.KeyPath)),
		nullableBytes(in.KeyPassphraseCiphertext), in.FontSize, now, id,
	)
	if err != nil {
		return Host{}, fmt.Errorf("store: update host: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Host{}, fmt.Errorf("store: update host: id %q not found", id)
	}
	return s.GetHost(id)
}

// DeleteHost removes a host.
func (s *Store) DeleteHost(id string) error {
	_, err := s.db.Exec(`DELETE FROM hosts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete host: %w", err)
	}
	return nil
}

// SetHostFontSize updates only the font_size column for a host.
func (s *Store) SetHostFontSize(id string, fontSize *int) error {
	res, err := s.db.Exec(`UPDATE hosts SET font_size = ? WHERE id = ?`, fontSize, id)
	if err != nil {
		return fmt.Errorf("store: set font size: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: set font size: id %q not found", id)
	}
	return nil
}

// --- scanner & null helpers ---

// scanner is a tiny interface satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanHost(sc scanner) (Host, error) {
	var (
		h           Host
		groupID     sql.NullString
		password    []byte
		keyPath     sql.NullString
		keyPassword []byte
		fontSize    sql.NullInt64
	)
	if err := sc.Scan(
		&h.ID, &groupID, &h.Label, &h.Hostname, &h.Port, &h.Username, &h.AuthKind,
		&password, &keyPath, &keyPassword,
		&h.SortOrder, &fontSize, &h.CreatedAt, &h.UpdatedAt,
	); err != nil {
		return Host{}, err
	}
	if groupID.Valid {
		h.GroupID = &groupID.String
	}
	h.PasswordCiphertext = password
	if keyPath.Valid {
		h.KeyPath = keyPath.String
	}
	h.KeyPassphraseCiphertext = keyPassword
	if fontSize.Valid {
		v := int(fontSize.Int64)
		h.FontSize = &v
	}
	return h, nil
}

func nullableString(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
