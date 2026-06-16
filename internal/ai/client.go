package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const requestTimeout = 30 * time.Second

// AIConfig holds the user's AI provider settings, encrypted in vault_meta.
type AIConfig struct {
	Provider      string `json:"provider"`                 // "openai", "anthropic", "openai-compatible"
	Model         string `json:"model"`
	Endpoint      string `json:"endpoint,omitempty"`       // custom endpoint for openai-compatible
	APIKey        string `json:"apiKey"`
	SystemPrompt  string `json:"systemPrompt,omitempty"`  // custom system prompt; empty = use default
}

// CompletionRequest is the payload for the chat completion endpoint.
type CompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Message is a single turn in the chat.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// completionResponse is the response from the LLM API.
type completionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// anthropicRequest is the payload for Anthropic's Messages API.
type anthropicRequest struct {
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens"`
	Messages    []anthropicMsg   `json:"messages"`
	System      string           `json:"system,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Chat calls the configured LLM with a list of messages and returns the
// response text. Supports OpenAI, Anthropic, and OpenAI-compatible APIs.
func Chat(cfg AIConfig, messages []Message, systemPrompt string) (string, error) {
	switch cfg.Provider {
	case "openai", "openai-compatible":
		return chatOpenAI(cfg, messages, systemPrompt)
	case "anthropic":
		return chatAnthropic(cfg, messages, systemPrompt)
	default:
		return "", fmt.Errorf("ai: unknown provider %q", cfg.Provider)
	}
}

func chatOpenAI(cfg AIConfig, messages []Message, systemPrompt string) (string, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	url := endpoint + "/chat/completions"

	msgs := messages
	if systemPrompt != "" {
		msgs = append([]Message{{Role: "system", Content: systemPrompt}}, msgs...)
	}
	req := CompletionRequest{
		Model:    cfg.Model,
		Messages: msgs,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("ai: marshal: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ai: request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai: read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ai: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var cr completionResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("ai: unmarshal: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("ai: no choices in response")
	}
	return cr.Choices[0].Message.Content, nil
}

func chatAnthropic(cfg AIConfig, messages []Message, systemPrompt string) (string, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1"
	}
	url := endpoint + "/messages"

	msgs := make([]anthropicMsg, len(messages))
	for i, m := range messages {
		msgs[i] = anthropicMsg{Role: m.Role, Content: m.Content}
	}

	req := anthropicRequest{
		Model:       cfg.Model,
		MaxTokens:   1024,
		Messages:    msgs,
		System:      systemPrompt,
		Temperature: 0.3,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("ai: marshal: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ai: request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai: read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ai: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var ar anthropicResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return "", fmt.Errorf("ai: unmarshal: %w", err)
	}
	if len(ar.Content) == 0 {
		return "", fmt.Errorf("ai: no content in response")
	}
	return ar.Content[0].Text, nil
}
