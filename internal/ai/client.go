package ai

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/openai"
)

// AIConfig holds the user's AI provider settings, encrypted in vault_meta.
type AIConfig struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Endpoint     string `json:"endpoint,omitempty"`
	APIKey       string `json:"apiKey"`
	SystemPrompt string `json:"systemPrompt,omitempty"`
	AutoExplainErrors bool `json:"autoExplainErrors,omitempty"`
}

// Message is a single turn in the chat.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// newLLM creates a LangChain llms.Model from the given config.
func newLLM(cfg AIConfig) (llms.Model, error) {
	switch cfg.Provider {
	case "openai":
		opts := []openai.Option{
			openai.WithToken(cfg.APIKey),
			openai.WithModel(cfg.Model),
		}
		if cfg.Endpoint != "" {
			opts = append(opts, openai.WithBaseURL(cfg.Endpoint))
		}
		return openai.New(opts...)
	case "anthropic":
		opts := []anthropic.Option{
			anthropic.WithToken(cfg.APIKey),
			anthropic.WithModel(cfg.Model),
		}
		return anthropic.New(opts...)
	case "openai-compatible":
		opts := []openai.Option{
			openai.WithToken(cfg.APIKey),
			openai.WithModel(cfg.Model),
		}
		if cfg.Endpoint != "" {
			opts = append(opts, openai.WithBaseURL(cfg.Endpoint))
		} else {
			opts = append(opts, openai.WithBaseURL("http://localhost:11434/v1"))
		}
		return openai.New(opts...)
	default:
		return nil, fmt.Errorf("ai: unknown provider %q", cfg.Provider)
	}
}

// Chat calls the configured LLM with a list of messages and returns the
// response text. This is a stateless helper used by ExplainError and
// TestAIConnection. For stateful conversations use SessionManager.
func Chat(cfg AIConfig, messages []Message, systemPrompt string) (string, error) {
	llm, err := newLLM(cfg)
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	msgContent := make([]llms.MessageContent, 0, len(messages)+1)

	if systemPrompt != "" {
		msgContent = append(msgContent, llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt))
	}

	for _, m := range messages {
		switch m.Role {
		case "user":
			msgContent = append(msgContent, llms.TextParts(llms.ChatMessageTypeHuman, m.Content))
		case "assistant":
			msgContent = append(msgContent, llms.TextParts(llms.ChatMessageTypeAI, m.Content))
		case "system":
			msgContent = append(msgContent, llms.TextParts(llms.ChatMessageTypeSystem, m.Content))
		}
	}

	resp, err := llm.GenerateContent(ctx, msgContent)
	if err != nil {
		return "", fmt.Errorf("ai: chat: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("ai: no response choices")
	}

	return resp.Choices[0].Content, nil
}
