package store

import (
	"bytes"
	"testing"
)

// openMemory opens an in-memory SQLite for tests. Each call returns an
// isolated DB (different `cache` key).
func openMemory(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpen_CreatesAllTables(t *testing.T) {
	s := openMemory(t)

	want := []string{"vault_meta", "groups", "hosts", "snippets", "port_forwards"}
	for _, name := range want {
		var found string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
		).Scan(&found)
		if err != nil {
			t.Errorf("missing table %q: %v", name, err)
		}
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	s := openMemory(t)

	// Run migrations a second time on the same connection — should be a no-op.
	if err := s.migrate(); err != nil {
		t.Fatalf("second migrate must succeed: %v", err)
	}
}

func TestVaultMeta_SetAndGet(t *testing.T) {
	s := openMemory(t)

	if err := s.SetMeta("salt", []byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	got, ok, err := s.GetMeta("salt")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if !ok {
		t.Fatal("expected meta key 'salt' to exist")
	}
	if !bytes.Equal(got, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("got %x want 010203", got)
	}
}

func TestVaultMeta_Overwrite(t *testing.T) {
	s := openMemory(t)
	_ = s.SetMeta("salt", []byte{0xaa})
	_ = s.SetMeta("salt", []byte{0xbb})

	got, _, _ := s.GetMeta("salt")
	if !bytes.Equal(got, []byte{0xbb}) {
		t.Fatalf("expected overwrite, got %x", got)
	}
}

func TestVaultMeta_GetMissing(t *testing.T) {
	s := openMemory(t)

	_, ok, err := s.GetMeta("missing")
	if err != nil {
		t.Fatalf("GetMeta on missing key must not error: %v", err)
	}
	if ok {
		t.Fatal("missing key must report ok=false")
	}
}
