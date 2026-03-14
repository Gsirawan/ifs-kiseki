// Package memory — SQLiteStore: the memory engine for IFS-Kiseki.
// Implements the Store interface: save sessions, search by vector similarity,
// generate session briefings via the LLM.
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"

	"github.com/Gsirawan/ifs-kiseki/internal/embedder"
	"github.com/Gsirawan/ifs-kiseki/internal/id"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// Compile-time check: *SQLiteStore implements Store.
var _ Store = (*SQLiteStore)(nil)

// SQLiteStore is the SQLite+Vec implementation of the Store interface.
// It persists sessions and their messages, embeds message content for
// vector similarity search, and generates LLM briefings from recent history.
type SQLiteStore struct {
	db      *sql.DB
	embeddr embedder.Embedder // nil means embeddings are disabled (graceful degradation)
}

// NewSQLiteStore creates a SQLiteStore backed by the given database and embedder.
// embedder may be nil — in that case, SaveSession still persists sessions and
// messages, but skips embedding. Search returns empty results when embedder is nil.
func NewSQLiteStore(db *sql.DB, emb embedder.Embedder) *SQLiteStore {
	return &SQLiteStore{
		db:      db,
		embeddr: emb,
	}
}

// SaveSession persists a completed chat session to SQLite.
//
// Flow:
//  1. Insert session metadata into the sessions table.
//  2. For each message: insert into messages, then embed + insert into vec_messages.
//  3. Embedding failures are logged and skipped — the session is always saved.
//  4. The entire operation runs inside a single transaction for atomicity.
func (s *SQLiteStore) SaveSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return fmt.Errorf("memory: SaveSession: session ID must not be empty")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory: SaveSession: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Insert session metadata.
	createdAt := time.Now().UTC().Format(time.RFC3339)
	var endedAtVal interface{}
	if !session.EndedAt.IsZero() {
		endedAtVal = session.EndedAt.UnixMilli()
	}

	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO sessions (id, started_at, ended_at, summary, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		session.ID,
		session.StartedAt.UnixMilli(),
		endedAtVal,
		nullableString(session.Summary),
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("memory: SaveSession: insert session: %w", err)
	}

	// 2. Insert each message.
	msgStmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO messages (id, session_id, role, content, timestamp)
		 VALUES (?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("memory: SaveSession: prepare message stmt: %w", err)
	}
	defer msgStmt.Close()

	// Collect messages that were actually inserted (new rows) for embedding.
	type pendingEmbed struct {
		id      string
		content string
	}
	var toEmbed []pendingEmbed

	for _, msg := range session.Messages {
		if msg.Content == "" {
			continue
		}
		msgID := id.New()
		ts := time.Now().UnixMilli() // messages don't carry their own timestamp in ChatMessage

		res, err := msgStmt.ExecContext(ctx, msgID, session.ID, msg.Role, msg.Content, ts)
		if err != nil {
			// Skip individual message failures — don't abort the whole session.
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			toEmbed = append(toEmbed, pendingEmbed{id: msgID, content: msg.Content})
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("memory: SaveSession: commit: %w", err)
	}

	// 3. Embed and insert into vec_messages (outside transaction — embedding is slow
	//    and we don't want to hold a write lock while calling Ollama).
	if s.embeddr == nil || len(toEmbed) == 0 {
		return nil
	}

	for _, m := range toEmbed {
		if len(m.content) < 5 {
			// Skip very short messages — they produce noisy embeddings.
			continue
		}
		embedding, err := s.embeddr.Embed(ctx, m.content)
		if err != nil {
			// Embedding failure is non-fatal. The message is already saved.
			continue
		}
		serialized, err := sqlite_vec.SerializeFloat32(embedding)
		if err != nil {
			continue
		}
		_, _ = s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO vec_messages (message_id, embedding) VALUES (?, ?)`,
			m.id, serialized,
		)
	}

	return nil
}

// Search performs vector similarity search over stored messages.
//
// Flow:
//  1. Embed the query string.
//  2. Query vec_messages for nearest neighbours using cosine distance.
//  3. Join with messages to retrieve text content and metadata.
//  4. Return results sorted by distance (closest first).
//
// Returns empty results (no error) when the embedder is nil or when no
// embeddings exist yet.
func (s *SQLiteStore) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if s.embeddr == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}

	embedding, err := s.embeddr.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("memory: Search: embed query: %w", err)
	}
	serialized, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, fmt.Errorf("memory: Search: serialize embedding: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT m.content, m.session_id, m.timestamp, v.distance
		FROM vec_messages v
		JOIN messages m ON m.id = v.message_id
		WHERE v.embedding MATCH ? AND k = ?
		ORDER BY v.distance ASC`,
		serialized, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: Search: query: %w", err)
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var (
			content   string
			sessionID string
			tsMillis  int64
			distance  float64
		)
		if err := rows.Scan(&content, &sessionID, &tsMillis, &distance); err != nil {
			continue
		}
		results = append(results, Result{
			Text:      content,
			SessionID: sessionID,
			Timestamp: time.UnixMilli(tsMillis),
			Distance:  distance,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: Search: rows: %w", err)
	}

	return results, nil
}

// GenerateBriefing creates a warm, contextual session-start briefing by
// retrieving the last 3–5 sessions and asking the LLM to summarise them.
//
// Flow:
//  1. Retrieve the last 5 sessions from the DB (ordered by started_at DESC).
//  2. For each session, load its messages.
//  3. Format everything as a context block for the LLM.
//  4. Call provider.StreamChat with a meta-prompt requesting a briefing.
//  5. Collect the full streamed response and return it.
//
// If no sessions exist, returns a welcome message without calling the LLM.
func (s *SQLiteStore) GenerateBriefing(ctx context.Context, p provider.Provider) (string, error) {
	sessions, err := s.recentSessions(ctx, 5)
	if err != nil {
		return "", fmt.Errorf("memory: GenerateBriefing: load sessions: %w", err)
	}

	if len(sessions) == 0 {
		return "Welcome. This is your first session. I'm here to help you explore your inner world with curiosity and compassion.", nil
	}

	// Build the context block from recent sessions.
	var sb strings.Builder
	for i, sess := range sessions {
		sb.WriteString(fmt.Sprintf("--- Session %d (started %s) ---\n",
			i+1, sess.StartedAt.Format("2006-01-02 15:04")))
		if sess.Summary != "" {
			sb.WriteString(fmt.Sprintf("Summary: %s\n", sess.Summary))
		}
		for _, msg := range sess.Messages {
			sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
		}
		sb.WriteString("\n")
	}

	systemPrompt := `You are an IFS (Internal Family Systems) companion assistant.
You have been given transcripts of recent sessions with this person.
Your task is to generate a warm, brief (3-5 sentences) session briefing that:
1. Acknowledges the work done in recent sessions
2. Notes any parts or themes that came up
3. Gently orients the person to continue their inner work
4. Speaks with warmth, curiosity, and IFS-informed language
Do not be clinical. Do not list bullet points. Write in flowing, compassionate prose.`

	userPrompt := fmt.Sprintf("Here are the recent sessions:\n\n%s\n\nPlease generate a brief, warm session briefing.", sb.String())

	req := provider.ChatRequest{
		SystemPrompt: systemPrompt,
		Messages: []provider.ChatMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	eventCh, err := p.StreamChat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("memory: GenerateBriefing: stream chat: %w", err)
	}

	var response strings.Builder
	for event := range eventCh {
		switch event.Type {
		case "delta":
			response.WriteString(event.Delta)
		case "error":
			if event.Error != nil {
				return "", fmt.Errorf("memory: GenerateBriefing: stream error: %w", event.Error)
			}
		}
	}

	if response.Len() == 0 {
		return "Welcome back. I'm here whenever you're ready to continue.", nil
	}

	return response.String(), nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// recentSessions loads the last n sessions (by started_at DESC) with their messages.
func (s *SQLiteStore) recentSessions(ctx context.Context, n int) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, started_at, ended_at, summary
		FROM sessions
		ORDER BY started_at DESC
		LIMIT ?`,
		n,
	)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var (
			id          string
			startedAtMs int64
			endedAtMs   sql.NullInt64
			summary     sql.NullString
		)
		if err := rows.Scan(&id, &startedAtMs, &endedAtMs, &summary); err != nil {
			continue
		}
		sess := Session{
			ID:        id,
			StartedAt: time.UnixMilli(startedAtMs),
		}
		if endedAtMs.Valid {
			sess.EndedAt = time.UnixMilli(endedAtMs.Int64)
		}
		if summary.Valid {
			sess.Summary = summary.String
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan sessions: %w", err)
	}

	// Load messages for each session.
	for i := range sessions {
		msgs, err := s.loadMessages(ctx, sessions[i].ID)
		if err != nil {
			// Non-fatal — session without messages is still useful for briefing.
			continue
		}
		sessions[i].Messages = msgs
	}

	return sessions, nil
}

// loadMessages loads all messages for a session, ordered by timestamp.
func (s *SQLiteStore) loadMessages(ctx context.Context, sessionID string) ([]provider.ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT role, content
		FROM messages
		WHERE session_id = ?
		ORDER BY timestamp ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var msgs []provider.ChatMessage
	for rows.Next() {
		var msg provider.ChatMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

// nullableString returns nil if s is empty, otherwise the string value.
// Used for optional TEXT columns (summary).
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
