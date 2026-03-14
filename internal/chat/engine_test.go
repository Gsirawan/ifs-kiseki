package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	// memory package transitively imports sqlite-vec CGO bindings.
	// go-sqlite3 must be imported here to provide the sqlite3_* symbols
	// that sqlite-vec needs at link time.
	_ "github.com/mattn/go-sqlite3"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/memory"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// ── Mock Provider ──────────────────────────────────────────────────

// mockProvider implements provider.Provider with canned streaming responses.
type mockProvider struct {
	name     string
	response string          // text to stream back as deltas
	usage    *provider.Usage // usage to report on "done"
	err      error           // if set, StreamChat returns this error

	// lastRequest captures the most recent ChatRequest for assertions.
	lastRequest *provider.ChatRequest
}

func newMockProvider(response string) *mockProvider {
	return &mockProvider{
		name:     "MockLLM",
		response: response,
		usage: &provider.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Models() []string {
	return []string{"mock-model-v1"}
}

func (m *mockProvider) StreamChat(_ context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	if m.err != nil {
		return nil, m.err
	}

	// Capture the request for test assertions.
	captured := req
	m.lastRequest = &captured

	ch := make(chan provider.StreamEvent, 16)

	go func() {
		defer close(ch)

		// Stream the response as individual word deltas.
		words := strings.Fields(m.response)
		for i, word := range words {
			delta := word
			if i < len(words)-1 {
				delta += " "
			}
			ch <- provider.StreamEvent{
				Type:  "delta",
				Delta: delta,
			}
		}

		// Send done with usage.
		ch <- provider.StreamEvent{
			Type:  "done",
			Usage: m.usage,
		}
	}()

	return ch, nil
}

// testConfig returns a minimal config for testing.
func testConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Companion.Name = "TestKira"
	cfg.Companion.FocusAreas = []string{"testing", "validation"}
	cfg.Companion.CustomInstructions = "Be concise in tests."
	return cfg
}

// collectEvents drains a StreamEvent channel and returns all events.
func collectEvents(t *testing.T, ch <-chan provider.StreamEvent) []provider.StreamEvent {
	t.Helper()
	var events []provider.StreamEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, event)
		case <-timeout:
			t.Fatal("timed out waiting for stream events")
			return events
		}
	}
}

// ── Tests ──────────────────────────────────────────────────────────

func TestNewSession(t *testing.T) {
	mock := newMockProvider("hello")
	engine := NewEngine(mock, testConfig(), nil)

	id := engine.NewSession()

	// ID should be a UUID (36 chars: 8-4-4-4-12).
	if len(id) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(id), id)
	}
	if strings.Count(id, "-") != 4 {
		t.Errorf("expected 4 hyphens in UUID, got %d: %q", strings.Count(id, "-"), id)
	}

	session := engine.GetSession()
	if session == nil {
		t.Fatal("expected session to exist after NewSession()")
	}
	if session.ID != id {
		t.Errorf("session ID mismatch: got %q, want %q", session.ID, id)
	}
	if session.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
	if session.EndedAt != nil {
		t.Error("expected EndedAt to be nil for new session")
	}
	if len(session.Messages) != 0 {
		t.Errorf("expected empty message list, got %d messages", len(session.Messages))
	}
}

func TestSendMessage(t *testing.T) {
	mock := newMockProvider("I hear you")
	engine := NewEngine(mock, testConfig(), nil)
	engine.NewSession()

	ch, err := engine.SendMessage(context.Background(), "Hello, I feel anxious")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	events := collectEvents(t, ch)

	// Should have delta events + done event.
	var deltas []string
	var doneCount int
	var usage *provider.Usage

	for _, e := range events {
		switch e.Type {
		case "delta":
			deltas = append(deltas, e.Delta)
		case "done":
			doneCount++
			usage = e.Usage
		}
	}

	// Verify deltas reconstruct the response.
	fullResponse := strings.Join(deltas, "")
	if fullResponse != "I hear you" {
		t.Errorf("expected response %q, got %q", "I hear you", fullResponse)
	}

	// Verify exactly one done event.
	if doneCount != 1 {
		t.Errorf("expected 1 done event, got %d", doneCount)
	}

	// Verify usage was reported.
	if usage == nil {
		t.Fatal("expected usage in done event")
	}
	if usage.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %d", usage.OutputTokens)
	}

	// Verify session history: user message + assistant response.
	history := engine.GetHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 messages in history, got %d", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "Hello, I feel anxious" {
		t.Errorf("unexpected user message: %+v", history[0])
	}
	if history[1].Role != "assistant" || history[1].Content != "I hear you" {
		t.Errorf("unexpected assistant message: %+v", history[1])
	}

	// Verify usage tracking on session.
	session := engine.GetSession()
	if session.Usage.InputTokens != 10 {
		t.Errorf("expected session input tokens 10, got %d", session.Usage.InputTokens)
	}
	if session.Usage.OutputTokens != 5 {
		t.Errorf("expected session output tokens 5, got %d", session.Usage.OutputTokens)
	}
	if session.Usage.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", session.Usage.Turns)
	}
}

func TestSendMessageNoSession(t *testing.T) {
	mock := newMockProvider("hello")
	engine := NewEngine(mock, testConfig(), nil)

	// No session created — should error.
	_, err := engine.SendMessage(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error when sending without a session")
	}
	if !strings.Contains(err.Error(), "no active session") {
		t.Errorf("expected 'no active session' error, got: %v", err)
	}
}

func TestGetHistoryEmpty(t *testing.T) {
	mock := newMockProvider("hello")
	engine := NewEngine(mock, testConfig(), nil)

	// No session — should return nil.
	history := engine.GetHistory()
	if history != nil {
		t.Errorf("expected nil history without session, got %v", history)
	}

	// With session but no messages — should return empty slice.
	engine.NewSession()
	history = engine.GetHistory()
	if history == nil {
		t.Fatal("expected non-nil history with session")
	}
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d messages", len(history))
	}
}

func TestGetHistoryOrdering(t *testing.T) {
	mock := newMockProvider("response")
	engine := NewEngine(mock, testConfig(), nil)
	engine.NewSession()

	// Manually add messages to verify ordering.
	session := engine.GetSession()
	session.AddMessage("user", "first")
	session.AddMessage("assistant", "second")
	session.AddMessage("user", "third")

	history := engine.GetHistory()
	if len(history) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(history))
	}

	expected := []struct {
		role    string
		content string
	}{
		{"user", "first"},
		{"assistant", "second"},
		{"user", "third"},
	}

	for i, exp := range expected {
		if history[i].Role != exp.role || history[i].Content != exp.content {
			t.Errorf("message %d: expected {%s, %s}, got {%s, %s}",
				i, exp.role, exp.content, history[i].Role, history[i].Content)
		}
	}
}

func TestMultiTurn(t *testing.T) {
	mock := newMockProvider("first response")
	engine := NewEngine(mock, testConfig(), nil)
	engine.NewSession()

	// Turn 1.
	ch, err := engine.SendMessage(context.Background(), "message one")
	if err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}
	collectEvents(t, ch)

	// Change mock response for turn 2.
	mock.response = "second response"

	// Turn 2.
	ch, err = engine.SendMessage(context.Background(), "message two")
	if err != nil {
		t.Fatalf("turn 2 failed: %v", err)
	}
	collectEvents(t, ch)

	// Verify full history: 4 messages (2 user + 2 assistant).
	history := engine.GetHistory()
	if len(history) != 4 {
		t.Fatalf("expected 4 messages after 2 turns, got %d", len(history))
	}

	if history[0].Content != "message one" {
		t.Errorf("expected 'message one', got %q", history[0].Content)
	}
	if history[1].Content != "first response" {
		t.Errorf("expected 'first response', got %q", history[1].Content)
	}
	if history[2].Content != "message two" {
		t.Errorf("expected 'message two', got %q", history[2].Content)
	}
	if history[3].Content != "second response" {
		t.Errorf("expected 'second response', got %q", history[3].Content)
	}

	// Verify the provider received full history on turn 2.
	if mock.lastRequest == nil {
		t.Fatal("expected lastRequest to be captured")
	}
	// Turn 2 should have sent 3 messages: user1, assistant1, user2.
	if len(mock.lastRequest.Messages) != 3 {
		t.Errorf("expected 3 messages sent to provider on turn 2, got %d", len(mock.lastRequest.Messages))
	}

	// Verify system prompt was included.
	if mock.lastRequest.SystemPrompt == "" {
		t.Error("expected system prompt to be set")
	}
	if !strings.Contains(mock.lastRequest.SystemPrompt, "TestKira") {
		t.Error("expected system prompt to contain companion name 'TestKira'")
	}

	// Verify cumulative usage.
	session := engine.GetSession()
	if session.Usage.Turns != 2 {
		t.Errorf("expected 2 turns, got %d", session.Usage.Turns)
	}
	if session.Usage.InputTokens != 20 {
		t.Errorf("expected 20 cumulative input tokens, got %d", session.Usage.InputTokens)
	}
}

func TestSetProvider(t *testing.T) {
	mock1 := newMockProvider("from provider one")
	mock1.name = "Provider1"
	mock2 := newMockProvider("from provider two")
	mock2.name = "Provider2"

	engine := NewEngine(mock1, testConfig(), nil)
	engine.NewSession()

	// Turn 1 with provider 1.
	ch, err := engine.SendMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}
	events := collectEvents(t, ch)

	var response1 strings.Builder
	for _, e := range events {
		if e.Type == "delta" {
			response1.WriteString(e.Delta)
		}
	}
	if response1.String() != "from provider one" {
		t.Errorf("expected 'from provider one', got %q", response1.String())
	}

	// Switch provider.
	engine.SetProvider(mock2)

	// Turn 2 with provider 2 — same session.
	ch, err = engine.SendMessage(context.Background(), "still here")
	if err != nil {
		t.Fatalf("turn 2 failed: %v", err)
	}
	events = collectEvents(t, ch)

	var response2 strings.Builder
	for _, e := range events {
		if e.Type == "delta" {
			response2.WriteString(e.Delta)
		}
	}
	if response2.String() != "from provider two" {
		t.Errorf("expected 'from provider two', got %q", response2.String())
	}

	// Verify session preserved across provider switch.
	history := engine.GetHistory()
	if len(history) != 4 {
		t.Fatalf("expected 4 messages after provider switch, got %d", len(history))
	}

	// Verify mock2 received the full history (including turn 1).
	if mock2.lastRequest == nil {
		t.Fatal("expected mock2 to capture request")
	}
	// Should have: user1, assistant1, user2 = 3 messages.
	if len(mock2.lastRequest.Messages) != 3 {
		t.Errorf("expected 3 messages sent to provider2, got %d", len(mock2.lastRequest.Messages))
	}
}

func TestStreamError(t *testing.T) {
	mock := &mockProvider{
		name:     "ErrorMock",
		response: "",
		usage:    nil,
	}

	// Override StreamChat to send an error event mid-stream.
	engine := NewEngine(mock, testConfig(), nil)
	engine.NewSession()

	// Use a custom mock that sends an error event.
	errMock := &errorStreamMock{}
	engine.SetProvider(errMock)

	ch, err := engine.SendMessage(context.Background(), "trigger error")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	events := collectEvents(t, ch)

	// Should have at least one error event.
	var hasError bool
	for _, e := range events {
		if e.Type == "error" {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected at least one error event")
	}
}

// errorStreamMock sends a delta then an error event.
type errorStreamMock struct{}

func (m *errorStreamMock) Name() string     { return "ErrorStream" }
func (m *errorStreamMock) Models() []string { return []string{"error-model"} }
func (m *errorStreamMock) StreamChat(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 4)
	go func() {
		defer close(ch)
		ch <- provider.StreamEvent{Type: "delta", Delta: "partial "}
		ch <- provider.StreamEvent{
			Type:  "error",
			Error: context.DeadlineExceeded,
		}
	}()
	return ch, nil
}

func TestBuildSystemPrompt(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "", []string{"anxiety", "perfectionism"}, "Be gentle.")

	// The new prompt structure uses "Your name is: Kira" in the companion definition.
	if !strings.Contains(prompt, "Your name is: Kira") {
		t.Error("expected prompt to contain companion name in definition section")
	}
	if !strings.Contains(prompt, "IFS-informed") {
		t.Error("expected prompt to mention IFS")
	}
	if !strings.Contains(prompt, "NOT a therapist") {
		t.Error("expected prompt to disclaim therapy")
	}
	if !strings.Contains(prompt, "anxiety, perfectionism") {
		t.Error("expected prompt to contain focus areas")
	}
	if !strings.Contains(prompt, "Be gentle.") {
		t.Error("expected prompt to contain custom instructions")
	}
}

func TestBuildSystemPromptMinimal(t *testing.T) {
	prompt := BuildSystemPrompt("Companion", "", nil, "")

	// The new prompt structure uses "Your name is: Companion" in the companion definition.
	if !strings.Contains(prompt, "Your name is: Companion") {
		t.Error("expected prompt to contain companion name in definition section")
	}
	if strings.Contains(prompt, "Focus areas:") {
		t.Error("expected no focus areas section when empty")
	}
	// When user name is empty, the prompt uses "friend" fallback — no explicit "user's name is" line.
	if strings.Contains(prompt, "The user's name is: \n") {
		t.Error("expected no empty user name line")
	}
}

func TestBuildSystemPromptWithUserName(t *testing.T) {
	prompt := BuildSystemPrompt("Kira", "Ghaith", []string{"anxiety"}, "")

	if !strings.Contains(prompt, "Ghaith") {
		t.Error("expected prompt to contain user name")
	}
	if !strings.Contains(prompt, "The user's name is: Ghaith") {
		t.Error("expected prompt to contain user name in definition section")
	}
}

func TestSessionEnd(t *testing.T) {
	session := newSession()

	if session.EndedAt != nil {
		t.Error("expected EndedAt to be nil before End()")
	}

	session.End()

	if session.EndedAt == nil {
		t.Fatal("expected EndedAt to be set after End()")
	}
	if session.EndedAt.After(time.Now().Add(time.Second)) {
		t.Error("EndedAt is in the future")
	}
}

func TestNewSessionReplacesOld(t *testing.T) {
	mock := newMockProvider("hello")
	engine := NewEngine(mock, testConfig(), nil)

	id1 := engine.NewSession()
	engine.GetSession().AddMessage("user", "old message")

	id2 := engine.NewSession()

	if id1 == id2 {
		t.Error("expected different session IDs")
	}

	// New session should have empty history.
	history := engine.GetHistory()
	if len(history) != 0 {
		t.Errorf("expected empty history after new session, got %d messages", len(history))
	}
}

// ── Mock Memory Store ──────────────────────────────────────────────

// mockMemoryStore implements memory.Store for testing.
// It records every SaveSession call so tests can assert on it.
type mockMemoryStore struct {
	mu      sync.Mutex
	saved   []memory.Session
	saveErr error // if set, SaveSession returns this error
}

func (m *mockMemoryStore) SaveSession(_ context.Context, sess memory.Session) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saved = append(m.saved, sess)
	return nil
}

func (m *mockMemoryStore) Search(_ context.Context, _ string, _ int) ([]memory.Result, error) {
	return nil, nil
}

func (m *mockMemoryStore) GenerateBriefing(_ context.Context, _ provider.Provider) (string, error) {
	return "", nil
}

// savedSessions returns a snapshot of all saved sessions (thread-safe).
func (m *mockMemoryStore) savedSessions() []memory.Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]memory.Session, len(m.saved))
	copy(out, m.saved)
	return out
}

// ── Step 3.4: InjectMemoryContext tests ────────────────────────────

func TestInjectMemoryContextEmpty(t *testing.T) {
	base := "You are Kira, an IFS companion."

	// nil slice — prompt must be returned unchanged.
	result := InjectMemoryContext(base, nil)
	if result != base {
		t.Errorf("expected unchanged prompt with nil memories, got %q", result)
	}

	// empty slice — same.
	result = InjectMemoryContext(base, []memory.Result{})
	if result != base {
		t.Errorf("expected unchanged prompt with empty memories, got %q", result)
	}
}

func TestInjectMemoryContextWithResults(t *testing.T) {
	base := "You are Kira."
	ts := time.Date(2026, 3, 10, 14, 30, 0, 0, time.UTC)

	memories := []memory.Result{
		{Text: "I feel anxious about my perfectionist part.", Timestamp: ts},
		{Text: "We explored the inner critic today.", Timestamp: ts.Add(time.Hour)},
	}

	result := InjectMemoryContext(base, memories)

	// Must contain the base prompt.
	if !strings.Contains(result, base) {
		t.Error("expected result to contain base prompt")
	}
	// Must contain the memory section markers.
	if !strings.Contains(result, "[MEMORY CONTEXT]") {
		t.Error("expected result to contain [MEMORY CONTEXT] marker")
	}
	if !strings.Contains(result, "[END MEMORY CONTEXT]") {
		t.Error("expected result to contain [END MEMORY CONTEXT] marker")
	}
	// Must contain the memory text.
	if !strings.Contains(result, "perfectionist part") {
		t.Error("expected result to contain first memory text")
	}
	if !strings.Contains(result, "inner critic") {
		t.Error("expected result to contain second memory text")
	}
	// Must contain the timestamp.
	if !strings.Contains(result, "2026-03-10") {
		t.Error("expected result to contain formatted timestamp")
	}
	// Memory section must come after the base prompt.
	baseIdx := strings.Index(result, base)
	memIdx := strings.Index(result, "[MEMORY CONTEXT]")
	if memIdx <= baseIdx {
		t.Error("expected [MEMORY CONTEXT] to appear after base prompt")
	}
}

func TestInjectMemoryContextTruncation(t *testing.T) {
	base := "You are Kira."
	ts := time.Now()

	// Build a text longer than maxMemoryTextLen (200 chars).
	longText := strings.Repeat("a", 250)

	memories := []memory.Result{
		{Text: longText, Timestamp: ts},
	}

	result := InjectMemoryContext(base, memories)

	// The full 250-char string must NOT appear verbatim.
	if strings.Contains(result, longText) {
		t.Error("expected long memory text to be truncated")
	}
	// The truncated version (200 chars + "...") must appear.
	truncated := longText[:maxMemoryTextLen] + "..."
	if !strings.Contains(result, truncated) {
		t.Errorf("expected truncated text to appear in result")
	}
}

// ── Step 3.4: mockSearchStore — extends mockMemoryStore with search tracking ──

// mockSearchStore wraps mockMemoryStore and adds configurable Search behaviour.
// Used only by Step 3.4 tests so it doesn't conflict with Step 3.3's mockMemoryStore.
type mockSearchStore struct {
	mockMemoryStore                 // embed for SaveSession / GenerateBriefing
	searchQuery     string          // last query passed to Search
	searchResults   []memory.Result // results to return
	searchErr       error           // if set, Search returns this error
	searchMu        sync.Mutex
}

func (m *mockSearchStore) Search(_ context.Context, query string, _ int) ([]memory.Result, error) {
	m.searchMu.Lock()
	defer m.searchMu.Unlock()
	m.searchQuery = query
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

// ── Step 3.4: SendMessage memory injection tests ───────────────────

func TestSendMessageInjectsMemoryContext(t *testing.T) {
	mock := newMockProvider("I remember your perfectionist part.")
	ts := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	store := &mockSearchStore{
		searchResults: []memory.Result{
			{Text: "my perfectionist part causes me stress", Timestamp: ts},
		},
	}

	engine := NewEngine(mock, testConfig(), store)
	engine.NewSession()

	ch, err := engine.SendMessage(context.Background(), "I feel anxious")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	collectEvents(t, ch)

	// Verify the store was searched with the user's message as the query.
	store.searchMu.Lock()
	query := store.searchQuery
	store.searchMu.Unlock()

	if query != "I feel anxious" {
		t.Errorf("expected search query %q, got %q", "I feel anxious", query)
	}

	// Verify the system prompt sent to the provider contains the memory context.
	if mock.lastRequest == nil {
		t.Fatal("expected lastRequest to be captured")
	}
	if !strings.Contains(mock.lastRequest.SystemPrompt, "[MEMORY CONTEXT]") {
		t.Error("expected system prompt to contain [MEMORY CONTEXT] section")
	}
	if !strings.Contains(mock.lastRequest.SystemPrompt, "perfectionist part") {
		t.Error("expected system prompt to contain memory text")
	}
}

func TestSendMessageNoMemoryStoreSkipsInjection(t *testing.T) {
	mock := newMockProvider("hello")
	engine := NewEngine(mock, testConfig(), nil) // no store
	engine.NewSession()

	ch, err := engine.SendMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	collectEvents(t, ch)

	if mock.lastRequest == nil {
		t.Fatal("expected lastRequest to be captured")
	}
	// No memory store — system prompt must NOT contain memory section.
	if strings.Contains(mock.lastRequest.SystemPrompt, "[MEMORY CONTEXT]") {
		t.Error("expected no [MEMORY CONTEXT] section when store is nil")
	}
}

func TestSendMessageMemorySearchErrorIsNonFatal(t *testing.T) {
	mock := newMockProvider("hello")

	store := &mockSearchStore{
		searchErr: errors.New("embed: connection refused"),
	}

	engine := NewEngine(mock, testConfig(), store)
	engine.NewSession()

	// Search will fail — SendMessage must still succeed.
	ch, err := engine.SendMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SendMessage should not fail when memory search errors: %v", err)
	}
	collectEvents(t, ch)

	// System prompt must not contain memory section (search failed).
	if mock.lastRequest == nil {
		t.Fatal("expected lastRequest to be captured")
	}
	if strings.Contains(mock.lastRequest.SystemPrompt, "[MEMORY CONTEXT]") {
		t.Error("expected no [MEMORY CONTEXT] section when search fails")
	}
}

func TestSendMessageEmptyMemoryResultsNoSection(t *testing.T) {
	mock := newMockProvider("hello")

	store := &mockSearchStore{
		searchResults: []memory.Result{}, // empty — no relevant past context
	}

	engine := NewEngine(mock, testConfig(), store)
	engine.NewSession()

	ch, err := engine.SendMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	collectEvents(t, ch)

	if mock.lastRequest == nil {
		t.Fatal("expected lastRequest to be captured")
	}
	// Empty results — no memory section should be injected.
	if strings.Contains(mock.lastRequest.SystemPrompt, "[MEMORY CONTEXT]") {
		t.Error("expected no [MEMORY CONTEXT] section when search returns empty results")
	}
}

// ── New Tests ──────────────────────────────────────────────────────

// TestEndSession verifies that EndSession saves the current session to the
// memory store and marks it as ended.
func TestEndSession(t *testing.T) {
	mock := newMockProvider("hello")
	store := &mockMemoryStore{}
	engine := NewEngine(mock, testConfig(), store)

	// No session — EndSession should be a no-op.
	if err := engine.EndSession(); err != nil {
		t.Fatalf("EndSession with no session returned error: %v", err)
	}
	if len(store.savedSessions()) != 0 {
		t.Fatal("expected no saves when there is no session")
	}

	// Create a session and add a message.
	engine.NewSession()
	engine.GetSession().AddMessage("user", "I feel overwhelmed")
	engine.GetSession().AddMessage("assistant", "Tell me more about that feeling.")

	sessionID := engine.GetSession().ID

	// End the session.
	if err := engine.EndSession(); err != nil {
		t.Fatalf("EndSession returned error: %v", err)
	}

	// The goroutine is async — give it a moment to complete.
	deadline := time.Now().Add(2 * time.Second)
	var sessions []memory.Session
	for time.Now().Before(deadline) {
		sessions = store.savedSessions()
		if len(sessions) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 saved session, got %d", len(sessions))
	}

	saved := sessions[0]

	if saved.ID != sessionID {
		t.Errorf("saved session ID mismatch: got %q, want %q", saved.ID, sessionID)
	}
	if len(saved.Messages) != 2 {
		t.Errorf("expected 2 messages in saved session, got %d", len(saved.Messages))
	}
	if saved.Messages[0].Role != "user" || saved.Messages[0].Content != "I feel overwhelmed" {
		t.Errorf("unexpected first message: %+v", saved.Messages[0])
	}
	if saved.EndedAt.IsZero() {
		t.Error("expected EndedAt to be set on saved session")
	}
	if saved.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set on saved session")
	}
}

// TestNewSessionSavesOld verifies that starting a new session automatically
// saves the previous session to the memory store.
func TestNewSessionSavesOld(t *testing.T) {
	mock := newMockProvider("hello")
	store := &mockMemoryStore{}
	engine := NewEngine(mock, testConfig(), store)

	// Session 1 — add a message.
	engine.NewSession()
	engine.GetSession().AddMessage("user", "first session message")
	firstID := engine.GetSession().ID

	// Start session 2 — should trigger save of session 1.
	engine.NewSession()
	secondID := engine.GetSession().ID

	if firstID == secondID {
		t.Error("expected different session IDs")
	}

	// Wait for the async save to complete.
	deadline := time.Now().Add(2 * time.Second)
	var sessions []memory.Session
	for time.Now().Before(deadline) {
		sessions = store.savedSessions()
		if len(sessions) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 saved session after NewSession, got %d", len(sessions))
	}

	saved := sessions[0]

	if saved.ID != firstID {
		t.Errorf("expected first session to be saved, got ID %q", saved.ID)
	}
	if len(saved.Messages) != 1 {
		t.Errorf("expected 1 message in saved session, got %d", len(saved.Messages))
	}
	if saved.Messages[0].Content != "first session message" {
		t.Errorf("unexpected message content: %q", saved.Messages[0].Content)
	}
	if saved.EndedAt.IsZero() {
		t.Error("expected EndedAt to be set when session was replaced")
	}

	// Session 2 should be active and empty.
	history := engine.GetHistory()
	if len(history) != 0 {
		t.Errorf("expected empty history for new session, got %d messages", len(history))
	}
}
