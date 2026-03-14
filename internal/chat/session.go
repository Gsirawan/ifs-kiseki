// Package chat — Session: in-memory state for a single chat conversation.
package chat

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

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

// NewSession creates a fresh session with a UUID and current timestamp.
func NewSession() *Session {
	return &Session{
		ID:        generateUUID(),
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
func (s *Session) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
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

// generateUUID produces a v4 UUID using crypto/rand. No external dependencies.
func generateUUID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	// Set version 4 (bits 12-15 of time_hi_and_version).
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant bits (bits 6-7 of clock_seq_hi_and_reserved).
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
