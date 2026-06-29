package main

import "testing"

func TestValidateAIEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"empty is allowed (falls back to provider default)", "", false},
		{"whitespace-only is treated as empty", "   ", false},
		{"https remote", "https://api.openai.com/v1", false},
		{"https with port", "https://example.com:8443/v1", false},
		{"http localhost (Ollama)", "http://localhost:11434/v1", false},
		{"http 127.0.0.1 loopback", "http://127.0.0.1:11434/v1", false},
		{"http ipv6 loopback", "http://[::1]:11434/v1", false},
		{"http to remote host is rejected", "http://evil.example.com/v1", true},
		{"http to non-loopback IP is rejected", "http://10.0.0.5/v1", true},
		{"non-http scheme is rejected", "ftp://example.com/v1", true},
		{"file scheme is rejected", "file:///etc/passwd", true},
		{"missing host is rejected", "https://", true},
		{"garbage is rejected", "://nope", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAIEndpoint(tc.endpoint)
			if tc.wantErr && err == nil {
				t.Fatalf("validateAIEndpoint(%q) = nil, want error", tc.endpoint)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateAIEndpoint(%q) = %v, want nil", tc.endpoint, err)
			}
		})
	}
}

// TestSetAIConfig_RejectsBadEndpoint verifies an invalid endpoint is rejected
// before anything is persisted, so a hostile value never reaches storage.
func TestSetAIConfig_RejectsBadEndpoint(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("correct horse battery staple"); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err := a.SetAIConfig("openai-compatible", "llama3.2", "http://evil.example.com/v1", "sk-secret", "", false)
	if err == nil {
		t.Fatal("SetAIConfig accepted an http remote endpoint; want rejection")
	}

	if _, ok, _ := a.store.GetMeta(metaAIConfig); ok {
		t.Fatal("a rejected SetAIConfig must not persist any config")
	}
}

// TestSetAIConfig_RoundTrip covers the happy path: a valid endpoint stores,
// and a later blank-key save preserves the key.
func TestSetAIConfig_RoundTrip(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("correct horse battery staple"); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	if err := a.SetAIConfig("openai-compatible", "llama3.2", "http://localhost:11434/v1", "sk-secret", "", true); err != nil {
		t.Fatalf("SetAIConfig (initial): %v", err)
	}

	view, err := a.GetAIConfig()
	if err != nil {
		t.Fatalf("GetAIConfig: %v", err)
	}
	if !view.HasKey {
		t.Fatal("HasKey should be true after storing a key")
	}
	if !view.AutoExplainErrors {
		t.Fatal("AutoExplainErrors should round-trip as true")
	}
	if view.Endpoint != "http://localhost:11434/v1" {
		t.Fatalf("endpoint = %q, want localhost", view.Endpoint)
	}

	// A blank-key save must keep the stored key (renderer never holds it).
	if err := a.SetAIConfig("openai-compatible", "llama3.2", "http://localhost:11434/v1", "", "", false); err != nil {
		t.Fatalf("SetAIConfig (blank key): %v", err)
	}
	cfg, err := a.readAIConfig()
	if err != nil {
		t.Fatalf("readAIConfig: %v", err)
	}
	if cfg.APIKey != "sk-secret" {
		t.Fatalf("API key = %q, want it preserved across a blank-key save", cfg.APIKey)
	}
	if cfg.AutoExplainErrors {
		t.Fatal("AutoExplainErrors should now be false after the second save")
	}
}

// TestNewChat_WithoutConfig verifies NewChat fails when AI is not configured.
func TestNewChat_WithoutConfig(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("test-password"); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	_, err := a.NewChat()
	if err == nil {
		t.Fatal("NewChat must fail when AI is not configured")
	}
}

// TestChat_WithoutSession verifies Chat fails when no session exists.
func TestChat_WithoutSession(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("test-password"); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	_, err := a.Chat("nonexistent", "hello")
	if err == nil {
		t.Fatal("Chat must fail when AI is not configured")
	}
}

// TestChat_WithLockedVault verifies Chat IPC methods fail when vault is locked.
func TestChat_WithLockedVault(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("test-password"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := a.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}

	_, err := a.NewChat()
	if err != errLocked {
		t.Fatalf("NewChat with locked vault: got %v, want %v", err, errLocked)
	}

	_, err = a.Chat("any", "hello")
	if err != errLocked {
		t.Fatalf("Chat with locked vault: got %v, want %v", err, errLocked)
	}

	err = a.ClearChat("any")
	if err != errLocked {
		t.Fatalf("ClearChat with locked vault: got %v, want %v", err, errLocked)
	}
}

// TestClearChat_WithoutConfig verifies ClearChat fails when AI is not configured.
func TestClearChat_WithoutConfig(t *testing.T) {
	a := newTestApp(t)
	if err := a.Setup("test-password"); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	err := a.ClearChat("any")
	if err == nil {
		t.Fatal("ClearChat must fail when AI is not configured")
	}
}

// TestExtractShellCommand verifies the fenced-code-block extraction.
func TestExtractShellCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"No fence here", ""},
		{"```shell\nls -la\n```", "ls -la\n"},
		{"Text before\n```shell\nfind . -name '*.go'\n```\nText after", "find . -name '*.go'\n"},
		{"```shell\n   ps aux   \n```", "   ps aux   \n"},
		{"Unclosed fence ```shell\necho hi", ""},
	}
	for _, tc := range tests {
		got := extractShellCommand(tc.input)
		if got != tc.want {
			t.Errorf("extractShellCommand(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
