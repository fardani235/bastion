package store

import (
	"testing"
)

func TestPortForward_CreateAndList(t *testing.T) {
	s := openMemory(t)

	host := seedHost(t, s)

	in := PortForwardInput{
		HostID:     host.ID,
		Label:      "web app",
		LocalPort:  8080,
		RemoteHost: "10.0.0.42",
		RemotePort: 80,
		Enabled:    true,
	}
	pf, err := s.CreatePortForward(in)
	if err != nil {
		t.Fatalf("CreatePortForward: %v", err)
	}
	if pf.ID == "" {
		t.Fatal("expected an ID")
	}
	if pf.LocalPort != 8080 {
		t.Fatalf("local_port: got %d", pf.LocalPort)
	}
	if !pf.Enabled {
		t.Fatal("expected enabled")
	}

	list, err := s.ListPortForwards(host.ID)
	if err != nil {
		t.Fatalf("ListPortForwards: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 forward, got %d", len(list))
	}
	if list[0].ID != pf.ID {
		t.Fatalf("id mismatch: %s vs %s", list[0].ID, pf.ID)
	}
}

func TestPortForward_Update(t *testing.T) {
	s := openMemory(t)
	host := seedHost(t, s)

	pf, _ := s.CreatePortForward(PortForwardInput{
		HostID: host.ID, Label: "old", LocalPort: 3000,
		RemoteHost: "a", RemotePort: 3000, Enabled: true,
	})

	updated, err := s.UpdatePortForward(pf.ID, PortForwardInput{
		HostID: host.ID, Label: "renamed", LocalPort: 4000,
		RemoteHost: "b", RemotePort: 4000, Enabled: false,
	})
	if err != nil {
		t.Fatalf("UpdatePortForward: %v", err)
	}
	if updated.Label != "renamed" || updated.LocalPort != 4000 || updated.RemoteHost != "b" {
		t.Fatalf("update not applied: %+v", updated)
	}
	if updated.Enabled {
		t.Fatal("expected disabled after update")
	}
}

func TestPortForward_Delete(t *testing.T) {
	s := openMemory(t)
	host := seedHost(t, s)

	pf, _ := s.CreatePortForward(PortForwardInput{
		HostID: host.ID, Label: "del", LocalPort: 5000,
		RemoteHost: "x", RemotePort: 5000, Enabled: true,
	})

	if err := s.DeletePortForward(pf.ID); err != nil {
		t.Fatalf("DeletePortForward: %v", err)
	}

	list, _ := s.ListPortForwards(host.ID)
	if len(list) != 0 {
		t.Fatal("expected 0 forwards after delete")
	}
}

func TestPortForward_HostDeleteCascades(t *testing.T) {
	s := openMemory(t)
	host := seedHost(t, s)

	_, _ = s.CreatePortForward(PortForwardInput{
		HostID: host.ID, Label: "will cascade", LocalPort: 6000,
		RemoteHost: "y", RemotePort: 6000, Enabled: true,
	})

	if err := s.DeleteHost(host.ID); err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}

	list, _ := s.ListPortForwards(host.ID)
	if len(list) != 0 {
		t.Fatal("expected port forwards to cascade on host delete")
	}
}

func TestPortForward_GetByID(t *testing.T) {
	s := openMemory(t)
	host := seedHost(t, s)

	pf, _ := s.CreatePortForward(PortForwardInput{
		HostID: host.ID, Label: "gettest", LocalPort: 7000,
		RemoteHost: "z", RemotePort: 7000, Enabled: true,
	})

	got, err := s.GetPortForward(pf.ID)
	if err != nil {
		t.Fatalf("GetPortForward: %v", err)
	}
	if got.Label != "gettest" {
		t.Fatalf("label: got %q", got.Label)
	}
}

// seedHost creates a host in s and returns it.
func seedHost(t *testing.T, s *Store) Host {
	t.Helper()
	h, err := s.CreateHost(HostInput{
		Label: "pf-test-host", Hostname: "10.0.0.1",
		Port: 22, Username: "u", AuthKind: "password",
	})
	if err != nil {
		t.Fatalf("seedHost: %v", err)
	}
	return h
}
