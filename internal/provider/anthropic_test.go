package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// anthropicSSEStream builds a valid Anthropic SSE stream from the given text chunks.
// It produces: message_start → content_block_start → ping → content_block_delta(s) →
// content_block_stop → message_delta → message_stop.
func anthropicSSEStream(chunks []string, inputTokens, outputTokens int) string {
	var b strings.Builder

	// message_start
	b.WriteString(fmt.Sprintf(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-20250514\",\"stop_reason\":null,\"usage\":{\"input_tokens\":%d,\"output_tokens\":1}}}\n\n",
		inputTokens,
	))

	// content_block_start
	b.WriteString("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")

	// ping
	b.WriteString("event: ping\ndata: {\"type\":\"ping\"}\n\n")

	// content_block_delta for each chunk
	for _, chunk := range chunks {
		escaped, _ := json.Marshal(chunk)
		b.WriteString(fmt.Sprintf(
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n",
			string(escaped),
		))
	}

	// content_block_stop
	b.WriteString("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")

	// message_delta with usage
	b.WriteString(fmt.Sprintf(
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":%d}}\n\n",
		outputTokens,
	))

	// message_stop
	b.WriteString("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")

	return b.String()
}

func TestAnthropicStreamChat_Success(t *testing.T) {
	chunks := []string{"Hello", ", ", "world", "!"}
	inputTokens := 25
	outputTokens := 15

	// Create mock server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers.
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("expected x-api-key 'test-key', got %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("expected anthropic-version '2023-06-01', got %q", got)
		}
		if got := r.Header.Get("content-type"); got != "application/json" {
			t.Errorf("expected content-type 'application/json', got %q", got)
		}

		// Verify request body.
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if !reqBody.Stream {
			t.Error("expected stream=true in request body")
		}
		if len(reqBody.Messages) != 1 || reqBody.Messages[0].Content != "Hi" {
			t.Errorf("unexpected messages: %+v", reqBody.Messages)
		}
		if reqBody.System != "You are helpful." {
			t.Errorf("expected system prompt 'You are helpful.', got %q", reqBody.System)
		}

		// Write SSE response.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		stream := anthropicSSEStream(chunks, inputTokens, outputTokens)
		fmt.Fprint(w, stream)
	}))
	defer server.Close()

	// Create provider pointing at mock server.
	p := NewAnthropicProvider("test-key", server.URL, "claude-sonnet-4-20250514", 4096, 0.7)

	ctx := context.Background()
	ch, err := p.StreamChat(ctx, ChatRequest{
		SystemPrompt: "You are helpful.",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat failed: %v", err)
	}

	// Collect all events.
	var deltas []string
	var doneEvent *StreamEvent
	var errorEvents []StreamEvent

	for evt := range ch {
		switch evt.Type {
		case "delta":
			deltas = append(deltas, evt.Delta)
		case "done":
			doneEvent = &evt
		case "error":
			errorEvents = append(errorEvents, evt)
		}
	}

	// Verify no errors.
	if len(errorEvents) > 0 {
		t.Errorf("unexpected errors: %v", errorEvents)
	}

	// Verify delta text.
	fullText := strings.Join(deltas, "")
	if fullText != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", fullText)
	}

	// Verify done event with usage.
	if doneEvent == nil {
		t.Fatal("expected done event, got none")
	}
	if doneEvent.Usage == nil {
		t.Fatal("expected usage in done event")
	}
	if doneEvent.Usage.InputTokens != inputTokens {
		t.Errorf("expected input_tokens=%d, got %d", inputTokens, doneEvent.Usage.InputTokens)
	}
	if doneEvent.Usage.OutputTokens != outputTokens {
		t.Errorf("expected output_tokens=%d, got %d", outputTokens, doneEvent.Usage.OutputTokens)
	}
}

func TestAnthropicStreamChat_ContextCancellation(t *testing.T) {
	// Create a server that streams slowly.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send message_start.
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-20250514\",\"stop_reason\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n")

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Send one delta.
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Block until client disconnects — simulates a long stream.
		<-r.Context().Done()
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL, "claude-sonnet-4-20250514", 4096, 0.7)

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := p.StreamChat(ctx, ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat failed: %v", err)
	}

	// Read the first delta.
	evt := <-ch
	if evt.Type != "delta" || evt.Delta != "Hello" {
		t.Errorf("expected delta 'Hello', got %+v", evt)
	}

	// Cancel context.
	cancel()

	// Drain remaining events — should get an error or channel close.
	var gotError bool
	timeout := time.After(5 * time.Second)
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				goto done
			}
			if evt.Type == "error" {
				gotError = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for channel to close after cancellation")
		}
	}
done:
	if !gotError {
		// It's acceptable if the channel just closes without an explicit error
		// event, since the scanner may return cleanly on connection close.
		t.Log("channel closed without explicit error event (acceptable)")
	}
}

func TestAnthropicStreamChat_APIError(t *testing.T) {
	// Create a server that returns a non-200 status.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	}))
	defer server.Close()

	p := NewAnthropicProvider("bad-key", server.URL, "claude-sonnet-4-20250514", 4096, 0.7)

	ctx := context.Background()
	_, err := p.StreamChat(ctx, ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to contain '401', got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Errorf("expected error to contain 'invalid x-api-key', got: %v", err)
	}
}

func TestAnthropicStreamChat_SSEErrorEvent(t *testing.T) {
	// Create a server that sends an error event in the stream.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-20250514\",\"stop_reason\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n")
		fmt.Fprint(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n")
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key", server.URL, "claude-sonnet-4-20250514", 4096, 0.7)

	ch, err := p.StreamChat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat failed: %v", err)
	}

	var gotError bool
	for evt := range ch {
		if evt.Type == "error" && strings.Contains(evt.Error.Error(), "overloaded_error") {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected to receive an overloaded_error event")
	}
}

func TestAnthropicName(t *testing.T) {
	p := NewAnthropicProvider("key", "http://localhost", "model", 4096, 0.7)
	if p.Name() != "Claude" {
		t.Errorf("expected Name()='Claude', got %q", p.Name())
	}
}

func TestAnthropicModels(t *testing.T) {
	p := NewAnthropicProvider("key", "http://localhost", "model", 4096, 0.7)
	models := p.Models()
	if len(models) == 0 {
		t.Error("expected at least one model")
	}
	// Verify the default model is in the list.
	found := false
	for _, m := range models {
		if m == "claude-sonnet-4-20250514" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected claude-sonnet-4-20250514 in models list")
	}
}
