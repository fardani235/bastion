package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// testAddr is a minimal net.Addr for feeding the HostKeyCallback.
type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

// testKey generates a throwaway ed25519 host key for tests.
func testKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	return signer.PublicKey()
}

func TestKnownHosts_UnknownHostReturnsStructuredError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	kh, err := NewKnownHosts(path)
	if err != nil {
		t.Fatalf("NewKnownHosts: %v", err)
	}

	cb := kh.Callback()
	key := testKey(t)

	err = cb("example.com:22", testAddr("203.0.113.7:22"), key)
	if err == nil {
		t.Fatal("expected an error for an unknown host")
	}
	var unknown *UnknownHostKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected *UnknownHostKeyError, got %T: %v", err, err)
	}
	if unknown.Hostname != "example.com" || unknown.Port != 22 {
		t.Fatalf("host/port not parsed: %+v", unknown)
	}
	if unknown.KeyType != key.Type() {
		t.Fatalf("key type: got %q want %q", unknown.KeyType, key.Type())
	}
	if unknown.FingerprintSHA256 != gossh.FingerprintSHA256(key) {
		t.Fatalf("fingerprint mismatch")
	}
	if unknown.Base64Key == "" {
		t.Fatal("expected a base64-encoded key for later Trust()")
	}
}

func TestKnownHosts_TrustThenAccept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	kh, _ := NewKnownHosts(path)
	key := testKey(t)

	// Trust the key as the UI would after the user accepts the prompt.
	if err := kh.Trust("example.com", 22, key); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	// A fresh callback (reloads the file) must now accept the same key.
	cb := kh.Callback()
	if err := cb("example.com:22", testAddr("203.0.113.7:22"), key); err != nil {
		t.Fatalf("expected trusted host to be accepted, got: %v", err)
	}
}

func TestKnownHosts_MismatchReturnsMismatchError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	kh, _ := NewKnownHosts(path)
	good := testKey(t)
	if err := kh.Trust("example.com", 22, good); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	// A different key for the same host must be a hard mismatch, not "unknown".
	evil := testKey(t)
	cb := kh.Callback()
	err := cb("example.com:22", testAddr("203.0.113.7:22"), evil)
	if err == nil {
		t.Fatal("expected a mismatch error")
	}
	var mismatch *HostKeyMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected *HostKeyMismatchError, got %T: %v", err, err)
	}
	var unknown *UnknownHostKeyError
	if errors.As(err, &unknown) {
		t.Fatal("a mismatch must NOT be reported as an unknown host")
	}
}

func TestNewKnownHosts_UncreatablePathFails(t *testing.T) {
	// Make a regular file, then ask for a known_hosts *under* it. MkdirAll
	// cannot create a directory beneath a file, so construction must fail
	// rather than silently proceeding with an unusable store.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "iam-a-file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := NewKnownHosts(filepath.Join(blocker, "known_hosts")); err == nil {
		t.Fatal("NewKnownHosts must fail when the parent path is not a directory")
	}
}

func TestKnownHosts_TrustEncodedRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	kh, _ := NewKnownHosts(path)
	key := testKey(t)

	// Simulate what the IPC layer passes: key type + standard-base64 wire key.
	b64 := base64.StdEncoding.EncodeToString(key.Marshal())
	if err := kh.TrustEncoded("example.com", 22, key.Type(), b64); err != nil {
		t.Fatalf("TrustEncoded: %v", err)
	}

	cb := kh.Callback()
	if err := cb("example.com:22", testAddr("203.0.113.7:22"), key); err != nil {
		t.Fatalf("expected encoded-trusted host to be accepted, got: %v", err)
	}
}

func TestKnownHosts_TrustEncodedRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	kh, _ := NewKnownHosts(path)

	if err := kh.TrustEncoded("example.com", 22, "ssh-ed25519", "not-valid-base64!!"); err == nil {
		t.Fatal("TrustEncoded must reject undecodable base64")
	}

	// Valid base64 but not a parseable public key.
	bad := base64.StdEncoding.EncodeToString([]byte("nonsense"))
	if err := kh.TrustEncoded("example.com", 22, "ssh-ed25519", bad); err == nil {
		t.Fatal("TrustEncoded must reject a non-public-key blob")
	}
}

func TestKnownHosts_TrustEncodedRejectsTypeMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	kh, _ := NewKnownHosts(path)
	key := testKey(t)
	b64 := base64.StdEncoding.EncodeToString(key.Marshal())

	// Claimed type disagrees with the actual key type.
	if err := kh.TrustEncoded("example.com", 22, "ssh-rsa", b64); err == nil {
		t.Fatal("TrustEncoded must reject a key whose type contradicts the claim")
	}
}

func TestKnownHosts_NonStandardPortIsScoped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	kh, _ := NewKnownHosts(path)
	key := testKey(t)

	// Trust on port 2222 only.
	if err := kh.Trust("example.com", 2222, key); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	cb := kh.Callback()
	// Same host on port 22 must still be unknown (port is part of identity).
	err := cb("example.com:22", testAddr("203.0.113.7:22"), key)
	var unknown *UnknownHostKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("port 22 should be unknown when only 2222 is trusted, got %T: %v", err, err)
	}
}
