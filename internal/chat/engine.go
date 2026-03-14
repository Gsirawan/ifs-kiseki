// Package chat — Chat engine: turn management, streaming, and session state.
package chat

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/memory"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// Engine manages multi-turn chat conversations with an LLM provider.
// It holds the active provider, current session, and config for prompt assembly.
type Engine struct {
	provider    provider.Provider
	session     *Session
	config      *config.Config
	memoryStore memory.Store // optional — nil means auto-save is disabled

	mu sync.RWMutex
}

// NewEngine creates a chat engine with the given provider, config, and optional
// memory store. Pass nil for memoryStore to disable automatic session saving.
func NewEngine(p provider.Provider, cfg *config.Config, memoryStore memory.Store) *Engine {
	return &Engine{
		provider:    p,
		config:      cfg,
		memoryStore: memoryStore,
	}
}

// NewSession creates a new chat session and returns its ID.
// If a session is already active, it is ended and saved to memory before
// the new session begins.
func (e *Engine) NewSession() string {
	// Capture and clear the old session under the write lock, then release
	// before calling EndSession — EndSession acquires the lock internally and
	// we must not hold it while doing I/O.
	e.mu.Lock()
	old := e.session
	e.session = nil
	e.mu.Unlock()

	// Save the old session outside the lock.
	if old != nil {
		if err := e.endAndSave(old); err != nil {
			log.Printf("[engine] EndSession (on NewSession): %v", err)
		}
	}

	// Create the new session.
	e.mu.Lock()
	defer e.mu.Unlock()
	e.session = newSession()
	return e.session.ID
}

// EndSession ends the current session and asynchronously saves it to memory.
// Returns nil if there is no active session or no memory store is configured.
// Safe to call multiple times — a session is only saved once (End() is idempotent
// in that it only sets EndedAt on the first call).
func (e *Engine) EndSession() error {
	e.mu.Lock()
	sess := e.session
	e.mu.Unlock()

	if sess == nil || e.memoryStore == nil {
		return nil
	}

	return e.endAndSave(sess)
}

// endAndSave marks the session as ended and fires an async goroutine to persist
// it. The goroutine uses context.Background() because the caller's context
// (e.g. the WebSocket request context) may already be cancelled at this point.
// Respects config.Memory.AutoSave — if false, the session is ended but not saved.
func (e *Engine) endAndSave(sess *Session) error {
	// Mark the session as ended (idempotent — End() only sets EndedAt once).
	sess.End()

	if e.memoryStore == nil {
		return nil
	}

	// Respect the auto_save config flag.
	if e.config != nil && !e.config.Memory.AutoSave {
		return nil
	}

	// Snapshot the session data under its own lock before handing off to the
	// goroutine — avoids any race if the session is somehow reused.
	sess.mu.Lock()
	msgs := make([]provider.ChatMessage, len(sess.Messages))
	copy(msgs, sess.Messages)
	memSess := memory.Session{
		ID:        sess.ID,
		Messages:  msgs,
		StartedAt: sess.StartedAt,
	}
	if sess.EndedAt != nil {
		memSess.EndedAt = *sess.EndedAt
	}
	memSess.Summary = sess.Summary
	sess.mu.Unlock()

	store := e.memoryStore // capture for goroutine

	go func() {
		ctx := context.Background()
		if err := store.SaveSession(ctx, memSess); err != nil {
			log.Printf("[engine] async SaveSession failed (session %s): %v", memSess.ID, err)
			return
		}
		log.Printf("[engine] session %s saved to memory (%d messages)", memSess.ID, len(memSess.Messages))
	}()

	return nil
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
//  3. Search memory for relevant context and inject it into the system prompt
//  4. Create ChatRequest with system prompt + full message history
//  5. Call provider.StreamChat
//  6. Wrap returned channel: forward events, accumulate deltas, finalize on "done"
func (e *Engine) SendMessage(ctx context.Context, content string) (<-chan provider.StreamEvent, error) {
	e.mu.RLock()
	session := e.session
	p := e.provider
	cfg := e.config
	store := e.memoryStore
	e.mu.RUnlock()

	if session == nil {
		return nil, fmt.Errorf("chat: no active session — call NewSession() first")
	}

	// 1. Add user message to session history.
	session.AddMessage("user", content)

	// 2. Build system prompt from companion config.
	systemPrompt := BuildSystemPrompt(
		cfg.Companion.Name,
		cfg.Companion.UserName,
		cfg.Companion.FocusAreas,
		cfg.Companion.CustomInstructions,
	)

	// 3. Search memory for relevant context and inject it into the system prompt.
	// This is synchronous — the embed+vec search is ~100ms and the prompt needs
	// the results before the LLM call. Failures are non-fatal: we log and continue
	// with the unaugmented prompt.
	if store != nil {
		limit := cfg.Memory.MaxContextChunks
		if limit <= 0 {
			limit = 5
		}
		memories, err := store.Search(ctx, content, limit)
		if err != nil {
			log.Printf("[engine] memory search failed (non-fatal): %v", err)
		} else {
			systemPrompt = InjectMemoryContext(systemPrompt, memories)
		}
	}

	// 4. Build the chat request with full history.
	session.mu.Lock()
	messages := make([]provider.ChatMessage, len(session.Messages))
	copy(messages, session.Messages)
	session.mu.Unlock()

	req := provider.ChatRequest{
		SystemPrompt: systemPrompt,
		Messages:     messages,
	}

	// 5. Call provider.
	sourceCh, err := p.StreamChat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chat: stream failed: %w", err)
	}

	// 6. Wrap the source channel: accumulate deltas, finalize on done.
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
