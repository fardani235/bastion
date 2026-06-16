package main

import (
	"testing"
)

func unlockedApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t)
	if err := a.Setup("master-pw"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return a
}

func samplePwHost() HostInput {
	return HostInput{
		Label:    "web-01",
		Hostname: "10.0.0.5",
		Port:     22,
		Username: "deploy",
		AuthKind: "password",
		Password: "s3cret-pw",
	}
}

func TestHostsIPC_CreateHidesCredentials(t *testing.T) {
	a := unlockedApp(t)

	dto, err := a.CreateHost(samplePwHost())
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if dto.ID == "" {
		t.Fatal("expected an ID")
	}
	if !dto.HasPassword {
		t.Fatal("a password host must report HasPassword == true")
	}
	if dto.AuthKind != "password" {
		t.Fatalf("auth_kind: got %q", dto.AuthKind)
	}

	// The on-disk row must hold ciphertext, never the plaintext.
	row, err := a.store.GetHost(dto.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if len(row.PasswordCiphertext) == 0 {
		t.Fatal("password must be stored as ciphertext")
	}
	if string(row.PasswordCiphertext) == "s3cret-pw" {
		t.Fatal("password must NOT be stored in plaintext")
	}
}

func TestHostsIPC_RoundTripDecrypts(t *testing.T) {
	a := unlockedApp(t)
	dto, err := a.CreateHost(samplePwHost())
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	// The app must be able to recover the plaintext for opening a session.
	pw, err := a.decryptHostPassword(dto.ID)
	if err != nil {
		t.Fatalf("decryptHostPassword: %v", err)
	}
	defer zero(pw)
	if string(pw) != "s3cret-pw" {
		t.Fatalf("decrypted password: got %q want %q", string(pw), "s3cret-pw")
	}
}

func TestHostsIPC_ListReturnsDTOsWithoutSecrets(t *testing.T) {
	a := unlockedApp(t)
	if _, err := a.CreateHost(samplePwHost()); err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	list, err := a.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 host, got %d", len(list))
	}
	if !list[0].HasPassword {
		t.Fatal("expected HasPassword true")
	}
}

func TestHostsIPC_UpdateChangePasswordReEncrypts(t *testing.T) {
	a := unlockedApp(t)
	dto, _ := a.CreateHost(samplePwHost())

	in := samplePwHost()
	in.Label = "web-01-renamed"
	in.Password = "new-password"
	updated, err := a.UpdateHost(dto.ID, in)
	if err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}
	if updated.Label != "web-01-renamed" {
		t.Fatalf("label: got %q", updated.Label)
	}

	pw, err := a.decryptHostPassword(dto.ID)
	if err != nil {
		t.Fatalf("decryptHostPassword: %v", err)
	}
	defer zero(pw)
	if string(pw) != "new-password" {
		t.Fatalf("password not re-encrypted: got %q", string(pw))
	}
}

func TestHostsIPC_UpdateKeepsPasswordWhenBlank(t *testing.T) {
	// A blank Password on update means "leave the stored credential as-is" —
	// the UI doesn't round-trip the secret, so it can't resend it.
	a := unlockedApp(t)
	dto, _ := a.CreateHost(samplePwHost())

	in := samplePwHost()
	in.Label = "renamed-only"
	in.Password = "" // unchanged
	if _, err := a.UpdateHost(dto.ID, in); err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}

	pw, err := a.decryptHostPassword(dto.ID)
	if err != nil {
		t.Fatalf("decryptHostPassword: %v", err)
	}
	defer zero(pw)
	if string(pw) != "s3cret-pw" {
		t.Fatalf("blank update must preserve the old password, got %q", string(pw))
	}
}

func TestHostsIPC_KeyAuthHostHasNoPassword(t *testing.T) {
	a := unlockedApp(t)
	in := HostInput{
		Label:    "key-host",
		Hostname: "10.0.0.9",
		Port:     22,
		Username: "deploy",
		AuthKind: "key",
		KeyPath:  "/home/deploy/.ssh/id_ed25519",
	}
	dto, err := a.CreateHost(in)
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if dto.HasPassword {
		t.Fatal("a key-auth host must report HasPassword == false")
	}
	if dto.KeyPath != "/home/deploy/.ssh/id_ed25519" {
		t.Fatalf("key_path: got %q", dto.KeyPath)
	}
}

func TestHostsIPC_RequireUnlocked(t *testing.T) {
	a := newTestApp(t) // not set up / locked
	if _, err := a.CreateHost(samplePwHost()); err == nil {
		t.Fatal("CreateHost must fail when locked")
	}
}

func TestHostsIPC_DeleteRemoves(t *testing.T) {
	a := unlockedApp(t)
	dto, _ := a.CreateHost(samplePwHost())
	if err := a.DeleteHost(dto.ID); err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}
	list, _ := a.ListHosts()
	if len(list) != 0 {
		t.Fatalf("expected 0 hosts, got %d", len(list))
	}
}

func TestGroupsIPC_CRUD(t *testing.T) {
	a := unlockedApp(t)
	g, err := a.CreateGroup("Production")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := a.RenameGroup(g.ID, "Prod"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	groups, err := a.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Prod" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	if err := a.DeleteGroup(g.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
}

func TestSnippetsIPC_CRUD(t *testing.T) {
	a := unlockedApp(t)
	sn, err := a.CreateSnippet("Restart nginx", "sudo systemctl restart nginx")
	if err != nil {
		t.Fatalf("CreateSnippet: %v", err)
	}
	if err := a.UpdateSnippet(sn.ID, "Restart", "systemctl restart nginx"); err != nil {
		t.Fatalf("UpdateSnippet: %v", err)
	}
	list, err := a.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets: %v", err)
	}
	if len(list) != 1 || list[0].Label != "Restart" {
		t.Fatalf("unexpected snippets: %+v", list)
	}
	if err := a.DeleteSnippet(sn.ID); err != nil {
		t.Fatalf("DeleteSnippet: %v", err)
	}
}
