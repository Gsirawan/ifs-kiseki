// Package chat — Session: in-memory state for a single chat conversation.
package chat

import (
	"sync"
	"time"

	"github.com/Gsirawan/ifs-kiseki/internal/id"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// Session holds the state for a single chat conversation.
// It is the live, in-memory session — distinct from memory.Session which is
// the persisted form used for search and briefing.
type Session struct {
	ID        string
	Messages  []provider.ChatMessage
	StartedAt time.Time
	EndedAt   *time.Time
	Summary   string
	Usage     UsageTotal

	mu sync.Mutex
}

// UsageTotal tracks cumulative token usage across all turns in a session.
type UsageTotal struct {
	InputTokens  int
	OutputTokens int
	Turns        int
}

// newSession creates a fresh session with a UUID and current timestamp.
// Unexported — callers outside this package go through Engine.NewSession().
func newSession() *Session {
	return &Session{
		ID:        id.New(),
		Messages:  make([]provider.ChatMessage, 0),
		StartedAt: time.Now(),
	}
}

// AddMessage appends a message to the session history.
// Thread-safe.
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, provider.ChatMessage{
		Role:    role,
		Content: content,
	})
}

// End marks the session as ended with the current timestamp.
// Idempotent — only sets EndedAt on the first call.
func (s *Session) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.EndedAt != nil {
		return // already ended
	}
	now := time.Now()
	s.EndedAt = &now
}

// AddUsage accumulates token usage from a single response.
// Thread-safe.
func (s *Session) AddUsage(u *provider.Usage) {
	if u == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Usage.InputTokens += u.InputTokens
	s.Usage.OutputTokens += u.OutputTokens
	s.Usage.Turns++
}
