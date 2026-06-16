package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bastion/internal/store"
)

// parsedHost is one entry from an SSH config file.
type parsedHost struct {
	Label         string // from Host directive
	Hostname      string
	Port          int
	Username      string
	IdentityFile  string
}

// ScannedHost is a preview of a host found in the SSH config (not yet imported).
type ScannedHost struct {
	Label        string `json:"label"`
	Hostname     string `json:"hostname"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	IdentityFile string `json:"identityFile"`
}

// ScanSSHConfig parses ~/.ssh/config and returns a preview of hosts that would
// be imported, without saving anything. The user confirms the list, then calls
// ImportSSHConfig to actually persist.
func (a *App) ScanSSHConfig() ([]ScannedHost, error) {
	if !a.IsUnlocked() {
		return nil, errLocked
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("bastion: scan ssh config: %w", err)
	}
	path := filepath.Join(home, ".ssh", "config")

	entries, err := parseSSHConfig(path)
	if err != nil {
		return nil, err
	}

	out := make([]ScannedHost, len(entries))
	for i, e := range entries {
		out[i] = ScannedHost{
			Label:        e.Label,
			Hostname:     e.Hostname,
			Port:         e.Port,
			Username:     e.Username,
			IdentityFile: e.IdentityFile,
		}
	}
	return out, nil
}

// ImportSSHConfig parses ~/.ssh/config and bulk-imports all non-wildcard hosts.
func (a *App) ImportSSHConfig() ([]HostDTO, error) {
	if !a.IsUnlocked() {
		return nil, errLocked
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("bastion: import ssh config: %w", err)
	}
	path := filepath.Join(home, ".ssh", "config")

	entries, err := parseSSHConfig(path)
	if err != nil {
		return nil, err
	}

	key, err := a.keyCopy()
	if err != nil {
		return nil, err
	}
	defer zero(key)

	var imported []HostDTO
	var errs []error
	for _, e := range entries {
		si := store.HostInput{
			Label:    e.Label,
			Hostname: e.Hostname,
			Port:     e.Port,
			Username: e.Username,
			AuthKind: "key",
			KeyPath:  e.IdentityFile,
		}

		h, err := a.store.CreateHost(si)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Label, err))
			continue
		}
		imported = append(imported, toHostDTO(h))
	}
	for _, err := range errs {
		log.Printf("[import] %v", err)
	}

	return imported, nil
}

// parseSSHConfig reads an OpenSSH config file and returns non-wildcard host
// entries with their resolved properties.
func parseSSHConfig(path string) ([]parsedHost, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("bastion: open ssh config: %w", err)
	}
	defer f.Close()

	type block struct {
		patterns []string
		props    map[string]string
	}

	var blocks []*block
	var current *block

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for Host directive (starts line, case-insensitive).
		if u := strings.ToUpper(line); strings.HasPrefix(u, "HOST ") || strings.HasPrefix(u, "HOST\t") || line == "HOST" || line == "host" {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			current = &block{
				patterns: parts[1:],
				props:    make(map[string]string),
			}
			blocks = append(blocks, current)
			continue
		}

		if current == nil {
			continue
		}

		// Property line: "Key Value"
		if idx := strings.Index(line, " "); idx > 0 {
			key := strings.ToLower(strings.TrimSpace(line[:idx]))
			val := strings.TrimSpace(line[idx+1:])
			current.props[key] = val
		} else if idx := strings.Index(line, "="); idx > 0 {
			key := strings.ToLower(strings.TrimSpace(line[:idx]))
			val := strings.TrimSpace(line[idx+1:])
			current.props[key] = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("bastion: read ssh config: %w", err)
	}

	// Flatten blocks into entries, skipping wildcards.
	var entries []parsedHost
	for _, b := range blocks {
		hostname := b.props["hostname"]
		if hostname == "" {
			continue
		}

		// Use the first non-wildcard Host pattern as the label.
		label := b.patterns[0]
		for _, p := range b.patterns {
			if !strings.ContainsAny(p, "*?") {
				label = p
				break
			}
		}

		port := 22
		if p, ok := b.props["port"]; ok {
			if n, err := strconv.Atoi(p); err == nil && n > 0 && n <= 65535 {
				port = n
			}
		}

		username := b.props["user"]
		identityFile := expandTilde(b.props["identityfile"])

		entries = append(entries, parsedHost{
			Label:        label,
			Hostname:     hostname,
			Port:         port,
			Username:     username,
			IdentityFile: identityFile,
		})
	}

	return entries, nil
}

func expandTilde(p string) string {
	if p == "" {
		return ""
	}
	if p[0] == '~' {
		home, _ := os.UserHomeDir()
		if home != "" {
			return home + p[1:]
		}
	}
	return p
}
