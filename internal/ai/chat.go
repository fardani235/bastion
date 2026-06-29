package ai

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/prompts"
)

const _conversationTemplate = `{{.systemPrompt}}

Current conversation:
{{.history}}
Human: {{.input}}
AI:`

// ChatResult is returned by SessionManager.SendMessage.
type ChatResult struct {
	Reply string
}

// ChatSession holds one conversation chain with its memory and system prompt.
type ChatSession struct {
	ID           string
	chain        chains.LLMChain
	mem          *memory.ConversationBuffer
	systemPrompt string
}

// SessionManager manages multiple chat sessions, each with independent
// conversation history. Created per vault unlock; destroyed on lock.
type SessionManager struct {
	mu   sync.RWMutex
	llm  llms.Model
	sess map[string]*ChatSession
}

// NewSessionManager creates a manager using the given LLM model.
func NewSessionManager(model llms.Model) *SessionManager {
	return &SessionManager{
		llm:  model,
		sess: make(map[string]*ChatSession),
	}
}

// NewSessionManagerFromConfig creates a session manager from an AIConfig.
func NewSessionManagerFromConfig(cfg AIConfig) (*SessionManager, error) {
	model, err := newLLM(cfg)
	if err != nil {
		return nil, err
	}
	return NewSessionManager(model), nil
}

// NewSession creates a new chat session with the given system prompt.
// Returns the session ID.
func (m *SessionManager) NewSession(systemPrompt string) string {
	prompt := prompts.NewPromptTemplate(
		_conversationTemplate,
		[]string{"systemPrompt", "history", "input"},
	)
	chain := chains.NewLLMChain(m.llm, prompt)

	chatHistory := memory.NewChatMessageHistory()
	mem := memory.NewConversationBuffer(
		memory.WithChatHistory(chatHistory),
		memory.WithInputKey("input"),
		memory.WithOutputKey("text"),
	)
	chain.Memory = mem

	id := uuid.New().String()
	sess := &ChatSession{
		ID:           id,
		chain:        *chain,
		mem:          mem,
		systemPrompt: systemPrompt,
	}

	m.mu.Lock()
	m.sess[id] = sess
	m.mu.Unlock()

	return id
}

// SendMessage sends a user message to the session and returns the AI reply.
// The systemPrompt is baked in at session creation — this only passes the
// user message.
func (m *SessionManager) SendMessage(ctx context.Context, sessionID, message string) (ChatResult, error) {
	m.mu.RLock()
	sess, ok := m.sess[sessionID]
	m.mu.RUnlock()

	if !ok {
		return ChatResult{}, fmt.Errorf("ai: session %q not found", sessionID)
	}

	output, err := chains.Call(ctx, &sess.chain, map[string]any{
		"input":        message,
		"systemPrompt": sess.systemPrompt,
	})
	if err != nil {
		return ChatResult{}, fmt.Errorf("ai: chat: %w", err)
	}

	reply, _ := output["text"].(string)
	return ChatResult{Reply: reply}, nil
}

// ClearHistory clears the conversation history for a session.
func (m *SessionManager) ClearHistory(ctx context.Context, sessionID string) error {
	m.mu.RLock()
	sess, ok := m.sess[sessionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("ai: session %q not found", sessionID)
	}
	return sess.mem.Clear(ctx)
}

// DeleteSession removes a session and its memory.
func (m *SessionManager) DeleteSession(sessionID string) {
	m.mu.Lock()
	delete(m.sess, sessionID)
	m.mu.Unlock()
}

// Destroy clears all sessions.
func (m *SessionManager) Destroy() {
	m.mu.Lock()
	m.sess = make(map[string]*ChatSession)
	m.mu.Unlock()
}

// Len returns the number of active sessions.
func (m *SessionManager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sess)
}

