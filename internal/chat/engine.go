// Package chat — Chat engine: turn management, streaming, and session state.
package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// Engine manages multi-turn chat conversations with an LLM provider.
// It holds the active provider, current session, and config for prompt assembly.
type Engine struct {
	provider provider.Provider
	session  *Session
	config   *config.Config

	mu sync.RWMutex
}

// NewEngine creates a chat engine with the given provider and config.
// Does NOT create a session — call NewSession() before sending messages.
func NewEngine(p provider.Provider, cfg *config.Config) *Engine {
	return &Engine{
		provider: p,
		config:   cfg,
	}
}

// NewSession creates a new chat session and returns its ID.
// Any existing session is replaced (caller should persist it first if needed).
func (e *Engine) NewSession() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.session = NewSession()
	return e.session.ID
}

// GetSession returns the current session, or nil if none exists.
func (e *Engine) GetSession() *Session {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.session
}

// GetHistory returns the message history for the current session.
// Returns nil if no session exists.
func (e *Engine) GetHistory() []provider.ChatMessage {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.session == nil {
		return nil
	}
	// Return a copy to avoid data races.
	e.session.mu.Lock()
	defer e.session.mu.Unlock()
	msgs := make([]provider.ChatMessage, len(e.session.Messages))
	copy(msgs, e.session.Messages)
	return msgs
}

// SetProvider switches the active LLM provider.
// The current session (if any) is preserved — only future calls use the new provider.
func (e *Engine) SetProvider(p provider.Provider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.provider = p
}

// SendMessage sends a user message and returns a channel of streaming events.
// The goroutine behind the channel accumulates the assistant's response and
// adds it to session history when the "done" event arrives.
//
// Flow:
//  1. Add user message to session history
//  2. Build system prompt from config
//  3. Create ChatRequest with system prompt + full message history
//  4. Call provider.StreamChat
//  5. Wrap returned channel: forward events, accumulate deltas, finalize on "done"
func (e *Engine) SendMessage(ctx context.Context, content string) (<-chan provider.StreamEvent, error) {
	e.mu.RLock()
	session := e.session
	p := e.provider
	cfg := e.config
	e.mu.RUnlock()

	if session == nil {
		return nil, fmt.Errorf("chat: no active session — call NewSession() first")
	}

	// 1. Add user message to session history.
	session.AddMessage("user", content)

	// 2. Build system prompt from companion config.
	systemPrompt := BuildSystemPrompt(
		cfg.Companion.Name,
		cfg.Companion.FocusAreas,
		cfg.Companion.CustomInstructions,
	)

	// 3. Build the chat request with full history.
	session.mu.Lock()
	messages := make([]provider.ChatMessage, len(session.Messages))
	copy(messages, session.Messages)
	session.mu.Unlock()

	req := provider.ChatRequest{
		SystemPrompt: systemPrompt,
		Messages:     messages,
	}

	// 4. Call provider.
	sourceCh, err := p.StreamChat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chat: stream failed: %w", err)
	}

	// 5. Wrap the source channel: accumulate deltas, finalize on done.
	outCh := make(chan provider.StreamEvent, 64)
	go e.relayStream(sourceCh, outCh, session)

	return outCh, nil
}

// relayStream forwards events from the provider channel to the output channel.
// It accumulates text deltas into the full assistant response and adds it to
// session history when the "done" event arrives.
func (e *Engine) relayStream(source <-chan provider.StreamEvent, out chan<- provider.StreamEvent, session *Session) {
	defer close(out)

	var response strings.Builder

	for event := range source {
		switch event.Type {
		case "delta":
			response.WriteString(event.Delta)
			out <- event

		case "done":
			// Add the accumulated assistant response to session history.
			if response.Len() > 0 {
				session.AddMessage("assistant", response.String())
			}
			// Track token usage.
			session.AddUsage(event.Usage)
			out <- event

		case "error":
			out <- event
			return

		default:
			// Forward unknown event types as-is.
			out <- event
		}
	}
}
