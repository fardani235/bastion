package main

import (
	"path/filepath"
	"testing"

	"bastion/internal/store"
)

// newTestApp builds an App backed by a throwaway on-disk store. It mirrors what
// startup() does (open a store) without needing the Wails runtime. The known
// hosts path is also under the temp dir so session tests never touch the real
// config directory.
func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "vault.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &App{store: s, knownHostsPath: filepath.Join(dir, "known_hosts")}
}

func TestAuth_FirstRunLifecycle(t *testing.T) {
	a := newTestApp(t)

	if !a.IsFirstRun() {
		t.Fatal("a fresh vault must report IsFirstRun() == true")
	}
	if a.IsUnlocked() {
		t.Fatal("a fresh vault must not be unlocked")
	}

	if err := a.Setup("correct horse battery staple"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !a.IsUnlocked() {
		t.Fatal("Setup must leave the vault unlocked")
	}
	if a.IsFirstRun() {
		t.Fatal("after Setup, IsFirstRun() must be false")
	}
}

func TestAuth_SetupRejectedWhenAlreadySetUp(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("password1"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := a.Setup("password2"); err == nil {
		t.Fatal("Setup must fail when a vault already exists")
	}
}

func TestAuth_SetupRejectsShortPassword(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("short"); err == nil {
		t.Fatal("Setup must reject a password shorter than the minimum")
	}
	if !a.IsFirstRun() {
		t.Fatal("a rejected Setup must leave the vault un-initialized")
	}
	if a.IsUnlocked() {
		t.Fatal("a rejected Setup must not unlock the vault")
	}
}

func TestAuth_UnlockWithCorrectPassword(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("hunter2x"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// Simulate a fresh launch: lock, then unlock.
	if err := a.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if a.IsUnlocked() {
		t.Fatal("Lock must clear the unlocked state")
	}
	if err := a.Unlock("hunter2x"); err != nil {
		t.Fatalf("Unlock with correct password: %v", err)
	}
	if !a.IsUnlocked() {
		t.Fatal("Unlock must set the unlocked state")
	}
}

func TestAuth_UnlockWithWrongPasswordFails(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("hunter2x"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	_ = a.Lock()

	if err := a.Unlock("wrong"); err == nil {
		t.Fatal("Unlock with a wrong password must fail")
	}
	if a.IsUnlocked() {
		t.Fatal("a failed Unlock must leave the vault locked")
	}
}

func TestAuth_UnlockBeforeSetupFails(t *testing.T) {
	a := newTestApp(t)
	if err := a.Unlock("anything"); err == nil {
		t.Fatal("Unlock before Setup must fail")
	}
}

func TestAuth_UnlockClampsTamperedKDFParams(t *testing.T) {
	// A vault is set up with default params, then the on-disk KDF memory is
	// tampered to an absurd value. Unlock must clamp it back to the default and
	// still succeed — a tampered DB cannot lock out a legitimate password nor
	// force a weak/oversized KDF.
	a := newTestApp(t)
	if err := a.Setup("a-good-password"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	_ = a.Lock()

	// Downgrade attempt (memory = 8 KiB, far below the floor) and an inflate
	// attempt (time well above the ceiling).
	if err := a.store.SetMeta(metaKDFMemory, []byte("8")); err != nil {
		t.Fatalf("SetMeta memory: %v", err)
	}
	if err := a.store.SetMeta(metaKDFTime, []byte("100000")); err != nil {
		t.Fatalf("SetMeta time: %v", err)
	}

	if err := a.Unlock("a-good-password"); err != nil {
		t.Fatalf("Unlock with tampered KDF params must still succeed (clamped): %v", err)
	}
	if !a.IsUnlocked() {
		t.Fatal("vault must be unlocked after clamped Unlock")
	}
}

func TestAuth_UnlockSurvivesReopen(t *testing.T) {
	// Persisted salt/verify_blob must allow unlocking a brand-new App against
	// the same DB file — the real "quit and relaunch" path.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "vault.db")

	s1, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	a1 := &App{store: s1, knownHostsPath: filepath.Join(dir, "known_hosts")}
	if err := a1.Setup("s3cret99"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	_ = s1.Close()

	s2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open #2: %v", err)
	}
	defer s2.Close()
	a2 := &App{store: s2, knownHostsPath: filepath.Join(dir, "known_hosts")}

	if a2.IsFirstRun() {
		t.Fatal("a re-opened vault must not be first-run")
	}
	if err := a2.Unlock("s3cret99"); err != nil {
		t.Fatalf("Unlock after reopen: %v", err)
	}
}
