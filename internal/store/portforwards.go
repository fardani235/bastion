package store

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PortForward is one saved port forwarding rule attached to a host.
type PortForward struct {
	ID         string `json:"id"`
	HostID     string `json:"hostId"`
	Label      string `json:"label"`
	LocalPort  int    `json:"localPort"`
	RemoteHost string `json:"remoteHost"`
	RemotePort int    `json:"remotePort"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"createdAt"`
}

// PortForwardInput is the writable surface for create/update.
type PortForwardInput struct {
	HostID     string
	Label      string
	LocalPort  int
	RemoteHost string
	RemotePort int
	Enabled    bool
}

// ListPortForwards returns all port forwards for a given host.
func (s *Store) ListPortForwards(hostID string) ([]PortForward, error) {
	rows, err := s.db.Query(`
		SELECT id, host_id, label, local_port, remote_host, remote_port, enabled, created_at
		FROM port_forwards WHERE host_id = ?
		ORDER BY local_port`, hostID)
	if err != nil {
		return nil, fmt.Errorf("store: list port_forwards: %w", err)
	}
	defer rows.Close()

	var out []PortForward
	for rows.Next() {
		var (
			pf      PortForward
			enabled int
		)
		if err := rows.Scan(&pf.ID, &pf.HostID, &pf.Label, &pf.LocalPort, &pf.RemoteHost, &pf.RemotePort, &enabled, &pf.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan port_forward: %w", err)
		}
		pf.Enabled = enabled != 0
		out = append(out, pf)
	}
	return out, rows.Err()
}

// CreatePortForward inserts a new port forward and returns it.
func (s *Store) CreatePortForward(in PortForwardInput) (PortForward, error) {
	now := time.Now().Unix()
	pf := PortForward{
		ID:         uuid.NewString(),
		HostID:     in.HostID,
		Label:      in.Label,
		LocalPort:  in.LocalPort,
		RemoteHost: in.RemoteHost,
		RemotePort: in.RemotePort,
		Enabled:    in.Enabled,
		CreatedAt:  now,
	}
	enabled := 0
	if pf.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO port_forwards(id, host_id, label, local_port, remote_host, remote_port, enabled, created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		pf.ID, pf.HostID, pf.Label, pf.LocalPort, pf.RemoteHost, pf.RemotePort, enabled, pf.CreatedAt,
	)
	if err != nil {
		return PortForward{}, fmt.Errorf("store: create port_forward: %w", err)
	}
	return pf, nil
}

// UpdatePortForward overwrites a port forward's mutable fields.
func (s *Store) UpdatePortForward(id string, in PortForwardInput) (PortForward, error) {
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	res, err := s.db.Exec(
		`UPDATE port_forwards SET label=?, local_port=?, remote_host=?, remote_port=?, enabled=?
		 WHERE id=?`,
		in.Label, in.LocalPort, in.RemoteHost, in.RemotePort, enabled, id,
	)
	if err != nil {
		return PortForward{}, fmt.Errorf("store: update port_forward: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return PortForward{}, fmt.Errorf("store: update port_forward: id %q not found", id)
	}
	return s.GetPortForward(id)
}

// GetPortForward returns one port forward by id.
func (s *Store) GetPortForward(id string) (PortForward, error) {
	var (
		pf      PortForward
		enabled int
	)
	err := s.db.QueryRow(`
		SELECT id, host_id, label, local_port, remote_host, remote_port, enabled, created_at
		FROM port_forwards WHERE id=?`, id,
	).Scan(&pf.ID, &pf.HostID, &pf.Label, &pf.LocalPort, &pf.RemoteHost, &pf.RemotePort, &enabled, &pf.CreatedAt)
	if err != nil {
		return PortForward{}, fmt.Errorf("store: get port_forward: %w", err)
	}
	pf.Enabled = enabled != 0
	return pf, nil
}

// DeletePortForward removes a port forward.
func (s *Store) DeletePortForward(id string) error {
	_, err := s.db.Exec(`DELETE FROM port_forwards WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete port_forward: %w", err)
	}
	return nil
}
