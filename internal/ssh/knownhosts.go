// Package ssh implements Bastion's SSH layer: a known-hosts trust store and an
// interactive PTY session manager. It is deliberately independent of the vault
// and store packages — callers pass already-resolved connection details and
// receive output through an Emitter, so the package is fully testable without
// Wails or a real remote server.
package ssh

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// UnknownHostKeyError is returned by a HostKeyCallback when the server's key is
// not present in the known-hosts file. It carries everything the UI needs to
// render a one-time trust prompt and everything Trust/TrustEncoded needs to
// persist the key afterward.
type UnknownHostKeyError struct {
	Hostname          string
	Port              int
	KeyType           string // e.g. "ssh-ed25519"
	FingerprintSHA256 string // "SHA256:..."
	Base64Key         string // standard-base64 of the wire-format public key
}

func (e *UnknownHostKeyError) Error() string {
	return fmt.Sprintf("ssh: unknown host key for %s:%d (%s %s)",
		e.Hostname, e.Port, e.KeyType, e.FingerprintSHA256)
}

// HostKeyMismatchError is returned when the server presents a key that differs
// from the one already pinned for that host. This is a hard failure — never a
// trust prompt — because it may indicate a man-in-the-middle.
type HostKeyMismatchError struct {
	Hostname          string
	Port              int
	FingerprintSHA256 string
}

func (e *HostKeyMismatchError) Error() string {
	return fmt.Sprintf("ssh: host key mismatch for %s:%d (got %s)",
		e.Hostname, e.Port, e.FingerprintSHA256)
}

// KnownHosts wraps an on-disk OpenSSH known_hosts file. It is safe for
// concurrent use; the file is the source of truth and is reloaded on each
// Callback() so that a Trust() takes effect on the next connection attempt.
type KnownHosts struct {
	path string
	mu   sync.Mutex
}

// NewKnownHosts returns a KnownHosts backed by the file at path. The file (and
// its parent directory) are created if absent so the first connection can read
// an empty store rather than erroring.
func NewKnownHosts(path string) (*KnownHosts, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("ssh: known_hosts dir: %w", err)
	}
	// Touch the file so knownhosts.New does not fail on a missing path.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ssh: known_hosts create: %w", err)
	}
	_ = f.Close()
	return &KnownHosts{path: path}, nil
}

// Callback returns an ssh.HostKeyCallback that consults the known-hosts file.
// An unknown host yields *UnknownHostKeyError; a changed key yields
// *HostKeyMismatchError; a match yields nil.
func (k *KnownHosts) Callback() gossh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		k.mu.Lock()
		cb, err := knownhosts.New(k.path)
		k.mu.Unlock()
		if err != nil {
			return fmt.Errorf("ssh: load known_hosts: %w", err)
		}

		err = cb(hostname, remote, key)
		if err == nil {
			return nil
		}

		host, port := splitHostPort(hostname)

		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) == 0 {
				return &UnknownHostKeyError{
					Hostname:          host,
					Port:              port,
					KeyType:           key.Type(),
					FingerprintSHA256: gossh.FingerprintSHA256(key),
					Base64Key:         base64.StdEncoding.EncodeToString(key.Marshal()),
				}
			}
			return &HostKeyMismatchError{
				Hostname:          host,
				Port:              port,
				FingerprintSHA256: gossh.FingerprintSHA256(key),
			}
		}
		return err
	}
}

// Trust appends key for host:port to the known-hosts file. Used internally and
// by tests where a parsed public key is already in hand.
func (k *KnownHosts) Trust(hostname string, port int, key gossh.PublicKey) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	addr := knownhosts.Normalize(net.JoinHostPort(hostname, strconv.Itoa(port)))
	line := knownhosts.Line([]string{addr}, key)

	f, err := os.OpenFile(k.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("ssh: open known_hosts: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("ssh: append known_hosts: %w", err)
	}
	return nil
}

// TrustEncoded is the string-friendly entry point for the Phase 3 IPC layer,
// which receives keyType and a base64-encoded key across the Wails boundary.
func (k *KnownHosts) TrustEncoded(hostname string, port int, keyType, base64Key string) error {
	raw, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return fmt.Errorf("ssh: decode trusted key: %w", err)
	}
	key, err := gossh.ParsePublicKey(raw)
	if err != nil {
		return fmt.Errorf("ssh: parse trusted key: %w", err)
	}
	if key.Type() != keyType {
		return fmt.Errorf("ssh: trusted key type %q does not match %q", key.Type(), keyType)
	}
	return k.Trust(hostname, port, key)
}

// splitHostPort parses "host:port"; on any parse failure it returns the input
// as the host with port 22.
func splitHostPort(hostport string) (string, int) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport, 22
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 22
	}
	return host, port
}
