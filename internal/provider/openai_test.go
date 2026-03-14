package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── Helpers ──────────────────────────────────────────────────────────

// collectEvents drains a StreamEvent channel and returns all events.
// Times out after 5 seconds to prevent test hangs.
func collectEvents(t *testing.T, ch <-chan StreamEvent) []StreamEvent {
	t.Helper()
	var events []StreamEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-timeout:
			t.Fatal("timed out waiting for stream events")
			return events
		}
	}
}

// newTestProvider creates an OpenAICompatibleProvider pointing at a test server.
func newTestProvider(serverURL string) *OpenAICompatibleProvider {
	return NewOpenAICompatibleProvider("test-key", serverURL, "test-model", 1024, 0.7)
}

// ── Tests ────────────────────────────────────────────────────────────

func TestStreamChat_HappyPath(t *testing.T) {
	// Mock server returns a proper SSE stream.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request basics.
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}

		// Write SSE response.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("server does not support flushing")
		}

		// Role-only delta (should be skipped).
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		flusher.Flush()

		// Content deltas.
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"}}]}\n\n")
		flusher.Flush()

		// Final chunk with usage.
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\n")
		flusher.Flush()

		// Stream termination.
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		SystemPrompt: "You are helpful.",
		Messages: []ChatMessage{
			{Role: "user", Content: "Say hello"},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}

	events := collectEvents(t, ch)

	// Expect: delta("Hello"), delta(" world"), done.
	var deltas []string
	var doneCount int
	var usage *Usage

	for _, ev := range events {
		switch ev.Type {
		case "delta":
			deltas = append(deltas, ev.Delta)
		case "done":
			doneCount++
			usage = ev.Usage
		case "error":
			t.Fatalf("unexpected error event: %v", ev.Error)
		}
	}

	if got := strings.Join(deltas, ""); got != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", got)
	}

	if doneCount != 1 {
		t.Errorf("expected 1 done event, got %d", doneCount)
	}

	if usage == nil {
		t.Fatal("expected usage in done event, got nil")
	}
	if usage.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 2 {
		t.Errorf("expected 2 output tokens, got %d", usage.OutputTokens)
	}
}

func TestStreamChat_SystemPromptInjection(t *testing.T) {
	// Verify that the system prompt is sent as the first message with role "system".
	var capturedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read and capture the request body.
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)

		// Return minimal valid SSE.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		SystemPrompt: "You are an IFS therapist.",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
			{Role: "user", Content: "How are you?"},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}

	// Drain the channel.
	collectEvents(t, ch)

	// Parse the captured body and verify message order.
	var req openaiRequest
	if err := parseJSON(capturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	// Should have 4 messages: system + 3 conversation messages.
	if len(req.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(req.Messages))
	}

	// First message must be system.
	if req.Messages[0].Role != "system" {
		t.Errorf("first message role should be 'system', got %q", req.Messages[0].Role)
	}
	if req.Messages[0].Content != "You are an IFS therapist." {
		t.Errorf("system message content mismatch: %q", req.Messages[0].Content)
	}

	// Remaining messages should preserve order.
	expectedRoles := []string{"user", "assistant", "user"}
	for i, role := range expectedRoles {
		if req.Messages[i+1].Role != role {
			t.Errorf("message[%d] role: expected %q, got %q", i+1, role, req.Messages[i+1].Role)
		}
	}

	// Verify stream is true.
	if !req.Stream {
		t.Error("expected stream=true in request")
	}
}

func TestStreamChat_NoSystemPrompt(t *testing.T) {
	// When no system prompt is provided, messages should not include a system message.
	var capturedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	collectEvents(t, ch)

	var req openaiRequest
	if err := parseJSON(capturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	// Should have 1 message only — no system message.
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Errorf("expected 'user' role, got %q", req.Messages[0].Role)
	}
}

func TestOpenAI_StreamChat_APIError(t *testing.T) {
	// Mock server returns an API error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Invalid API key","type":"authentication_error","code":"invalid_api_key"}}`)
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	_, err := p.StreamChat(context.Background(), ChatRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "Invalid API key") {
		t.Errorf("error should contain 'Invalid API key', got: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should contain status code 401, got: %v", err)
	}
}

func TestStreamChat_APIErrorPlainText(t *testing.T) {
	// Mock server returns a non-JSON error body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal server error")
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	_, err := p.StreamChat(context.Background(), ChatRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code 500, got: %v", err)
	}
}

func TestOpenAI_StreamChat_ContextCancellation(t *testing.T) {
	// Mock server sends data slowly — context cancellation should stop it.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("server does not support flushing")
		}

		// Send first chunk.
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()

		// Wait long enough for the context to be cancelled.
		time.Sleep(2 * time.Second)

		// This chunk should not be received.
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"}}]}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	p := newTestProvider(server.URL)
	ch, err := p.StreamChat(ctx, ChatRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}

	// Read first event.
	ev := <-ch
	if ev.Type != "delta" || ev.Delta != "Hello" {
		t.Errorf("expected delta 'Hello', got type=%q delta=%q", ev.Type, ev.Delta)
	}

	// Cancel context.
	cancel()

	// Drain remaining events — should get an error event, then channel closes.
	events := collectEvents(t, ch)

	// Should have gotten an error event from context cancellation.
	hasError := false
	for _, ev := range events {
		if ev.Type == "error" {
			hasError = true
		}
	}

	if !hasError {
		t.Error("expected an error event from context cancellation")
	}
}

func TestStreamChat_MalformedJSON(t *testing.T) {
	// Mock server sends malformed JSON in SSE.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprint(w, "data: {broken json\n\n")
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}

	events := collectEvents(t, ch)

	// Should get: delta("Hello"), error (malformed JSON).
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	if events[0].Type != "delta" {
		t.Errorf("first event should be delta, got %q", events[0].Type)
	}
	if events[1].Type != "error" {
		t.Errorf("second event should be error, got %q", events[1].Type)
	}
}

func TestOpenAI_Name(t *testing.T) {
	tests := []struct {
		baseURL  string
		expected string
	}{
		{"https://api.x.ai", "Grok"},
		{"https://api.openai.com", "OpenAI"},
		{"https://api.groq.com", "Groq"},
		{"http://localhost:11434", "Local"},
		{"http://127.0.0.1:8080", "Local"},
		{"https://custom-llm.example.com", "OpenAI-Compatible"},
	}

	for _, tt := range tests {
		p := NewOpenAICompatibleProvider("key", tt.baseURL, "model", 1024, 0.7)
		if got := p.Name(); got != tt.expected {
			t.Errorf("Name() for %s: expected %q, got %q", tt.baseURL, tt.expected, got)
		}
	}
}

func TestOpenAI_Models(t *testing.T) {
	p := NewOpenAICompatibleProvider("key", "https://api.x.ai", "grok-4-1-fast-reasoning", 1024, 0.7)
	models := p.Models()
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0] != "grok-4-1-fast-reasoning" {
		t.Errorf("expected 'grok-4-1-fast-reasoning', got %q", models[0])
	}
}

func TestStreamChat_ModelOverride(t *testing.T) {
	// Verify that ChatRequest.Model overrides the provider default.
	var capturedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		capturedBody = string(bodyBytes)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := newTestProvider(server.URL)
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		Model: "grok-4-latest",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	collectEvents(t, ch)

	var req openaiRequest
	if err := parseJSON(capturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	if req.Model != "grok-4-latest" {
		t.Errorf("expected model 'grok-4-latest', got %q", req.Model)
	}
}

// ── Test helpers ─────────────────────────────────────────────────────

// parseJSON is a test helper to unmarshal JSON from a string.
func parseJSON(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}
