package main

import (
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"crypto/ed25519"
	"crypto/rand"

	appssh "bastion/internal/ssh"

	glssh "github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// --- test emitter (captures events without Wails) ---------------------------

type capturingEmitter struct {
	mu     sync.Mutex
	output map[string][]byte
	closed map[string]string
}

func newCapturingEmitter() *capturingEmitter {
	return &capturingEmitter{output: map[string][]byte{}, closed: map[string]string{}}
}

func (c *capturingEmitter) EmitOutput(id string, chunk []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.output[id] = append(c.output[id], chunk...)
}

func (c *capturingEmitter) EmitClosed(id, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed[id] = reason
}

func (c *capturingEmitter) waitOutput(t *testing.T, id, substr string, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		got := string(c.output[id])
		c.mu.Unlock()
		if strings.Contains(got, substr) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q on session %s", substr, id)
}

func (c *capturingEmitter) waitClosed(t *testing.T, id string, d time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		r, ok := c.closed[id]
		c.mu.Unlock()
		if ok {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for close of session %s", id)
	return ""
}

// startEchoServer launches an in-process SSH echo server for these tests.
func startEchoServer(t *testing.T) (host string, port int) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, _ := gossh.NewSignerFromKey(priv)

	srv := &glssh.Server{
		Handler: func(s glssh.Session) {
			if _, _, isPty := s.Pty(); !isPty {
				return
			}
			_, _ = io.WriteString(s, "READY\r\n")
			buf := make([]byte, 1024)
			for {
				n, err := s.Read(buf)
				if n > 0 {
					_, _ = s.Write(buf[:n])
				}
				if err != nil {
					return
				}
			}
		},
		PasswordHandler: func(glssh.Context, string) bool { return true },
	}
	srv.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "127.0.0.1", ln.Addr().(*net.TCPAddr).Port
}

// appWithEmitter builds an unlocked App whose session manager reports to em.
func appWithEmitter(t *testing.T, em appssh.Emitter) *App {
	t.Helper()
	a := unlockedApp(t)
	a.sessions = appssh.NewManager(em, nil)
	return a
}

func TestOpenSession_UnknownHostThenTrustThenConnect(t *testing.T) {
	host, port := startEchoServer(t)
	em := newCapturingEmitter()
	a := appWithEmitter(t, em)

	dto, err := a.CreateHost(HostInput{
		Label: "echo", Hostname: host, Port: port, Username: "u",
		AuthKind: "password", Password: "pw",
	})
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	// First open: host key is unknown -> structured prompt, NOT an error.
	res, err := a.OpenSession(dto.ID, 80, 24)
	if err != nil {
		t.Fatalf("OpenSession (unknown host) should not error, got: %v", err)
	}
	if res.SessionID != "" {
		t.Fatal("no session should be opened for an unknown host")
	}
	if res.UnknownHostKey == nil {
		t.Fatal("expected an unknownHostKey prompt payload")
	}
	if res.UnknownHostKey.Port != port || res.UnknownHostKey.FingerprintSHA256 == "" {
		t.Fatalf("bad prompt payload: %+v", res.UnknownHostKey)
	}

	// Trust the key as the UI would, then retry.
	u := res.UnknownHostKey
	if err := a.TrustHostKey(u.Hostname, u.Port, u.KeyType, u.Base64Key); err != nil {
		t.Fatalf("TrustHostKey: %v", err)
	}

	res, err = a.OpenSession(dto.ID, 80, 24)
	if err != nil {
		t.Fatalf("OpenSession after trust: %v", err)
	}
	if res.SessionID == "" || res.UnknownHostKey != nil {
		t.Fatalf("expected a session id after trust, got %+v", res)
	}

	em.waitOutput(t, res.SessionID, "READY", 5*time.Second)

	if err := a.WriteToSession(res.SessionID, "hello\n"); err != nil {
		t.Fatalf("WriteToSession: %v", err)
	}
	em.waitOutput(t, res.SessionID, "hello", 5*time.Second)

	if err := a.ResizeSession(res.SessionID, 120, 40); err != nil {
		t.Fatalf("ResizeSession: %v", err)
	}

	if err := a.CloseSession(res.SessionID); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	if reason := em.waitClosed(t, res.SessionID, 5*time.Second); reason != "closed_by_user" {
		t.Fatalf("close reason: got %q", reason)
	}
}

func TestOpenSession_RequiresUnlock(t *testing.T) {
	a := newTestApp(t) // locked
	a.sessions = appssh.NewManager(newCapturingEmitter(), nil)
	if _, err := a.OpenSession("any", 80, 24); err == nil {
		t.Fatal("OpenSession must fail when locked")
	}
}

func TestOpenSession_KeyAuthMissingFileErrors(t *testing.T) {
	host, port := startEchoServer(t)
	a := appWithEmitter(t, newCapturingEmitter())
	dto, _ := a.CreateHost(HostInput{
		Label: "k", Hostname: host, Port: port, Username: "u",
		AuthKind: "key", KeyPath: "/nonexistent/id_ed25519",
	})
	if _, err := a.OpenSession(dto.ID, 80, 24); err == nil {
		t.Fatal("OpenSession with a missing key file must error")
	}
}

func TestSessionLogging_DisabledByDefault(t *testing.T) {
	a := unlockedApp(t)
	if a.SessionLoggingEnabled() {
		t.Fatal("session logging must be OFF by default")
	}
	if err := a.SetSessionLogging(true); err != nil {
		t.Fatalf("SetSessionLogging: %v", err)
	}
	if !a.SessionLoggingEnabled() {
		t.Fatal("SetSessionLogging(true) must enable logging")
	}
	if err := a.SetSessionLogging(false); err != nil {
		t.Fatalf("SetSessionLogging: %v", err)
	}
	if a.SessionLoggingEnabled() {
		t.Fatal("SetSessionLogging(false) must disable logging")
	}
}

func TestSetSessionLogging_RequiresUnlock(t *testing.T) {
	a := newTestApp(t) // locked
	if err := a.SetSessionLogging(true); err == nil {
		t.Fatal("SetSessionLogging must fail when the vault is locked")
	}
}

// openTrustedSession dials the echo server, trusting its key, and returns a live
// session id. It centralizes the unknown-host-then-trust dance.
func openTrustedSession(t *testing.T, a *App, em *capturingEmitter, hostID string) string {
	t.Helper()
	res, err := a.OpenSession(hostID, 80, 24)
	if err != nil {
		t.Fatalf("OpenSession (prompt): %v", err)
	}
	if res.UnknownHostKey == nil {
		t.Fatal("expected an unknown-host prompt")
	}
	u := res.UnknownHostKey
	if err := a.TrustHostKey(u.Hostname, u.Port, u.KeyType, u.Base64Key); err != nil {
		t.Fatalf("TrustHostKey: %v", err)
	}
	res, err = a.OpenSession(hostID, 80, 24)
	if err != nil || res.SessionID == "" {
		t.Fatalf("OpenSession after trust: id=%q err=%v", res.SessionID, err)
	}
	em.waitOutput(t, res.SessionID, "READY", 5*time.Second)
	return res.SessionID
}

func TestLock_TearsDownLiveSessionsAndBlocksWrites(t *testing.T) {
	host, port := startEchoServer(t)
	em := newCapturingEmitter()
	a := appWithEmitter(t, em)

	dto, err := a.CreateHost(HostInput{
		Label: "echo", Hostname: host, Port: port, Username: "u",
		AuthKind: "password", Password: "pw",
	})
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	id := openTrustedSession(t, a, em, dto.ID)

	// Locking must close the live session...
	if err := a.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if reason := em.waitClosed(t, id, 5*time.Second); reason != "closed_by_user" {
		t.Fatalf("locking should close live sessions, got reason %q", reason)
	}
	if n := a.sessions.Count(); n != 0 {
		t.Fatalf("no sessions should remain after Lock, have %d", n)
	}

	// ...and a locked vault must reject further writes/resizes outright.
	if err := a.WriteToSession(id, "x\n"); err == nil {
		t.Fatal("WriteToSession must fail while locked")
	}
	if err := a.ResizeSession(id, 100, 30); err == nil {
		t.Fatal("ResizeSession must fail while locked")
	}
}
