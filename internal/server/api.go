package server

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
)

// ── Health ──────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": s.version,
	})
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
