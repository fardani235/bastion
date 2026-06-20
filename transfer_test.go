package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDestDir(t *testing.T) {
	cases := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{"normal absolute path", "/home/alice/uploads", false},
		{"home shorthand", "~", false},
		{"relative path", "uploads", false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"embedded newline", "/home/alice\n/etc", true},
		{"embedded NUL", "/home/\x00alice", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDestDir(tc.dir)
			if tc.wantErr && err == nil {
				t.Fatalf("validateDestDir(%q) = nil, want error", tc.dir)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateDestDir(%q) = %v, want nil", tc.dir, err)
			}
		})
	}
}

func TestUploadFiles_LockedVault(t *testing.T) {
	a := newTestApp(t)
	// Not unlocked.
	if _, err := a.UploadFiles("sess", "/tmp", []string{"/tmp/x"}); err == nil {
		t.Fatal("UploadFiles on a locked vault must return an error")
	}
}

func TestPrepareUpload_ExpandsDirectories(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("correct horse battery staple"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	a.sessions = nil // PrepareUpload only needs ResolveUploadDir, which tolerates nil.

	dir := t.TempDir()
	file := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	innerFile := filepath.Join(subdir, "inner.txt")
	if err := os.WriteFile(innerFile, []byte("inner"), 0o644); err != nil {
		t.Fatalf("write inner: %v", err)
	}
	missing := filepath.Join(dir, "nope.txt")

	// Two valid paths (file + dir) + one missing path (gracefully skipped).
	res, err := a.PrepareUpload("sess-without-client", []string{file, subdir, missing})
	if err != nil {
		t.Fatalf("PrepareUpload: %v", err)
	}
	if res.DestDir != "~" {
		t.Fatalf("DestDir fallback: got %q want ~", res.DestDir)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("expected 2 candidates (file + expanded dir), got %d", len(res.Candidates))
	}

	byName := map[string]UploadCandidate{}
	for _, c := range res.Candidates {
		byName[c.Name] = c
	}
	if c := byName["real.txt"]; !c.Upload || c.Size != 2 {
		t.Fatalf("real.txt should be uploadable size 2, got %+v", c)
	}
	if c := byName["subdir/inner.txt"]; !c.Upload {
		t.Fatalf("subdir/inner.txt should be uploadable, got %+v", c)
	}
	if c := byName["subdir"]; c.Upload {
		t.Fatalf("subdir itself should not appear as a candidate (only its contents), got %+v", c)
	}
	// Missing path is silently dropped — the preview shows what's available.
}

func TestResolveUploadDir_NilSessions(t *testing.T) {
	a := newTestApp(t)
	a.sessions = nil
	if got := a.ResolveUploadDir("anything"); got != "~" {
		t.Fatalf("ResolveUploadDir with nil sessions = %q, want ~", got)
	}
}
