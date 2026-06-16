package ssh

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	gossh "golang.org/x/crypto/ssh"
)

// ForwardRule describes a single port forwarding rule to apply when a session
// is opened. The local address is always 127.0.0.1.
type ForwardRule struct {
	ID         string
	LocalPort  int
	RemoteHost string
	RemotePort int
}

// ActiveForwardInfo is returned to the frontend for the status indicator.
type ActiveForwardInfo struct {
	ID          string `json:"id"`
	LocalPort   int    `json:"localPort"`
	RemoteHost  string `json:"remoteHost"`
	RemotePort  int    `json:"remotePort"`
	ActiveConns int64  `json:"activeConns"`
	TotalConns  int64  `json:"totalConns"`
}

// forwardInstance is one running listener for a single rule.
type forwardInstance struct {
	rule       ForwardRule
	listener   net.Listener

	activeConns atomic.Int64
	totalConns  atomic.Int64

	wg sync.WaitGroup
}

// PortForwardManager owns all active port forwarding listeners across sessions.
type PortForwardManager struct {
	mu       sync.Mutex
	byClient map[string][]*forwardInstance // clientID -> forwards
}

// NewPortForwardManager returns an empty manager.
func NewPortForwardManager() *PortForwardManager {
	return &PortForwardManager{
		byClient: make(map[string][]*forwardInstance),
	}
}

// Start begins listening for each rule and proxying connections through the
// SSH client. It is a no-op if rules is empty. Errors for individual rules
// are returned via a slice — a port conflict on one rule does not block others.
func (m *PortForwardManager) Start(clientID string, client *gossh.Client, rules []ForwardRule) []error {
	if len(rules) == 0 {
		return nil
	}

	var instances []*forwardInstance
	var errs []error

	for _, rule := range rules {
		addr := fmt.Sprintf("127.0.0.1:%d", rule.LocalPort)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			errs = append(errs, fmt.Errorf("forward %s (%d): %w", rule.ID, rule.LocalPort, err))
			continue
		}

		fi := &forwardInstance{
			rule:     rule,
			listener: listener,
		}
		fi.wg.Add(1)
		instances = append(instances, fi)
		go fi.acceptLoop(client)
	}

	m.mu.Lock()
	m.byClient[clientID] = append(m.byClient[clientID], instances...)
	m.mu.Unlock()

	return errs
}

// Stop terminates all forward listeners for clientID and waits for proxy
// goroutines to finish.
func (m *PortForwardManager) Stop(clientID string) {
	m.mu.Lock()
	instances := m.byClient[clientID]
	delete(m.byClient, clientID)
	m.mu.Unlock()

	for _, fi := range instances {
		fi.shutdown()
	}
}

// StopAll terminates every active forward across all clients.
func (m *PortForwardManager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.byClient))
	for id := range m.byClient {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.Stop(id)
	}
}

// ListActive returns a snapshot of all running forwards and their connection
// counts.
func (m *PortForwardManager) ListActive() []ActiveForwardInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []ActiveForwardInfo
	for _, instances := range m.byClient {
		for _, fi := range instances {
			out = append(out, ActiveForwardInfo{
				ID:          fi.rule.ID,
				LocalPort:   fi.rule.LocalPort,
				RemoteHost:  fi.rule.RemoteHost,
				RemotePort:  fi.rule.RemotePort,
				ActiveConns: fi.activeConns.Load(),
				TotalConns:  fi.totalConns.Load(),
			})
		}
	}
	return out
}

// acceptLoop runs until the listener is closed.
func (fi *forwardInstance) acceptLoop(client *gossh.Client) {
	defer fi.wg.Done()
	for {
		local, err := fi.listener.Accept()
		if err != nil {
			return // listener closed
		}

		fi.activeConns.Add(1)
		fi.totalConns.Add(1)

		go func() {
			defer local.Close()
			defer fi.activeConns.Add(-1)

			remote, err := client.Dial("tcp", net.JoinHostPort(fi.rule.RemoteHost, fmt.Sprintf("%d", fi.rule.RemotePort)))
			if err != nil {
				return
			}
			defer remote.Close()

			var wg sync.WaitGroup
			wg.Add(2)
			go func() { io.Copy(remote, local); wg.Done() }()
			go func() { io.Copy(local, remote); wg.Done() }()
			wg.Wait()
		}()
	}
}

func (fi *forwardInstance) shutdown() {
	fi.listener.Close()
	fi.wg.Wait()
}
