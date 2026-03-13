// Package memory implements the simplified Kiseki memory store for IFS-Kiseki.
// Three operations only: save session, search, generate briefing.
package memory

import (
	"context"
	"time"

	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// Store is the memory interface for IFS-Kiseki.
// Intentionally minimal — NOT the full 16-tool Kiseki suite.
type Store interface {
	// SaveSession persists a completed chat session.
	// Chunks the conversation, embeds it, stores in SQLite+Vec.
	SaveSession(ctx context.Context, session Session) error

	// Search performs semantic search over stored sessions.
	// Returns relevant conversation fragments for context injection.
	Search(ctx context.Context, query string, limit int) ([]Result, error)

	// GenerateBriefing creates a session-start briefing from recent memory.
	// Uses the LLM to summarize recent sessions, key themes, and parts identified.
	GenerateBriefing(ctx context.Context, p provider.Provider) (string, error)
}

// Session represents a completed chat session.
type Session struct {
	ID        string
	Messages  []provider.ChatMessage
	StartedAt time.Time
	EndedAt   time.Time
	Summary   string // auto-generated after session ends
}

// Result is a search hit from memory.
type Result struct {
	Text      string    `json:"text"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Distance  float64   `json:"distance"`
}
