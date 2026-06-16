package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"bastion/internal/ai"
	"bastion/internal/vault"
)

// AICommandResult is returned by GenerateCommand.
type AICommandResult struct {
	Command     string `json:"command"`
	Explanation string `json:"explanation"`
}

// AIErrorResult is returned by ExplainError.
type AIErrorResult struct {
	Explanation string `json:"explanation"`
	FixCommand  string `json:"fixCommand,omitempty"`
}

// AIConfigStatus reports whether AI is configured (no secrets exposed).
type AIConfigStatus struct {
	Configured bool `json:"configured"`
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

// GetAIConfig returns the stored AI provider config (without secrets in log).
func (a *App) GetAIConfig() (ai.AIConfig, error) {
	if !a.IsUnlocked() {
		return ai.AIConfig{}, errLocked
	}
	return a.readAIConfig()
}

// SetAIConfig saves AI provider settings, encrypted in the vault.
func (a *App) SetAIConfig(provider, model, endpoint, apiKey, systemPrompt string) error {
	if !a.IsUnlocked() {
		return errLocked
	}
	defer a.touchAutoLock()

	return a.writeAIConfig(ai.AIConfig{
		Provider:     provider,
		Model:        model,
		Endpoint:     endpoint,
		APIKey:       apiKey,
		SystemPrompt: systemPrompt,
	})
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
	return AIConfigStatus{Configured: cfg.APIKey != ""}, nil
}

// GenerateCommand sends a natural-language prompt to the AI and returns a shell
// command. sessionID is used for context (host label available from the session
// manager, but unused here — the caller can include it in the prompt).
func (a *App) GenerateCommand(sessionID, prompt string) (AICommandResult, error) {
	if !a.IsUnlocked() {
		return AICommandResult{}, errLocked
	}
	defer a.touchAutoLock()

	cfg, err := a.readAIConfig()
	if err != nil {
		return AICommandResult{}, err
	}
	if cfg.APIKey == "" {
		return AICommandResult{}, fmt.Errorf("bastion: AI not configured")
	}

	sysPrompt := cfg.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = ai.SystemPromptCommand
	}

	messages := []ai.Message{
		{Role: "user", Content: prompt},
	}

	reply, err := ai.Chat(cfg, messages, sysPrompt)
	if err != nil {
		return AICommandResult{}, fmt.Errorf("bastion: ai command: %w", err)
	}

	result := AICommandResult{Explanation: reply}
	// Try to extract a shell command from the reply. The prompt instructs the
	// AI to wrap commands in ```shell ... ```. We extract it for the UI.
	result.Command = extractShellCommand(reply)
	return result, nil
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
