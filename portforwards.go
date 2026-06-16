package main

import (
	"fmt"

	appssh "bastion/internal/ssh"
	"bastion/internal/store"
)

// PortForwardDTO is the frontend-facing view of a port forward.
type PortForwardDTO struct {
	ID         string `json:"id"`
	HostID     string `json:"hostId"`
	Label      string `json:"label"`
	LocalPort  int    `json:"localPort"`
	RemoteHost string `json:"remoteHost"`
	RemotePort int    `json:"remotePort"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"createdAt"`
}

func toPortForwardDTO(pf store.PortForward) PortForwardDTO {
	return PortForwardDTO{
		ID:         pf.ID,
		HostID:     pf.HostID,
		Label:      pf.Label,
		LocalPort:  pf.LocalPort,
		RemoteHost: pf.RemoteHost,
		RemotePort: pf.RemotePort,
		Enabled:    pf.Enabled,
		CreatedAt:  pf.CreatedAt,
	}
}

// ListPortForwards returns all port forwards for the given host.
func (a *App) ListPortForwards(hostID string) ([]PortForwardDTO, error) {
	if !a.IsUnlocked() {
		return nil, errLocked
	}
	rows, err := a.store.ListPortForwards(hostID)
	if err != nil {
		return nil, err
	}
	out := make([]PortForwardDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPortForwardDTO(r))
	}
	return out, nil
}

// CreatePortForward adds a new port forward config.
func (a *App) CreatePortForward(hostID, label string, localPort int, remoteHost string, remotePort int, enabled bool) (PortForwardDTO, error) {
	if !a.IsUnlocked() {
		return PortForwardDTO{}, errLocked
	}
	if err := validatePort(localPort); err != nil {
		return PortForwardDTO{}, fmt.Errorf("bastion: local port: %w", err)
	}
	if err := validatePort(remotePort); err != nil {
		return PortForwardDTO{}, fmt.Errorf("bastion: remote port: %w", err)
	}
	if err := validateHostname(remoteHost); err != nil {
		return PortForwardDTO{}, fmt.Errorf("bastion: remote host: %w", err)
	}
	pf, err := a.store.CreatePortForward(store.PortForwardInput{
		HostID: hostID, Label: label, LocalPort: localPort,
		RemoteHost: remoteHost, RemotePort: remotePort, Enabled: enabled,
	})
	if err != nil {
		return PortForwardDTO{}, err
	}
	return toPortForwardDTO(pf), nil
}

// UpdatePortForward modifies an existing port forward config.
func (a *App) UpdatePortForward(id, hostID, label string, localPort int, remoteHost string, remotePort int, enabled bool) (PortForwardDTO, error) {
	if !a.IsUnlocked() {
		return PortForwardDTO{}, errLocked
	}
	if err := validatePort(localPort); err != nil {
		return PortForwardDTO{}, fmt.Errorf("bastion: local port: %w", err)
	}
	if err := validatePort(remotePort); err != nil {
		return PortForwardDTO{}, fmt.Errorf("bastion: remote port: %w", err)
	}
	if err := validateHostname(remoteHost); err != nil {
		return PortForwardDTO{}, fmt.Errorf("bastion: remote host: %w", err)
	}
	pf, err := a.store.UpdatePortForward(id, store.PortForwardInput{
		HostID: hostID, Label: label, LocalPort: localPort,
		RemoteHost: remoteHost, RemotePort: remotePort, Enabled: enabled,
	})
	if err != nil {
		return PortForwardDTO{}, err
	}
	return toPortForwardDTO(pf), nil
}

// DeletePortForward removes a port forward config.
func (a *App) DeletePortForward(id string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	return a.store.DeletePortForward(id)
}

// ListActiveForwards returns the current stats of running port forwards.
func (a *App) ListActiveForwards() ([]appssh.ActiveForwardInfo, error) {
	if !a.IsUnlocked() {
		return nil, errLocked
	}
	if a.forwardManager == nil {
		return nil, nil
	}
	return a.forwardManager.ListActive(), nil
}
