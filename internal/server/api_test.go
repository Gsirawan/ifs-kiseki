package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/db"
	"github.com/Gsirawan/ifs-kiseki/internal/memory"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// testServer creates a Server backed by a temp SQLite DB and default config.
// memoryStore and provider are both nil — tests that need them set them directly.
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

	srv := NewServer(database, cfg, http.Dir("."), nil, nil, nil, nil)
	return srv, database
}

// ── Briefing test helpers ────────────────────────────────────────

// mockMemoryStore is a minimal memory.Store for briefing tests.
type mockMemoryStore struct {
	briefing string
	err      error
}

func (m *mockMemoryStore) SaveSession(_ context.Context, _ memory.Session) error {
	return nil
}

func (m *mockMemoryStore) Search(_ context.Context, _ string, _ int) ([]memory.Result, error) {
	return nil, nil
}

func (m *mockMemoryStore) GenerateBriefing(_ context.Context, _ provider.Provider) (string, error) {
	return m.briefing, m.err
}

// mockProvider is a minimal provider.Provider for briefing tests.
type mockProvider struct{}

func (m *mockProvider) Name() string     { return "mock" }
func (m *mockProvider) Models() []string { return []string{"mock-model"} }
func (m *mockProvider) StreamChat(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 1)
	ch <- provider.StreamEvent{Type: "done"}
	close(ch)
	return ch, nil
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

// ── Briefing endpoint tests ──────────────────────────────────────

// TestBriefingNoMemory verifies that GET /api/briefing returns a welcome
// message when no memory store is configured (nil memoryStore).
func TestBriefingNoMemory(t *testing.T) {
	srv, _ := testServer(t) // memoryStore is nil
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/briefing", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp briefingResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Briefing == "" {
		t.Error("expected non-empty briefing in welcome response")
	}
	// Should be the static welcome message — no LLM call.
	if resp.Briefing != "Welcome! This is your first session." {
		t.Errorf("expected static welcome message, got %q", resp.Briefing)
	}
}

// TestBriefingNoProvider verifies that GET /api/briefing returns 503 when
// a memory store exists but no LLM provider is configured.
func TestBriefingNoProvider(t *testing.T) {
	srv, _ := testServer(t)
	srv.memoryStore = &mockMemoryStore{briefing: "should not be called"}
	// srv.provider remains nil
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/briefing", "")

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error field in response")
	}
}

// TestBriefingWithMemoryAndProvider verifies that GET /api/briefing returns
// 200 with the briefing text from the memory store when both are configured.
func TestBriefingWithMemoryAndProvider(t *testing.T) {
	srv, _ := testServer(t)
	expectedBriefing := "Welcome back. Last time we explored your anxious part."
	srv.memoryStore = &mockMemoryStore{briefing: expectedBriefing}
	srv.provider = &mockProvider{}
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/briefing", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp briefingResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Briefing != expectedBriefing {
		t.Errorf("expected briefing %q, got %q", expectedBriefing, resp.Briefing)
	}
}

// TestBriefingContentType verifies that GET /api/briefing returns JSON.
func TestBriefingContentType(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "GET", "/api/briefing", "")

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", ct)
	}
}

// ── Accept-disclaimer endpoint tests ────────────────────────────

// TestAcceptDisclaimerSetsFlag verifies that POST /api/accept-disclaimer
// sets DisclaimerAccepted=true in the in-memory config.
func TestAcceptDisclaimerSetsFlag(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	// Disclaimer should start as false.
	if srv.cfg.DisclaimerAccepted {
		t.Fatal("expected DisclaimerAccepted=false before acceptance")
	}

	rr := doRequest(t, handler, "POST", "/api/accept-disclaimer", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp["accepted"] {
		t.Error("expected accepted=true in response")
	}

	// In-memory config should be updated.
	if !srv.cfg.DisclaimerAccepted {
		t.Error("expected DisclaimerAccepted=true after acceptance")
	}
	if srv.cfg.DisclaimerAcceptedAt == "" {
		t.Error("expected DisclaimerAcceptedAt to be set after acceptance")
	}
}

// TestAcceptDisclaimerPersists verifies that POST /api/accept-disclaimer
// saves the acceptance to disk so it survives a reload.
func TestAcceptDisclaimerPersists(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "POST", "/api/accept-disclaimer", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Reload config from disk and verify the flag persisted.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !loaded.DisclaimerAccepted {
		t.Error("expected DisclaimerAccepted=true after reload from disk")
	}
	if loaded.DisclaimerAcceptedAt == "" {
		t.Error("expected DisclaimerAcceptedAt to be non-empty after reload from disk")
	}
}

// TestAcceptDisclaimerContentType verifies the response is JSON.
func TestAcceptDisclaimerContentType(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "POST", "/api/accept-disclaimer", "")

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", ct)
	}
}

// ── Test-provider endpoint tests ─────────────────────────────────

// testProviderServer creates a mock HTTP server that simulates an LLM API.
// It returns the server URL and a cleanup function.
func testProviderServer(t *testing.T, statusCode int, body string) string {
	t.Helper()
	mux := http.NewServeMux()
	// Anthropic endpoint.
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			// Minimal valid Anthropic SSE stream.
			fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n")
			fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
			fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"OK\"}}\n\n")
			fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
			fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n")
			fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		} else {
			fmt.Fprint(w, body)
		}
	})
	// OpenAI-compatible endpoint.
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			// Minimal valid OpenAI SSE stream.
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"},\"finish_reason\":null}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
		} else {
			fmt.Fprint(w, body)
		}
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts.URL
}

// TestTestProviderClaudeSuccess verifies that a valid Claude key returns success.
func TestTestProviderClaudeSuccess(t *testing.T) {
	mockURL := testProviderServer(t, http.StatusOK, "")

	srv, _ := testServer(t)
	// Point Claude at the mock server.
	srv.cfg.Providers.Claude.BaseURL = mockURL
	handler := srv.SetupRoutes()

	body := `{"provider":"claude","api_key":"sk-ant-valid-key-1234"}`
	rr := doRequest(t, handler, "POST", "/api/test-provider", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp testProviderResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success=true, got error: %s", resp.Error)
	}
}

// TestTestProviderGrokSuccess verifies that a valid Grok key returns success.
func TestTestProviderGrokSuccess(t *testing.T) {
	mockURL := testProviderServer(t, http.StatusOK, "")

	srv, _ := testServer(t)
	srv.cfg.Providers.Grok.BaseURL = mockURL
	handler := srv.SetupRoutes()

	body := `{"provider":"grok","api_key":"xai-valid-key-1234"}`
	rr := doRequest(t, handler, "POST", "/api/test-provider", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp testProviderResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success=true, got error: %s", resp.Error)
	}
}

// TestTestProviderInvalidKey verifies that a 401 response returns success=false.
func TestTestProviderInvalidKey(t *testing.T) {
	mockURL := testProviderServer(t, http.StatusUnauthorized,
		`{"error":{"type":"authentication_error","message":"invalid x-api-key"}}`)

	srv, _ := testServer(t)
	srv.cfg.Providers.Claude.BaseURL = mockURL
	handler := srv.SetupRoutes()

	body := `{"provider":"claude","api_key":"sk-ant-bad-key"}`
	rr := doRequest(t, handler, "POST", "/api/test-provider", body)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp testProviderResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Error("expected success=false for invalid key")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error message")
	}
}

// TestTestProviderMissingFields verifies that missing required fields return 400.
func TestTestProviderMissingFields(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	// Missing api_key.
	rr := doRequest(t, handler, "POST", "/api/test-provider", `{"provider":"claude"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing api_key, got %d", rr.Code)
	}

	// Missing provider.
	rr = doRequest(t, handler, "POST", "/api/test-provider", `{"api_key":"sk-ant-key"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing provider, got %d", rr.Code)
	}

	// Unknown provider.
	rr = doRequest(t, handler, "POST", "/api/test-provider", `{"provider":"openai","api_key":"sk-key"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown provider, got %d", rr.Code)
	}
}

// TestTestProviderInvalidJSON verifies that malformed JSON returns 400.
func TestTestProviderInvalidJSON(t *testing.T) {
	srv, _ := testServer(t)
	handler := srv.SetupRoutes()

	rr := doRequest(t, handler, "POST", "/api/test-provider", `{not valid json}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}
}
