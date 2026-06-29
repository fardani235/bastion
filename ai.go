package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"bastion/internal/ai"
	"bastion/internal/vault"
)

// validateAIEndpoint checks a user-supplied AI endpoint before it is stored and
// later POSTed to with the API key in an auth header. An empty endpoint is
// allowed (the client falls back to the provider's official URL). Otherwise the
// URL must be well-formed with a host, and the scheme must be https — except for
// loopback hosts, where plain http is allowed so local providers like Ollama
// (http://localhost:11434/v1) work. This stops a malformed or hostile endpoint
// from silently exfiltrating the key to an attacker-chosen, cleartext host.
func validateAIEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("bastion: invalid AI endpoint: %w", err)
	}
	if u.Host == "" {
		return fmt.Errorf("bastion: AI endpoint must include a host")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("bastion: AI endpoint must use https (http is allowed only for localhost)")
	default:
		return fmt.Errorf("bastion: AI endpoint scheme %q not allowed (use https)", u.Scheme)
	}
}

// ChatResult is returned by the Chat IPC method.
type ChatResult struct {
	Reply   string `json:"reply"`
	Command string `json:"command,omitempty"`
}

// AIErrorResult is returned by ExplainError.
type AIErrorResult struct {
	Explanation string `json:"explanation"`
	FixCommand  string `json:"fixCommand,omitempty"`
}

// AIConfigStatus reports whether AI is configured (no secrets exposed). It also
// carries the auto-explain-errors flag so the terminal can cheaply gate the
// automatic-egress path without fetching the full config.
type AIConfigStatus struct {
	Configured        bool `json:"configured"`
	AutoExplainErrors bool `json:"autoExplainErrors"`
}

// AIConfigView is the renderer-facing view of the AI config. Like HostDTO, it
// deliberately carries NO secret: the UI gets a HasKey flag so it can show
// "key set" without ever receiving the API key. The key travels renderer->Go
// only (via SetAIConfig) and never the other way.
type AIConfigView struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	Endpoint          string `json:"endpoint,omitempty"`
	SystemPrompt      string `json:"systemPrompt,omitempty"`
	HasKey            bool   `json:"hasKey"`
	AutoExplainErrors bool   `json:"autoExplainErrors"`
}

const metaAIConfig = "ai_config"

// readAIConfig decrypts and returns the stored AI config. Returns zero config
// and no error when no config has been saved yet.
func (a *App) readAIConfig() (ai.AIConfig, error) {
	raw, ok, err := a.store.GetMeta(metaAIConfig)
	if err != nil {
		return ai.AIConfig{}, fmt.Errorf("bastion: read ai config: %w", err)
	}
	if !ok {
		return ai.AIConfig{}, nil
	}

	key, err := a.keyCopy()
	if err != nil {
		return ai.AIConfig{}, err
	}
	defer zero(key)

	pt, err := vault.Decrypt(key, raw)
	if err != nil {
		return ai.AIConfig{}, fmt.Errorf("bastion: decrypt ai config: %w", err)
	}

	var cfg ai.AIConfig
	if err := json.Unmarshal(pt, &cfg); err != nil {
		return ai.AIConfig{}, fmt.Errorf("bastion: unmarshal ai config: %w", err)
	}
	return cfg, nil
}

// writeAIConfig encrypts and stores the AI config.
func (a *App) writeAIConfig(cfg ai.AIConfig) error {
	key, err := a.keyCopy()
	if err != nil {
		return err
	}
	defer zero(key)

	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("bastion: marshal ai config: %w", err)
	}

	ct, err := vault.Encrypt(key, raw)
	if err != nil {
		return fmt.Errorf("bastion: encrypt ai config: %w", err)
	}

	return a.store.SetMeta(metaAIConfig, ct)
}

// GetAIConfig returns the stored AI provider config as a key-free view. The
// API key is never sent to the renderer; the UI receives a HasKey flag instead
// (mirroring the host-credential boundary in hosts.go).
func (a *App) GetAIConfig() (AIConfigView, error) {
	if !a.IsUnlocked() {
		return AIConfigView{}, errLocked
	}
	cfg, err := a.readAIConfig()
	if err != nil {
		return AIConfigView{}, err
	}
	return AIConfigView{
		Provider:          cfg.Provider,
		Model:             cfg.Model,
		Endpoint:          cfg.Endpoint,
		SystemPrompt:      cfg.SystemPrompt,
		HasKey:            cfg.APIKey != "",
		AutoExplainErrors: cfg.AutoExplainErrors,
	}, nil
}

// SetAIConfig saves AI provider settings, encrypted in the vault. A blank
// apiKey means "keep the existing key" — the renderer never holds the key, so
// it cannot resend an unchanged one (same pattern as host passwords in
// hosts.go's encryptOrInherit).
func (a *App) SetAIConfig(provider, model, endpoint, apiKey, systemPrompt string, autoExplainErrors bool) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	defer a.touchAutoLock()

	if err := validateAIEndpoint(endpoint); err != nil {
		return err
	}

	if apiKey == "" {
		prev, err := a.readAIConfig()
		if err != nil {
			return err
		}
		apiKey = prev.APIKey
	}

	if err := a.writeAIConfig(ai.AIConfig{
		Provider:          provider,
		Model:             model,
		Endpoint:          endpoint,
		APIKey:            apiKey,
		SystemPrompt:      systemPrompt,
		AutoExplainErrors: autoExplainErrors,
	}); err != nil {
		return err
	}

	_ = a.initAIChat()
	return nil
}

// GetAIConfigStatus reports whether an AI config has been saved.
func (a *App) GetAIConfigStatus() (AIConfigStatus, error) {
	if !a.IsUnlocked() {
		return AIConfigStatus{}, errLocked
	}
	cfg, err := a.readAIConfig()
	if err != nil {
		return AIConfigStatus{}, err
	}
	return AIConfigStatus{
		Configured:        cfg.APIKey != "",
		AutoExplainErrors: cfg.AutoExplainErrors,
	}, nil
}

// NewChat creates a new AI chat session with conversation memory. Returns the
// session ID. The system prompt is loaded from the stored config; blank uses
// the default command-generation prompt.
func (a *App) NewChat() (string, error) {
	if !a.IsUnlocked() {
		return "", errLocked
	}
	defer a.touchAutoLock()

	cfg, err := a.readAIConfig()
	if err != nil {
		return "", err
	}
	if cfg.APIKey == "" {
		return "", fmt.Errorf("bastion: AI not configured")
	}

	sysPrompt := cfg.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = ai.SystemPromptCommand
	}

	if a.aiChat == nil {
		if err := a.initAIChat(); err != nil {
			return "", err
		}
	}

	return a.aiChat.NewSession(sysPrompt), nil
}

// Chat sends a message in an existing AI chat session and returns the reply
// with context from the conversation history.
func (a *App) Chat(chatID, message string) (ChatResult, error) {
	if !a.IsUnlocked() {
		return ChatResult{}, errLocked
	}
	defer a.touchAutoLock()

	if a.aiChat == nil {
		return ChatResult{}, fmt.Errorf("bastion: AI not configured")
	}

	result, err := a.aiChat.SendMessage(context.Background(), chatID, message)
	if err != nil {
		return ChatResult{}, fmt.Errorf("bastion: ai chat: %w", err)
	}

	return ChatResult{
		Reply:   result.Reply,
		Command: extractShellCommand(result.Reply),
	}, nil
}

// ClearChat clears the conversation history for an AI chat session.
func (a *App) ClearChat(chatID string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	defer a.touchAutoLock()

	if a.aiChat == nil {
		return fmt.Errorf("bastion: AI not configured")
	}

	return a.aiChat.ClearHistory(context.Background(), chatID)
}

// ExplainError sends recent terminal output containing an error to the AI for
// diagnosis.
func (a *App) ExplainError(sessionID, output string) (AIErrorResult, error) {
	if !a.IsUnlocked() {
		return AIErrorResult{}, errLocked
	}
	defer a.touchAutoLock()

	cfg, err := a.readAIConfig()
	if err != nil {
		return AIErrorResult{}, err
	}
	if cfg.APIKey == "" {
		return AIErrorResult{}, fmt.Errorf("bastion: AI not configured")
	}

	sysPrompt := cfg.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = ai.SystemPromptError
	}

	messages := []ai.Message{
		{Role: "user", Content: output},
	}

	reply, err := ai.Chat(cfg, messages, sysPrompt)
	if err != nil {
		return AIErrorResult{}, fmt.Errorf("bastion: ai error: %w", err)
	}

	result := AIErrorResult{Explanation: reply}
	result.FixCommand = extractShellCommand(reply)
	return result, nil
}

// TestAIConnection verifies that the stored AI config works.
func (a *App) TestAIConnection() error {
	if !a.IsUnlocked() {
		return errLocked
	}
	defer a.touchAutoLock()

	cfg, err := a.readAIConfig()
	if err != nil {
		return err
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("bastion: AI not configured")
	}

	messages := []ai.Message{
		{Role: "user", Content: "Reply with exactly the word 'ok'."},
	}
	_, err = ai.Chat(cfg, messages, "")
	return err
}

// extractShellCommand looks for a fenced code block with shell language tag
// in the AI response and returns its content. Returns empty if none found.
func extractShellCommand(text string) string {
	const marker = "```shell\n"
	start := strings.Index(text, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.Index(text[start:], "```")
	if end < 0 {
		return ""
	}
	return text[start : start+end]
}
