package store

import (
	"testing"
)

func TestGroup_CreateAndList(t *testing.T) {
	s := openMemory(t)

	g1, err := s.CreateGroup("Production")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if g1.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if g1.Name != "Production" {
		t.Fatalf("name: got %q want %q", g1.Name, "Production")
	}

	_, _ = s.CreateGroup("Staging")

	groups, err := s.ListGroups()
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

func TestGroup_CreateRejectsDuplicateName(t *testing.T) {
	s := openMemory(t)
	_, _ = s.CreateGroup("Production")
	if _, err := s.CreateGroup("Production"); err == nil {
		t.Fatal("expected error on duplicate group name")
	}
}

func TestGroup_Rename(t *testing.T) {
	s := openMemory(t)
	g, _ := s.CreateGroup("Stagng") // typo
	if err := s.RenameGroup(g.ID, "Staging"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	groups, _ := s.ListGroups()
	if groups[0].Name != "Staging" {
		t.Fatalf("rename did not stick: %q", groups[0].Name)
	}
}

func TestGroup_RenameMissingFails(t *testing.T) {
	s := openMemory(t)
	if err := s.RenameGroup("no-such-id", "Whatever"); err == nil {
		t.Fatal("RenameGroup on a missing id must return an error")
	}
}

func TestGroup_RenameToDuplicateFails(t *testing.T) {
	s := openMemory(t)
	_, _ = s.CreateGroup("Production")
	staging, _ := s.CreateGroup("Staging")
	// Renaming Staging -> Production collides with the UNIQUE(name) constraint.
	if err := s.RenameGroup(staging.ID, "Production"); err == nil {
		t.Fatal("RenameGroup to an existing name must fail (UNIQUE constraint)")
	}
}

func TestGroup_Delete(t *testing.T) {
	s := openMemory(t)
	g, _ := s.CreateGroup("Temp")
	if err := s.DeleteGroup(g.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	groups, _ := s.ListGroups()
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(groups))
	}
}
