package store

import (
	"bytes"
	"testing"
)

func sampleHostInput() HostInput {
	return HostInput{
		Label:                   "bastion-01",
		Hostname:                "10.0.0.1",
		Port:                    22,
		Username:                "deploy",
		AuthKind:                "password",
		PasswordCiphertext:      []byte{0xaa, 0xbb, 0xcc},
		KeyPath:                 "",
		KeyPassphraseCiphertext: nil,
		GroupID:                 nil,
	}
}

func TestHost_CreateAndList(t *testing.T) {
	s := openMemory(t)

	h, err := s.CreateHost(sampleHostInput())
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}
	if h.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if h.AuthKind != "password" {
		t.Fatalf("auth_kind: got %q", h.AuthKind)
	}
	if !bytes.Equal(h.PasswordCiphertext, []byte{0xaa, 0xbb, 0xcc}) {
		t.Fatalf("password_ciphertext round-trip mismatch")
	}

	hosts, err := s.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
}

func TestHost_Update(t *testing.T) {
	s := openMemory(t)
	h, _ := s.CreateHost(sampleHostInput())

	in := sampleHostInput()
	in.Label = "bastion-renamed"
	in.AuthKind = "key"
	in.PasswordCiphertext = nil
	in.KeyPath = "/home/deploy/.ssh/id_ed25519"

	updated, err := s.UpdateHost(h.ID, in)
	if err != nil {
		t.Fatalf("UpdateHost: %v", err)
	}
	if updated.Label != "bastion-renamed" {
		t.Fatalf("label not updated: %q", updated.Label)
	}
	if updated.AuthKind != "key" || updated.KeyPath == "" {
		t.Fatal("auth fields not updated")
	}
	if updated.PasswordCiphertext != nil {
		t.Fatal("password_ciphertext should be NULL after switching to key auth")
	}
}

func TestHost_Delete(t *testing.T) {
	s := openMemory(t)
	h, _ := s.CreateHost(sampleHostInput())

	if err := s.DeleteHost(h.ID); err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}
	hosts, _ := s.ListHosts()
	if len(hosts) != 0 {
		t.Fatalf("expected 0 hosts, got %d", len(hosts))
	}
}

func TestHost_CreateRejectsInvalidAuthKind(t *testing.T) {
	s := openMemory(t)
	in := sampleHostInput()
	in.AuthKind = "totp" // violates CHECK(auth_kind IN ('password','key'))
	if _, err := s.CreateHost(in); err == nil {
		t.Fatal("CreateHost must reject an auth_kind outside the allowed set")
	}
}

func TestHost_UpdateMissingFails(t *testing.T) {
	s := openMemory(t)
	if _, err := s.UpdateHost("no-such-id", sampleHostInput()); err == nil {
		t.Fatal("UpdateHost on a missing id must return an error")
	}
}

func TestHost_GetByID(t *testing.T) {
	s := openMemory(t)
	h, _ := s.CreateHost(sampleHostInput())

	got, err := s.GetHost(h.ID)
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if got.Label != "bastion-01" {
		t.Fatalf("label: got %q", got.Label)
	}

	if _, err := s.GetHost("does-not-exist"); err == nil {
		t.Fatal("GetHost on missing id must return an error")
	}
}

func TestHost_DeleteGroupSetsHostGroupToNull(t *testing.T) {
	s := openMemory(t)
	g, _ := s.CreateGroup("Production")

	in := sampleHostInput()
	in.GroupID = &g.ID
	h, _ := s.CreateHost(in)

	// Sanity: host belongs to the group.
	got, _ := s.GetHost(h.ID)
	if got.GroupID == nil || *got.GroupID != g.ID {
		t.Fatalf("expected group %q, got %v", g.ID, got.GroupID)
	}

	// Delete the group; the host should remain with group_id = NULL.
	if err := s.DeleteGroup(g.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	got, err := s.GetHost(h.ID)
	if err != nil {
		t.Fatalf("GetHost after DeleteGroup: %v", err)
	}
	if got.GroupID != nil {
		t.Fatalf("expected group_id NULL after group delete, got %q", *got.GroupID)
	}
}
