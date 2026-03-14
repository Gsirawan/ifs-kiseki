package memory

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"

	"github.com/Gsirawan/ifs-kiseki/internal/db"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

func init() {
	sqlite_vec.Auto()
}

// ── Test helpers ──────────────────────────────────────────────────────────────

// testDB creates a fresh in-memory SQLite database with the IFS-Kiseki schema.
// Uses a temp file (not :memory:) because sqlite-vec virtual tables require
// a real file path in some CGO configurations.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.InitDB(dbPath, 4) // dim=4 for fast fake embeddings
	if err != nil {
		t.Fatalf("testDB: InitDB failed: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// mockEmbedder returns deterministic fake vectors of the given dimension.
// The vector is seeded from the first byte of the input text so different
// inputs produce different (but reproducible) vectors.
type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, m.dim)
	seed := float32(1.0)
	if len(text) > 0 {
		seed = float32(text[0]) / 255.0
	}
	for i := range vec {
		vec[i] = seed + float32(i)*0.01
	}
	return vec, nil
}

// mockProvider is a minimal provider.Provider for briefing tests.
type mockProvider struct {
	response string
}

func (m *mockProvider) Name() string     { return "mock" }
func (m *mockProvider) Models() []string { return []string{"mock-model"} }
func (m *mockProvider) StreamChat(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 2)
	ch <- provider.StreamEvent{Type: "delta", Delta: m.response}
	ch <- provider.StreamEvent{Type: "done"}
	close(ch)
	return ch, nil
}

// makeSession builds a test Session with the given messages.
func makeSession(id string, messages []provider.ChatMessage) Session {
	return Session{
		ID:        id,
		Messages:  messages,
		StartedAt: time.Now().Add(-10 * time.Minute),
		EndedAt:   time.Now(),
		Summary:   "test session summary",
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestSaveSession_InsertsSessionAndMessages verifies that SaveSession writes
// rows to the sessions, messages, and vec_messages tables.
func TestSaveSession_InsertsSessionAndMessages(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	sess := makeSession("sess-001", []provider.ChatMessage{
		{Role: "user", Content: "I feel anxious about my perfectionist part."},
		{Role: "assistant", Content: "Let's get curious about that part. Where do you feel it in your body?"},
	})

	ctx := context.Background()
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Verify sessions table.
	var sessionID string
	err := database.QueryRow(`SELECT id FROM sessions WHERE id = ?`, sess.ID).Scan(&sessionID)
	if err != nil {
		t.Errorf("session not found in sessions table: %v", err)
	}
	if sessionID != sess.ID {
		t.Errorf("expected session ID %q, got %q", sess.ID, sessionID)
	}

	// Verify messages table.
	var msgCount int
	err = database.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sess.ID).Scan(&msgCount)
	if err != nil {
		t.Fatalf("count messages failed: %v", err)
	}
	if msgCount != 2 {
		t.Errorf("expected 2 messages, got %d", msgCount)
	}

	// Verify vec_messages table — embeddings should exist for both messages.
	var vecCount int
	err = database.QueryRow(`SELECT COUNT(*) FROM vec_messages`).Scan(&vecCount)
	if err != nil {
		t.Fatalf("count vec_messages failed: %v", err)
	}
	if vecCount != 2 {
		t.Errorf("expected 2 vec_messages rows, got %d", vecCount)
	}
}

// TestSaveSession_NilEmbedder verifies that SaveSession saves session and
// messages even when the embedder is nil — embeddings are simply skipped.
func TestSaveSession_NilEmbedder(t *testing.T) {
	database := testDB(t)
	store := NewSQLiteStore(database, nil) // nil embedder

	sess := makeSession("sess-nil-emb", []provider.ChatMessage{
		{Role: "user", Content: "Hello, I want to explore my inner critic."},
		{Role: "assistant", Content: "Of course. Let's start by noticing it."},
	})

	ctx := context.Background()
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession with nil embedder failed: %v", err)
	}

	// Session must be saved.
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, sess.ID).Scan(&count); err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 session, got %d", count)
	}

	// Messages must be saved.
	if err := database.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sess.ID).Scan(&count); err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 messages, got %d", count)
	}

	// No embeddings should exist.
	if err := database.QueryRow(`SELECT COUNT(*) FROM vec_messages`).Scan(&count); err != nil {
		t.Fatalf("query vec_messages: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 vec_messages rows with nil embedder, got %d", count)
	}
}

// TestSaveSession_EmptyMessages verifies that a session with no messages
// still saves the session metadata row without error.
func TestSaveSession_EmptyMessages(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	sess := makeSession("sess-empty", []provider.ChatMessage{})

	ctx := context.Background()
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession with empty messages failed: %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, sess.ID).Scan(&count); err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 session row, got %d", count)
	}

	if err := database.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sess.ID).Scan(&count); err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 messages for empty session, got %d", count)
	}
}

// TestSaveSession_Idempotent verifies that saving the same session twice
// does not duplicate rows (INSERT OR IGNORE semantics).
func TestSaveSession_Idempotent(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	sess := makeSession("sess-idem", []provider.ChatMessage{
		{Role: "user", Content: "Testing idempotency."},
	})

	ctx := context.Background()
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("first SaveSession failed: %v", err)
	}
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("second SaveSession failed: %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, sess.ID).Scan(&count); err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 session row after double-save, got %d", count)
	}
}

// TestSearch_ReturnsResults verifies that after saving a session, searching
// for a phrase from it returns at least one result.
func TestSearch_ReturnsResults(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	sess := makeSession("sess-search", []provider.ChatMessage{
		{Role: "user", Content: "My inner critic is very loud today."},
		{Role: "assistant", Content: "Let's acknowledge that part with compassion."},
	})

	ctx := context.Background()
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	results, err := store.Search(ctx, "inner critic", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one search result, got none")
	}

	// All results must have non-empty text and a valid session ID.
	for i, r := range results {
		if r.Text == "" {
			t.Errorf("result[%d]: empty text", i)
		}
		if r.SessionID == "" {
			t.Errorf("result[%d]: empty session ID", i)
		}
	}
}

// TestSearch_NilEmbedder verifies that Search returns empty results (no error)
// when the embedder is nil.
func TestSearch_NilEmbedder(t *testing.T) {
	database := testDB(t)
	store := NewSQLiteStore(database, nil) // nil embedder

	ctx := context.Background()
	results, err := store.Search(ctx, "anything", 5)
	if err != nil {
		t.Errorf("Search with nil embedder should not return error, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search with nil embedder should return empty results, got %d", len(results))
	}
}

// TestSearch_EmptyStore verifies that searching an empty store returns
// empty results without error.
func TestSearch_EmptyStore(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	ctx := context.Background()
	results, err := store.Search(ctx, "perfectionism", 5)
	if err != nil {
		t.Fatalf("Search on empty store failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty store, got %d", len(results))
	}
}

// TestSearch_LimitRespected verifies that Search returns at most `limit` results.
func TestSearch_LimitRespected(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	ctx := context.Background()

	// Save 3 sessions with 2 messages each = 6 total messages.
	for i := 0; i < 3; i++ {
		sess := makeSession(
			fmt.Sprintf("sess-limit-%d", i),
			[]provider.ChatMessage{
				{Role: "user", Content: fmt.Sprintf("Session %d user message about parts work.", i)},
				{Role: "assistant", Content: fmt.Sprintf("Session %d assistant response about parts.", i)},
			},
		)
		if err := store.SaveSession(ctx, sess); err != nil {
			t.Fatalf("SaveSession %d failed: %v", i, err)
		}
	}

	limit := 2
	results, err := store.Search(ctx, "parts work", limit)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) > limit {
		t.Errorf("expected at most %d results, got %d", limit, len(results))
	}
}

// TestGenerateBriefing_NoSessions verifies that GenerateBriefing returns a
// welcome message when no sessions exist (without calling the LLM).
func TestGenerateBriefing_NoSessions(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	p := &mockProvider{response: "should not be called"}

	ctx := context.Background()
	briefing, err := store.GenerateBriefing(ctx, p)
	if err != nil {
		t.Fatalf("GenerateBriefing with no sessions failed: %v", err)
	}
	if briefing == "" {
		t.Error("expected a non-empty welcome message, got empty string")
	}
}

// TestGenerateBriefing_WithSessions verifies that GenerateBriefing calls the
// provider and returns its response when sessions exist.
func TestGenerateBriefing_WithSessions(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	ctx := context.Background()

	// Save a session so GenerateBriefing has something to work with.
	sess := makeSession("sess-brief", []provider.ChatMessage{
		{Role: "user", Content: "I've been working on my anxious part."},
		{Role: "assistant", Content: "That's meaningful work. How does it feel now?"},
	})
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	expectedResponse := "Welcome back. Last time we explored your anxious part."
	p := &mockProvider{response: expectedResponse}

	briefing, err := store.GenerateBriefing(ctx, p)
	if err != nil {
		t.Fatalf("GenerateBriefing failed: %v", err)
	}
	if briefing != expectedResponse {
		t.Errorf("expected briefing %q, got %q", expectedResponse, briefing)
	}
}

// TestSaveSession_EmptyID verifies that SaveSession rejects a session with
// an empty ID.
func TestSaveSession_EmptyID(t *testing.T) {
	database := testDB(t)
	emb := &mockEmbedder{dim: 4}
	store := NewSQLiteStore(database, emb)

	sess := Session{
		ID:        "", // invalid
		Messages:  []provider.ChatMessage{{Role: "user", Content: "hello"}},
		StartedAt: time.Now(),
	}

	ctx := context.Background()
	err := store.SaveSession(ctx, sess)
	if err == nil {
		t.Error("expected error for empty session ID, got nil")
	}
}
