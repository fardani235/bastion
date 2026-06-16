package ssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	glssh "github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// --- test emitter -----------------------------------------------------------

type closeEvent struct {
	id     string
	reason string
}

type chanEmitter struct {
	out    chan []byte
	closed chan closeEvent
}

func newChanEmitter() *chanEmitter {
	return &chanEmitter{
		out:    make(chan []byte, 256),
		closed: make(chan closeEvent, 8),
	}
}

func (c *chanEmitter) EmitOutput(_ string, chunk []byte) { c.out <- chunk }
func (c *chanEmitter) EmitClosed(id, reason string)      { c.closed <- closeEvent{id, reason} }

// waitForOutput accumulates emitted chunks until they contain substr, or fails
// after the timeout.
func (c *chanEmitter) waitForOutput(t *testing.T, substr string, timeout time.Duration) {
	t.Helper()
	var acc strings.Builder
	deadline := time.After(timeout)
	for {
		select {
		case chunk := <-c.out:
			acc.Write(chunk)
			if strings.Contains(acc.String(), substr) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q; got so far: %q", substr, acc.String())
		}
	}
}

func (c *chanEmitter) waitForClose(t *testing.T, timeout time.Duration) closeEvent {
	t.Helper()
	select {
	case ev := <-c.closed:
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for close event")
		return closeEvent{}
	}
}

// --- fake SSH server --------------------------------------------------------

// startEchoServer launches a gliderlabs/ssh server that accepts any password,
// writes a banner on PTY allocation, and echoes stdin back to stdout. It
// returns the host, port, and the server's host public key.
func startEchoServer(t *testing.T) (host string, port int, hostKey gossh.PublicKey) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}

	handler := func(s glssh.Session) {
		_, _, isPty := s.Pty()
		if !isPty {
			_, _ = io.WriteString(s, "no pty\n")
			return
		}
		_, _ = io.WriteString(s, "WELCOME\r\n")
		buf := make([]byte, 1024)
		for {
			n, err := s.Read(buf)
			if n > 0 {
				// Ctrl-D (EOT) ends the shell, as a real login shell would.
				if idx := bytes.IndexByte(buf[:n], 0x04); idx >= 0 {
					_, _ = s.Write(buf[:idx]) // echo anything before it
					return
				}
				_, _ = s.Write(buf[:n]) // echo
			}
			if err != nil {
				return
			}
		}
	}

	srv := &glssh.Server{
		Handler:         handler,
		PasswordHandler: func(glssh.Context, string) bool { return true },
	}
	srv.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	tcp := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", tcp.Port, signer.PublicKey()
}

// --- tests ------------------------------------------------------------------

func TestSession_FullLifecycle(t *testing.T) {
	host, port, _ := startEchoServer(t)
	khPath := filepath.Join(t.TempDir(), "known_hosts")
	kh, err := NewKnownHosts(khPath)
	if err != nil {
		t.Fatalf("NewKnownHosts: %v", err)
	}

	em := newChanEmitter()
	mgr := NewManager(em, nil)

	cfg := DialConfig{
		Hostname: host,
		Port:     port,
		Username: "tester",
		Auth:     []gossh.AuthMethod{gossh.Password("any")},
		HostKey:  kh.Callback(),
		Timeout:  5 * time.Second,
	}

	// 1. First connect: the host is unknown -> structured trust prompt.
	_, err = mgr.Open(cfg, 80, 24)
	var unknown *UnknownHostKeyError
	if !errors.As(err, &unknown) {
		t.Fatalf("expected *UnknownHostKeyError on first connect, got %T: %v", err, err)
	}
	if unknown.Port != port {
		t.Fatalf("trust prompt port: got %d want %d", unknown.Port, port)
	}

	// 2. Trust the key exactly as the IPC layer would (type + base64).
	if err := kh.TrustEncoded(unknown.Hostname, unknown.Port, unknown.KeyType, unknown.Base64Key); err != nil {
		t.Fatalf("TrustEncoded: %v", err)
	}

	// 3. Reconnect: now accepted.
	id, err := mgr.Open(cfg, 80, 24)
	if err != nil {
		t.Fatalf("Open after trust: %v", err)
	}
	if id == "" {
		t.Fatal("expected a session id")
	}
	if mgr.Count() != 1 {
		t.Fatalf("expected 1 live session, got %d", mgr.Count())
	}

	// 4. Banner streams through the emitter.
	em.waitForOutput(t, "WELCOME", 5*time.Second)

	// 5. Keystrokes are written and echoed back.
	if err := mgr.Write(id, []byte("ping\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	em.waitForOutput(t, "ping", 5*time.Second)

	// 6. Resize is accepted by the live PTY.
	if err := mgr.Resize(id, 120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	// 7. Close emits exactly one closed_by_user event and drops the count.
	if err := mgr.Close(id); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ev := em.waitForClose(t, 5*time.Second)
	if ev.id != id || ev.reason != "closed_by_user" {
		t.Fatalf("close event: got %+v, want id=%s reason=closed_by_user", ev, id)
	}
	if mgr.Count() != 0 {
		t.Fatalf("expected 0 live sessions after close, got %d", mgr.Count())
	}
}

func TestSession_ServerHangupEmitsEOF(t *testing.T) {
	host, port, hostKey := startEchoServer(t)
	khPath := filepath.Join(t.TempDir(), "known_hosts")
	kh, _ := NewKnownHosts(khPath)
	// Pre-trust so Open succeeds directly.
	if err := kh.Trust(host, port, hostKey); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	em := newChanEmitter()
	mgr := NewManager(em, nil)
	cfg := DialConfig{
		Hostname: host, Port: port, Username: "tester",
		Auth:    []gossh.AuthMethod{gossh.Password("any")},
		HostKey: kh.Callback(), Timeout: 5 * time.Second,
	}

	id, err := mgr.Open(cfg, 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	em.waitForOutput(t, "WELCOME", 5*time.Second)

	// Ask the remote shell to exit; the output goroutine should see EOF and
	// emit a closed event with a non-user reason.
	if err := mgr.Write(id, []byte("\x04")); err != nil { // Ctrl-D
		t.Fatalf("Write EOF: %v", err)
	}
	ev := em.waitForClose(t, 5*time.Second)
	if ev.reason == "closed_by_user" {
		t.Fatalf("server-side hangup must not be reported as closed_by_user")
	}
}

func TestSession_CloseAllTerminatesEverySession(t *testing.T) {
	host, port, hostKey := startEchoServer(t)
	kh, _ := NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	if err := kh.Trust(host, port, hostKey); err != nil {
		t.Fatalf("Trust: %v", err)
	}

	em := newChanEmitter()
	mgr := NewManager(em, nil)
	cfg := DialConfig{
		Hostname: host, Port: port, Username: "tester",
		Auth:    []gossh.AuthMethod{gossh.Password("any")},
		HostKey: kh.Callback(), Timeout: 5 * time.Second,
	}

	for i := 0; i < 2; i++ {
		if _, err := mgr.Open(cfg, 80, 24); err != nil {
			t.Fatalf("Open #%d: %v", i, err)
		}
	}
	if mgr.Count() != 2 {
		t.Fatalf("expected 2 live sessions, got %d", mgr.Count())
	}

	mgr.CloseAll()

	if mgr.Count() != 0 {
		t.Fatalf("expected 0 live sessions after CloseAll, got %d", mgr.Count())
	}
	// Two close events, both closed_by_user.
	for i := 0; i < 2; i++ {
		ev := em.waitForClose(t, 5*time.Second)
		if ev.reason != "closed_by_user" {
			t.Fatalf("CloseAll event %d reason: got %q", i, ev.reason)
		}
	}
}

func TestSession_OperationsOnUnknownSessionFail(t *testing.T) {
	mgr := NewManager(newChanEmitter(), nil)
	if err := mgr.Write("nope", []byte("x")); err == nil {
		t.Fatal("Write on unknown session must fail")
	}
	if err := mgr.Resize("nope", 80, 24); err == nil {
		t.Fatal("Resize on unknown session must fail")
	}
	if err := mgr.Close("nope"); err == nil {
		t.Fatal("Close on unknown session must fail")
	}
}

func TestSession_DialFailureReturnsError(t *testing.T) {
	// Grab a port, then close the listener so nothing is listening there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	kh, _ := NewKnownHosts(filepath.Join(t.TempDir(), "known_hosts"))
	mgr := NewManager(newChanEmitter(), nil)
	cfg := DialConfig{
		Hostname: "127.0.0.1", Port: port, Username: "tester",
		Auth:    []gossh.AuthMethod{gossh.Password("any")},
		HostKey: kh.Callback(), Timeout: 2 * time.Second,
	}

	if _, err := mgr.Open(cfg, 80, 24); err == nil {
		t.Fatal("Open against a dead port must return a dial error")
	}
	if mgr.Count() != 0 {
		t.Fatalf("a failed Open must not leave a tracked session; got %d", mgr.Count())
	}
}

func TestSession_MismatchedHostKeyRejected(t *testing.T) {
	host, port, _ := startEchoServer(t)
	khPath := filepath.Join(t.TempDir(), "known_hosts")
	kh, _ := NewKnownHosts(khPath)

	// Trust a DIFFERENT key for this host:port, simulating a changed server key.
	wrong := testKey(t)
	if err := kh.Trust(host, port, wrong); err != nil {
		t.Fatalf("Trust wrong key: %v", err)
	}

	mgr := NewManager(newChanEmitter(), nil)
	cfg := DialConfig{
		Hostname: host, Port: port, Username: "tester",
		Auth:    []gossh.AuthMethod{gossh.Password("any")},
		HostKey: kh.Callback(), Timeout: 5 * time.Second,
	}
	_, err := mgr.Open(cfg, 80, 24)
	var mismatch *HostKeyMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected *HostKeyMismatchError, got %T: %v", err, err)
	}
}
