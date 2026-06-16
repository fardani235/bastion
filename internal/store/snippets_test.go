package store

import "testing"

func TestSnippet_CreateAndList(t *testing.T) {
	s := openMemory(t)
	sn, err := s.CreateSnippet("Restart nginx", "sudo systemctl restart nginx")
	if err != nil {
		t.Fatalf("CreateSnippet: %v", err)
	}
	if sn.ID == "" {
		t.Fatal("expected ID")
	}

	list, err := s.ListSnippets()
	if err != nil {
		t.Fatalf("ListSnippets: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(list))
	}
}

func TestSnippet_Update(t *testing.T) {
	s := openMemory(t)
	sn, _ := s.CreateSnippet("typo", "tial -f /var/log/app.log")
	if err := s.UpdateSnippet(sn.ID, "Tail app log", "tail -f /var/log/app.log"); err != nil {
		t.Fatalf("UpdateSnippet: %v", err)
	}
	list, _ := s.ListSnippets()
	if list[0].Label != "Tail app log" || list[0].Body != "tail -f /var/log/app.log" {
		t.Fatalf("snippet not updated: %+v", list[0])
	}
}

func TestSnippet_UpdateMissingFails(t *testing.T) {
	s := openMemory(t)
	if err := s.UpdateSnippet("no-such-id", "Label", "body"); err == nil {
		t.Fatal("UpdateSnippet on a missing id must return an error")
	}
}

func TestSnippet_Delete(t *testing.T) {
	s := openMemory(t)
	sn, _ := s.CreateSnippet("rm me", "echo gone")
	if err := s.DeleteSnippet(sn.ID); err != nil {
		t.Fatalf("DeleteSnippet: %v", err)
	}
	list, _ := s.ListSnippets()
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}
