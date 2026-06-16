package ssh

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"
)

// outputChunk is the size of the buffer used when reading PTY output. The
// reader emits whatever it has read each iteration, so this is a max chunk
// size, not a fixed frame.
const outputChunk = 4 * 1024

// Emitter is how the session manager streams PTY output and lifecycle events
// back to its caller. Phase 3 implements this with Wails' EventsEmit; tests
// implement it with a channel. It must be safe for concurrent calls.
type Emitter interface {
	// EmitOutput delivers a chunk of raw PTY bytes for the given session.
	EmitOutput(sessionID string, chunk []byte)
	// EmitClosed signals that the session has ended. reason is one of
	// "eof", "closed_by_user", or "error: <detail>".
	EmitClosed(sessionID string, reason string)
}

// DialConfig holds everything needed to open one session. Credentials are
// already resolved: the caller (Phase 3 app layer) decrypts stored secrets and
// supplies a ready AuthMethod. This package never touches the vault.
type DialConfig struct {
	Hostname string
	Port     int
	Username string
	Auth     []gossh.AuthMethod
	// HostKey is the callback consulted during the handshake. Use a
	// KnownHosts.Callback() in production; tests may pass a fixed callback.
	HostKey gossh.HostKeyCallback
	// Timeout bounds the TCP dial + handshake. Zero means a 15s default.
	Timeout time.Duration
	// ForwardRules are port forwarding rules to start when the session
	// connects. They are shut down automatically when the session ends.
	ForwardRules []ForwardRule
}

// runningSession is one live connection tracked by the Manager.
type runningSession struct {
	id      string
	client  *gossh.Client
	session *gossh.Session
	stdin   io.WriteCloser

	startedAt time.Time

	closeOnce sync.Once
}

// Manager owns the set of live SSH sessions and streams their output through
// an Emitter. It is safe for concurrent use.
type Manager struct {
	emitter        Emitter
	sessions       sync.Map // sessionID -> *runningSession
	forwardManager *PortForwardManager // optional, may be nil
}

// NewManager returns a Manager that emits output and lifecycle events through e.
// An optional PortForwardManager can be provided for automated forward lifecycle.
func NewManager(e Emitter, fm *PortForwardManager) *Manager {
	return &Manager{emitter: e, forwardManager: fm}
}

// Open dials the host, requests a PTY of the given size, starts the login
// shell, and begins streaming output. It returns the new session's ID.
//
// If the host key is unknown or changed, Open returns the structured error
// from the HostKeyCallback (*UnknownHostKeyError / *HostKeyMismatchError)
// without creating a session.
func (m *Manager) Open(cfg DialConfig, cols, rows int) (string, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	clientCfg := &gossh.ClientConfig{
		User:            cfg.Username,
		Auth:            cfg.Auth,
		HostKeyCallback: cfg.HostKey,
		Timeout:         timeout,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Hostname, cfg.Port)
	client, err := gossh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return "", err
	}

	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return "", fmt.Errorf("ssh: new session: %w", err)
	}

	modes := gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.ECHOCTL:       1,
		gossh.ICRNL:         1,
		gossh.ONLCR:         1,
		gossh.OPOST:         1,
		gossh.TTY_OP_ISPEED: 38400,
		gossh.TTY_OP_OSPEED: 38400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = session.Close()
		_ = client.Close()
		return "", fmt.Errorf("ssh: request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return "", fmt.Errorf("ssh: stdin pipe: %w", err)
	}
	// Merge stdout and stderr — a PTY normally muxes both onto one stream, but
	// be explicit so nothing is dropped.
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return "", fmt.Errorf("ssh: stdout pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return "", fmt.Errorf("ssh: stderr pipe: %w", err)
	}

	if err := session.Shell(); err != nil {
		_ = session.Close()
		_ = client.Close()
		return "", fmt.Errorf("ssh: start shell: %w", err)
	}

	// Consume stderr so the SSH channel never blocks. For PTY sessions the
	// remote shell merges both streams, but this also covers edge cases where
	// the server sends extended-data stderr packets.
	go io.Copy(io.Discard, stderr)

	rs := &runningSession{
		id:        uuid.NewString(),
		client:    client,
		session:   session,
		stdin:     stdin,
		startedAt: time.Now(),
	}
	m.sessions.Store(rs.id, rs)

	go m.pump(rs, stdout)

	// Start port forwards (best-effort — errors are logged but don't fail
	// the session).
	if m.forwardManager != nil && len(cfg.ForwardRules) > 0 {
		for _, err := range m.forwardManager.Start(rs.id, client, cfg.ForwardRules) {
			log.Printf("[forward] %s: %v", rs.id, err)
		}
	}

	return rs.id, nil
}

// pump reads PTY output until EOF or error, emitting chunks as they arrive.
// On exit it tears the session down and emits the closed event exactly once.
func (m *Manager) pump(rs *runningSession, stdout io.Reader) {
	buf := make([]byte, outputChunk)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			m.emitter.EmitOutput(rs.id, chunk)
		}
		if err != nil {
			reason := "eof"
			if err != io.EOF {
				reason = "error: " + err.Error()
			}
			m.finish(rs, reason)
			return
		}
	}
}

// finish closes the underlying session/client, removes it from the registry,
// and emits the closed event. It is idempotent per session.
func (m *Manager) finish(rs *runningSession, reason string) {
	rs.closeOnce.Do(func() {
		// Stop port forwards before closing the client.
		if m.forwardManager != nil {
			m.forwardManager.Stop(rs.id)
		}
		_ = rs.session.Close()
		_ = rs.client.Close()
		m.sessions.Delete(rs.id)
		m.emitter.EmitClosed(rs.id, reason)
	})
}

// Write sends data to the session's stdin (keystrokes, pasted snippets).
func (m *Manager) Write(sessionID string, data []byte) error {
	rs, ok := m.lookup(sessionID)
	if !ok {
		return fmt.Errorf("ssh: write: unknown session %q", sessionID)
	}
	if _, err := rs.stdin.Write(data); err != nil {
		return fmt.Errorf("ssh: write: %w", err)
	}
	return nil
}

// Resize informs the remote PTY of a new terminal size.
func (m *Manager) Resize(sessionID string, cols, rows int) error {
	rs, ok := m.lookup(sessionID)
	if !ok {
		return fmt.Errorf("ssh: resize: unknown session %q", sessionID)
	}
	if err := rs.session.WindowChange(rows, cols); err != nil {
		return fmt.Errorf("ssh: resize: %w", err)
	}
	return nil
}

// Close terminates a session on the user's request. It emits a closed event
// with reason "closed_by_user". Closing an already-closed session is a no-op.
func (m *Manager) Close(sessionID string) error {
	rs, ok := m.lookup(sessionID)
	if !ok {
		return fmt.Errorf("ssh: close: unknown session %q", sessionID)
	}
	m.finish(rs, "closed_by_user")
	return nil
}

// CloseAll terminates every live session (used on app shutdown).
func (m *Manager) CloseAll() {
	m.sessions.Range(func(_, v any) bool {
		m.finish(v.(*runningSession), "closed_by_user")
		return true
	})
}

// Count returns the number of live sessions.
func (m *Manager) Count() int {
	n := 0
	m.sessions.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// SessionInfo is the public health info for one live session.
type SessionInfo struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"startedAt"`
	Uptime    string    `json:"uptime"` // human-readable duration
}

// ListSessions returns info about all live sessions.
func (m *Manager) ListSessions() []SessionInfo {
	var out []SessionInfo
	m.sessions.Range(func(_, v any) bool {
		rs := v.(*runningSession)
		out = append(out, SessionInfo{
			ID:        rs.id,
			StartedAt: rs.startedAt,
			Uptime:    time.Since(rs.startedAt).Round(time.Second).String(),
		})
		return true
	})
	return out
}

func (m *Manager) lookup(sessionID string) (*runningSession, bool) {
	v, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	return v.(*runningSession), true
}
