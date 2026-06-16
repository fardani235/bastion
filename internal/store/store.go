// Package store is the SQLite persistence layer for Bastion. It owns the
// schema, the migrations, and CRUD for hosts/groups/snippets. It has no
// dependency on the vault package — encrypted blobs are stored as opaque
// BLOBs and decrypted by the caller.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// schema revisions. Bump this when adding a migration.
const schemaRevision = 2

// Store is a thin wrapper around a *sql.DB.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite DB at path (use ":memory:" for tests) and runs all
// pending migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// SQLite is fastest with WAL + NORMAL synchronous. For :memory: this is a no-op.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: pragmas: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying DB connection.
func (s *Store) Close() error { return s.db.Close() }

// migrate creates tables if they do not exist. It is idempotent — running it
// twice is a no-op.
func (s *Store) migrate() error {
	if err := s.migrateSchema(); err != nil {
		return err
	}
	// Read current revision and apply incremental migrations.
	rev := s.readRevision()
	if rev < 2 {
		if err := s.migrateV2(); err != nil {
			return err
		}
	}
	return nil
}

// migrateSchema creates tables (idempotent). These are the initial schema v1.
func (s *Store) migrateSchema() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS vault_meta (
  key   TEXT PRIMARY KEY,
  value BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS groups (
  id         TEXT PRIMARY KEY,
  name       TEXT UNIQUE NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS hosts (
  id                         TEXT PRIMARY KEY,
  group_id                   TEXT REFERENCES groups(id) ON DELETE SET NULL,
  label                      TEXT NOT NULL,
  hostname                   TEXT NOT NULL,
  port                       INTEGER NOT NULL DEFAULT 22,
  username                   TEXT NOT NULL,
  auth_kind                  TEXT NOT NULL CHECK(auth_kind IN ('password','key')),
  password_ciphertext        BLOB,
  key_path                   TEXT,
  key_passphrase_ciphertext  BLOB,
  sort_order                 INTEGER NOT NULL DEFAULT 0,
  created_at                 INTEGER NOT NULL,
  updated_at                 INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS snippets (
  id         TEXT PRIMARY KEY,
  label      TEXT NOT NULL,
  body       TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS port_forwards (
  id          TEXT PRIMARY KEY,
  host_id     TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  label       TEXT NOT NULL DEFAULT '',
  local_port  INTEGER NOT NULL,
  remote_host TEXT NOT NULL DEFAULT 'localhost',
  remote_port INTEGER NOT NULL,
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL
);
`
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// migrateV2 adds the font_size column to hosts.
func (s *Store) migrateV2() error {
	// SQLite does not support IF NOT EXISTS for ALTER TABLE, so we check
	// whether the column already exists via PRAGMA.
	rows, err := s.db.Query(`PRAGMA table_info(hosts)`)
	if err != nil {
		return fmt.Errorf("store: migration v2: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("store: migration v2 scan: %w", err)
		}
		if name == "font_size" {
			return nil // already migrated
		}
	}
	if _, err := s.db.Exec(`ALTER TABLE hosts ADD COLUMN font_size INTEGER`); err != nil {
		return fmt.Errorf("store: migration v2: alter: %w", err)
	}
	return s.setRevision(2)
}

// readRevision reads the schema revision from vault_meta, defaulting to 1.
func (s *Store) readRevision() int {
	raw, ok, err := s.GetMeta("schema_revision")
	if err != nil || !ok {
		return 1
	}
	rev := 1
	fmt.Sscanf(string(raw), "%d", &rev)
	return rev
}

// setRevision persists the schema revision to vault_meta.
func (s *Store) setRevision(rev int) error {
	return s.SetMeta("schema_revision", []byte(fmt.Sprintf("%d", rev)))
}

// SetMeta inserts or replaces a vault_meta row.
func (s *Store) SetMeta(key string, value []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO vault_meta(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("store: set meta %q: %w", key, err)
	}
	return nil
}

// GetMeta returns the value for the given vault_meta key. The second return
// is false if the key does not exist (without producing an error).
func (s *Store) GetMeta(key string) ([]byte, bool, error) {
	var v []byte
	err := s.db.QueryRow(`SELECT value FROM vault_meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: get meta %q: %w", key, err)
	}
	return v, true, nil
}
