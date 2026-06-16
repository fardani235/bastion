package ssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// startForwardSSHServer starts an SSH server using x/crypto directly so that
// tcpip-forward global requests are handled properly. It proxies direct-tcpip
// channels to a TCP echo server on the given echoPort.
func startForwardSSHServer(t *testing.T, echoAddr string) (host string, port int) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}

	config := &gossh.ServerConfig{
		PasswordCallback: func(conn gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			return nil, nil
		},
	}
	config.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			tcpConn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sConn, chans, reqs, err := gossh.NewServerConn(tcpConn, config)
				if err != nil {
					return
				}
				defer sConn.Close()

				go gossh.DiscardRequests(reqs)

				for ch := range chans {
					if ch.ChannelType() == "direct-tcpip" {
						// Accept the channel and proxy to echoAddr.
						remote, err := net.Dial("tcp", echoAddr)
						if err != nil {
							ch.Reject(gossh.ConnectionFailed, "cannot reach echo target")
							continue
						}
						sshChan, reqs, err := ch.Accept()
						if err != nil {
							remote.Close()
							continue
						}
						go gossh.DiscardRequests(reqs)

						go func() {
							var wg sync.WaitGroup
							wg.Add(2)
							go func() { io.Copy(remote, sshChan); wg.Done() }()
							go func() { io.Copy(sshChan, remote); wg.Done() }()
							wg.Wait()
							remote.Close()
							sshChan.Close()
						}()
					} else {
						ch.Reject(gossh.UnknownChannelType, "unsupported channel")
					}
				}
			}()
		}
	}()

	tcp := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", tcp.Port
}

func TestPortForward_Echo(t *testing.T) {
	// Start a TCP echo server (the "remote" target).
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()

	host, port := startForwardSSHServer(t, echoLn.Addr().String())

	// Dial the SSH server.
	client, err := gossh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &gossh.ClientConfig{
		User:            "tester",
		Auth:            []gossh.AuthMethod{gossh.Password("any")},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	echoTCP := echoLn.Addr().(*net.TCPAddr)
	mgr := NewPortForwardManager()
	rule := ForwardRule{
		ID:         "echo-test",
		LocalPort:  pickFreePort(t),
		RemoteHost: "127.0.0.1",
		RemotePort: echoTCP.Port,
	}

	errs := mgr.Start("c1", client, []ForwardRule{rule})
	for _, e := range errs {
		t.Fatalf("Start: %v", e)
	}
	defer mgr.Stop("c1")

	// Connect to the forwarded local port.
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", rule.LocalPort))
	if err != nil {
		t.Fatalf("Dial forwarded port: %v", err)
	}
	defer conn.Close()

	payload := []byte("hello-port-forward")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo: got %q want %q", got, payload)
	}

	// Verify stats.
	active := mgr.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected 1 active forward, got %d", len(active))
	}
	if active[0].ActiveConns != 1 {
		t.Fatalf("expected 1 active conn, got %d", active[0].ActiveConns)
	}
	if active[0].TotalConns != 1 {
		t.Fatalf("expected 1 total conn, got %d", active[0].TotalConns)
	}
}

func TestPortForward_StopClearsActive(t *testing.T) {
	// Quick echo server that just accepts and discards.
	echoLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echoLn.Close()
	go func() { for { c, _ := echoLn.Accept(); if c != nil { c.Close() } } }()

	host, port := startForwardSSHServer(t, echoLn.Addr().String())
	client, _ := gossh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &gossh.ClientConfig{
		User: "tester", Auth: []gossh.AuthMethod{gossh.Password("any")},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	})
	defer client.Close()

	mgr := NewPortForwardManager()
	rule := ForwardRule{ID: "stop-test", LocalPort: pickFreePort(t), RemoteHost: "127.0.0.1", RemotePort: 22}

	errs := mgr.Start("c1", client, []ForwardRule{rule})
	for _, e := range errs {
		t.Fatalf("Start: %v", e)
	}

	if len(mgr.ListActive()) != 1 {
		t.Fatal("expected 1 active forward before Stop")
	}

	mgr.Stop("c1")

	if len(mgr.ListActive()) != 0 {
		t.Fatal("expected 0 active forwards after Stop")
	}
}

func TestPortForward_StopAll(t *testing.T) {
	echoLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echoLn.Close()
	go func() { for { c, _ := echoLn.Accept(); if c != nil { c.Close() } } }()

	host, port := startForwardSSHServer(t, echoLn.Addr().String())

	dial := func() *gossh.Client {
		c, _ := gossh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &gossh.ClientConfig{
			User: "tester", Auth: []gossh.AuthMethod{gossh.Password("any")},
			HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
		})
		return c
	}
	c1 := dial()
	c2 := dial()
	defer c1.Close()
	defer c2.Close()

	mgr := NewPortForwardManager()
	r1 := ForwardRule{ID: "c1-f", LocalPort: pickFreePort(t), RemoteHost: "127.0.0.1", RemotePort: 22}
	r2 := ForwardRule{ID: "c2-f", LocalPort: pickFreePort(t), RemoteHost: "127.0.0.1", RemotePort: 22}
	_ = mgr.Start("c1", c1, []ForwardRule{r1})
	_ = mgr.Start("c2", c2, []ForwardRule{r2})

	mgr.StopAll()

	if len(mgr.ListActive()) != 0 {
		t.Fatal("expected 0 active forwards after StopAll")
	}
}

func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	_ = ln.Close()
	return addr.Port
}
