package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
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
	engine := NewEngine(mock, testConfig())

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
	engine := NewEngine(mock, testConfig())
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
	engine := NewEngine(mock, testConfig())

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
	engine := NewEngine(mock, testConfig())

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
	engine := NewEngine(mock, testConfig())
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
	engine := NewEngine(mock, testConfig())
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

	engine := NewEngine(mock1, testConfig())
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
	engine := NewEngine(mock, testConfig())
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
	prompt := BuildSystemPrompt("Kira", []string{"anxiety", "perfectionism"}, "Be gentle.")

	if !strings.Contains(prompt, "You are Kira") {
		t.Error("expected prompt to contain companion name")
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
	prompt := BuildSystemPrompt("Companion", nil, "")

	if !strings.Contains(prompt, "You are Companion") {
		t.Error("expected prompt to contain companion name")
	}
	if strings.Contains(prompt, "Focus areas") {
		t.Error("expected no focus areas section when empty")
	}
}

func TestSessionEnd(t *testing.T) {
	session := NewSession()

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
	engine := NewEngine(mock, testConfig())

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
