package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// ── Health ──────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": s.version,
	})
}

// ── Briefing ─────────────────────────────────────────────────────

// briefingResponse is the JSON shape for the briefing endpoint.
type briefingResponse struct {
	Briefing string `json:"briefing"`
}

// handleBriefing handles GET /api/briefing.
//
// It generates a warm, contextual session-start briefing by asking the LLM
// to summarise recent sessions from memory. Two graceful-degradation cases:
//   - No memory store: returns a static welcome message (first-run experience).
//   - No provider: returns 503 — briefing requires an LLM.
func (s *Server) handleBriefing(w http.ResponseWriter, r *http.Request) {
	// No memory store — first-run or memory disabled.
	if s.memoryStore == nil {
		writeJSON(w, http.StatusOK, briefingResponse{
			Briefing: "Welcome! This is your first session.",
		})
		return
	}

	// Briefing requires an LLM provider.
	if s.provider == nil {
		writeError(w, http.StatusServiceUnavailable, "no LLM provider configured — set an API key in settings")
		return
	}

	briefing, err := s.memoryStore.GenerateBriefing(r.Context(), s.provider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate briefing")
		return
	}

	writeJSON(w, http.StatusOK, briefingResponse{Briefing: briefing})
}

// ── Sessions ────────────────────────────────────────────────────

// sessionRow is the JSON shape for a session list entry.
type sessionRow struct {
	ID        string  `json:"id"`
	StartedAt int64   `json:"started_at"`
	EndedAt   *int64  `json:"ended_at,omitempty"`
	Summary   *string `json:"summary,omitempty"`
}

// messageRow is the JSON shape for a single message.
type messageRow struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// sessionDetail is the JSON shape for a single session with messages.
type sessionDetail struct {
	sessionRow
	Messages []messageRow `json:"messages"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, started_at, ended_at, summary FROM sessions ORDER BY started_at DESC`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query sessions")
		return
	}
	defer rows.Close()

	sessions := make([]sessionRow, 0)
	for rows.Next() {
		var sr sessionRow
		if err := rows.Scan(&sr.ID, &sr.StartedAt, &sr.EndedAt, &sr.Summary); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan session")
			return
		}
		sessions = append(sessions, sr)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to iterate sessions")
		return
	}

	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	// Fetch session.
	var sr sessionRow
	err := s.db.QueryRowContext(r.Context(),
		`SELECT id, started_at, ended_at, summary FROM sessions WHERE id = ?`, id,
	).Scan(&sr.ID, &sr.StartedAt, &sr.EndedAt, &sr.Summary)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query session")
		return
	}

	// Fetch messages.
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, role, content, timestamp FROM messages WHERE session_id = ? ORDER BY timestamp`, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query messages")
		return
	}
	defer rows.Close()

	messages := make([]messageRow, 0)
	for rows.Next() {
		var mr messageRow
		if err := rows.Scan(&mr.ID, &mr.Role, &mr.Content, &mr.Timestamp); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to scan message")
			return
		}
		messages = append(messages, mr)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to iterate messages")
		return
	}

	writeJSON(w, http.StatusOK, sessionDetail{
		sessionRow: sr,
		Messages:   messages,
	})
}

func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	// Check session exists.
	var exists string
	err := s.db.QueryRowContext(r.Context(),
		`SELECT id FROM sessions WHERE id = ?`, id,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query session")
		return
	}

	// Delete in order: vec_messages (embeddings) → messages (FK) → session.
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	// Clean up vector embeddings for this session's messages.
	if _, err := tx.ExecContext(r.Context(),
		`DELETE FROM vec_messages WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete embeddings")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM messages WHERE session_id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete messages")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM sessions WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit deletion")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ── Settings ────────────────────────────────────────────────────

// redactKey returns the last 4 characters of a key prefixed with "****",
// or an empty string if the key is empty.
func redactKey(key string) string {
	if len(key) <= 4 {
		if key == "" {
			return ""
		}
		return "****"
	}
	return "****" + key[len(key)-4:]
}

// redactConfig returns a copy of the config with API keys redacted.
func redactConfig(cfg *config.Config) *config.Config {
	// Shallow copy the config, then deep-copy the providers.
	out := *cfg
	out.Providers = config.ProvidersConfig{
		Claude: cfg.Providers.Claude,
		Grok:   cfg.Providers.Grok,
	}
	out.Providers.Claude.APIKey = redactKey(cfg.Providers.Claude.APIKey)
	out.Providers.Grok.APIKey = redactKey(cfg.Providers.Grok.APIKey)
	return &out
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, redactConfig(s.cfg))
}

func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var incoming config.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Preserve API keys if the incoming value is redacted or empty.
	// Users cannot set keys via this endpoint — use env vars or edit config.json.
	if incoming.Providers.Claude.APIKey == "" || incoming.Providers.Claude.APIKey == redactKey(s.cfg.Providers.Claude.APIKey) {
		incoming.Providers.Claude.APIKey = s.cfg.Providers.Claude.APIKey
	}
	if incoming.Providers.Grok.APIKey == "" || incoming.Providers.Grok.APIKey == redactKey(s.cfg.Providers.Grok.APIKey) {
		incoming.Providers.Grok.APIKey = s.cfg.Providers.Grok.APIKey
	}

	// Save to disk.
	if err := config.Save(&incoming); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	// Update in-memory config.
	*s.cfg = incoming

	writeJSON(w, http.StatusOK, redactConfig(s.cfg))
}

// ── Providers ───────────────────────────────────────────────────

// providerInfo is the JSON shape for a provider in the list.
type providerInfo struct {
	Name   string `json:"name"`
	Model  string `json:"model"`
	HasKey bool   `json:"has_key"`
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	providers := []providerInfo{
		{
			Name:   "claude",
			Model:  s.cfg.Providers.Claude.Model,
			HasKey: s.cfg.Providers.Claude.APIKey != "",
		},
		{
			Name:   "grok",
			Model:  s.cfg.Providers.Grok.Model,
			HasKey: s.cfg.Providers.Grok.APIKey != "",
		},
	}
	writeJSON(w, http.StatusOK, providers)
}

// ── Onboarding ───────────────────────────────────────────────────

// handleAcceptDisclaimer handles POST /api/accept-disclaimer.
//
// Records that the user has read and accepted the disclaimer. Sets
// DisclaimerAccepted=true and DisclaimerAcceptedAt to the current time,
// then persists the config to disk.
func (s *Server) handleAcceptDisclaimer(w http.ResponseWriter, r *http.Request) {
	s.cfg.DisclaimerAccepted = true
	s.cfg.DisclaimerAcceptedAt = time.Now().Format(time.RFC3339)

	if err := config.Save(s.cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save disclaimer acceptance")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
}

// testProviderRequest is the JSON body for POST /api/test-provider.
type testProviderRequest struct {
	Provider string `json:"provider"` // "claude" or "grok"
	APIKey   string `json:"api_key"`
}

// testProviderResponse is the JSON response for POST /api/test-provider.
type testProviderResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// handleTestProvider handles POST /api/test-provider.
//
// Creates a temporary provider instance with the supplied API key and sends
// a minimal test message ("Say OK", max_tokens=10) to verify the key is valid.
// The temporary provider is discarded after the test — it does not affect the
// running server's active provider.
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	var req testProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Provider = strings.TrimSpace(strings.ToLower(req.Provider))
	req.APIKey = strings.TrimSpace(req.APIKey)

	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	// Resolve the provider entry from config, then override the API key with
	// the one supplied by the user (they may not have saved it yet).
	var entry config.ProviderEntry
	switch req.Provider {
	case "claude":
		entry = s.cfg.Providers.Claude
	case "grok":
		entry = s.cfg.Providers.Grok
	default:
		writeError(w, http.StatusBadRequest, "unknown provider — must be 'claude' or 'grok'")
		return
	}
	entry.APIKey = req.APIKey

	// Create a temporary provider instance. Max tokens capped at 10 — we only
	// need a single word response to confirm the key is valid.
	entry.MaxTokens = 10

	p, err := provider.NewFromConfig(req.Provider, entry)
	if err != nil {
		writeJSON(w, http.StatusOK, testProviderResponse{
			Success: false,
			Error:   "failed to create provider: " + err.Error(),
		})
		return
	}

	// Send a minimal test message with a short timeout.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	chatReq := provider.ChatRequest{
		Messages: []provider.ChatMessage{
			{Role: "user", Content: "Say OK"},
		},
		MaxTokens: 10,
	}

	ch, err := p.StreamChat(ctx, chatReq)
	if err != nil {
		// Classify common error patterns into user-friendly messages.
		writeJSON(w, http.StatusOK, testProviderResponse{
			Success: false,
			Error:   classifyProviderError(err),
		})
		return
	}

	// Drain the channel — we only care whether the call succeeds, not the content.
	for event := range ch {
		if event.Type == "error" {
			writeJSON(w, http.StatusOK, testProviderResponse{
				Success: false,
				Error:   classifyProviderError(event.Error),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, testProviderResponse{Success: true})
}

// classifyProviderError converts a raw provider error into a user-friendly message.
// The raw errors contain API-specific details that are not meaningful to end users.
func classifyProviderError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "invalid x-api-key") || strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "authentication"):
		return "Invalid API key — please check and try again."
	case strings.Contains(msg, "403") || strings.Contains(msg, "forbidden"):
		return "Access denied — your API key may not have the required permissions."
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		return "Rate limit reached — please wait a moment and try again."
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline"):
		return "Connection timed out — please check your internet connection and try again."
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "network"):
		return "Could not reach the API — please check your internet connection."
	default:
		return "Connection failed — please verify your API key and try again."
	}
}
