package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/db"
)

// testServer creates a Server backed by a temp SQLite DB and default config.
func testServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.InitDB(dbPath, 1024)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.Providers.Claude.APIKey = "sk-ant-test-key-1234567890abcdef"
	cfg.Providers.Grok.APIKey = "xai-test-key-abcdef1234567890"

	srv := NewServer(database, cfg, http.Dir("."), nil)
	return srv, database
}

// doRequest performs a request against the test server's mux and returns the response.
func doRequest(t *testing.T, handler http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/health", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}
	if resp["version"] != "0.1.0" {
		t.Errorf("expected version=0.1.0, got %q", resp["version"])
	}
}

func TestSessionsEmptyList(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/sessions", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var sessions []sessionRow
	if err := json.Unmarshal(rr.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty session list, got %d", len(sessions))
	}
}

func TestSessionsWithData(t *testing.T) {
	srv, database := testServer(t)
	handler := srv.SetupRoutes()

	// Insert a test session.
	_, err := database.Exec(
		`INSERT INTO sessions (id, started_at, summary, created_at) VALUES (?, ?, ?, ?)`,
		"sess-1", 1700000000, "test session", "2024-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("failed to insert test session: %v", err)
	}

	rr := doRequest(t, handler, "GET", "/api/sessions", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var sessions []sessionRow
	if err := json.Unmarshal(rr.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "sess-1" {
		t.Errorf("expected session id=sess-1, got %q", sessions[0].ID)
	}
}

func TestSessionNotFound(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/sessions/nonexistent", "")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "session not found" {
		t.Errorf("expected error='session not found', got %q", resp["error"])
	}
}

func TestSessionWithMessages(t *testing.T) {
	srv, database := testServer(t)
	handler := srv.SetupRoutes()

	// Insert session + messages.
	_, err := database.Exec(
		`INSERT INTO sessions (id, started_at, summary, created_at) VALUES (?, ?, ?, ?)`,
		"sess-2", 1700000000, "session with messages", "2024-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}
	_, err = database.Exec(
		`INSERT INTO messages (id, session_id, role, content, timestamp) VALUES (?, ?, ?, ?, ?)`,
		"msg-1", "sess-2", "user", "hello", 1700000001,
	)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}
	_, err = database.Exec(
		`INSERT INTO messages (id, session_id, role, content, timestamp) VALUES (?, ?, ?, ?, ?)`,
		"msg-2", "sess-2", "assistant", "hi there", 1700000002,
	)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}

	rr := doRequest(t, handler, "GET", "/api/sessions/sess-2", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var detail sessionDetail
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if detail.ID != "sess-2" {
		t.Errorf("expected session id=sess-2, got %q", detail.ID)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(detail.Messages))
	}
	if detail.Messages[0].Role != "user" {
		t.Errorf("expected first message role=user, got %q", detail.Messages[0].Role)
	}
	if detail.Messages[1].Content != "hi there" {
		t.Errorf("expected second message content='hi there', got %q", detail.Messages[1].Content)
	}
}

func TestSessionDelete(t *testing.T) {
	srv, database := testServer(t)
	handler := srv.SetupRoutes()

	// Insert session + message.
	_, _ = database.Exec(
		`INSERT INTO sessions (id, started_at, summary, created_at) VALUES (?, ?, ?, ?)`,
		"sess-del", 1700000000, "to delete", "2024-01-01T00:00:00Z",
	)
	_, _ = database.Exec(
		`INSERT INTO messages (id, session_id, role, content, timestamp) VALUES (?, ?, ?, ?, ?)`,
		"msg-del", "sess-del", "user", "bye", 1700000001,
	)

	// Delete.
	rr := doRequest(t, handler, "DELETE", "/api/sessions/sess-del", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify gone.
	rr = doRequest(t, handler, "GET", "/api/sessions/sess-del", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rr.Code)
	}

	// Verify messages gone.
	var count int
	err := database.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, "sess-del").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count messages: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 messages after delete, got %d", count)
	}
}

func TestDeleteNonexistentSession(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "DELETE", "/api/sessions/ghost", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestSettingsRedactsKeys(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/settings", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var cfg config.Config
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Keys should be redacted — last 4 chars only.
	if cfg.Providers.Claude.APIKey != "****cdef" {
		t.Errorf("expected Claude key redacted to '****cdef', got %q", cfg.Providers.Claude.APIKey)
	}
	if cfg.Providers.Grok.APIKey != "****7890" {
		t.Errorf("expected Grok key redacted to '****7890', got %q", cfg.Providers.Grok.APIKey)
	}

	// Non-secret fields should be present.
	if cfg.Provider != "claude" {
		t.Errorf("expected provider=claude, got %q", cfg.Provider)
	}
	if cfg.Server.Port != 3737 {
		t.Errorf("expected port=3737, got %d", cfg.Server.Port)
	}
}

func TestSettingsEmptyKey(t *testing.T) {
	srv, _ := testServer(t)
	// Clear the keys.
	srv.cfg.Providers.Claude.APIKey = ""
	srv.cfg.Providers.Grok.APIKey = ""
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/settings", "")

	var cfg config.Config
	if err := json.Unmarshal(rr.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if cfg.Providers.Claude.APIKey != "" {
		t.Errorf("expected empty Claude key, got %q", cfg.Providers.Claude.APIKey)
	}
	if cfg.Providers.Grok.APIKey != "" {
		t.Errorf("expected empty Grok key, got %q", cfg.Providers.Grok.APIKey)
	}
}

func TestProvidersEndpoint(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/providers", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var providers []providerInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &providers); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}

	// Claude.
	if providers[0].Name != "claude" {
		t.Errorf("expected first provider=claude, got %q", providers[0].Name)
	}
	if !providers[0].HasKey {
		t.Error("expected Claude has_key=true")
	}

	// Grok.
	if providers[1].Name != "grok" {
		t.Errorf("expected second provider=grok, got %q", providers[1].Name)
	}
	if !providers[1].HasKey {
		t.Error("expected Grok has_key=true")
	}
}

func TestProvidersNoKeys(t *testing.T) {
	srv, _ := testServer(t)
	srv.cfg.Providers.Claude.APIKey = ""
	srv.cfg.Providers.Grok.APIKey = ""
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/providers", "")

	var providers []providerInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &providers); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if providers[0].HasKey {
		t.Error("expected Claude has_key=false when no key set")
	}
	if providers[1].HasKey {
		t.Error("expected Grok has_key=false when no key set")
	}
}

func TestRedactKeyFunction(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"abc", "****"},
		{"abcd", "****"},
		{"abcde", "****bcde"},
		{"sk-ant-test-key-1234567890abcdef", "****cdef"},
	}
	for _, tt := range tests {
		got := redactKey(tt.input)
		if got != tt.want {
			t.Errorf("redactKey(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestContentTypeJSON(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/health", "")

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", ct)
	}
}
