// Package provider — Anthropic (Claude) streaming chat provider.
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// anthropicAPIVersion is the Anthropic API version header value.
// See: https://docs.anthropic.com/en/api/versioning
const anthropicAPIVersion = "2023-06-01"

// Compile-time check: *AnthropicProvider implements Provider.
var _ Provider = (*AnthropicProvider)(nil)

// AnthropicProvider implements the Provider interface for Anthropic's Messages API.
type AnthropicProvider struct {
	apiKey      string
	baseURL     string
	model       string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider with the given configuration.
func NewAnthropicProvider(apiKey, baseURL, model string, maxTokens int, temperature float64) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:      apiKey,
		baseURL:     baseURL,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient: &http.Client{
			Timeout: 0, // no timeout — streaming responses can be long-lived
		},
	}
}

// Name returns the provider display name.
func (p *AnthropicProvider) Name() string {
	return "Claude"
}

// Models returns available Claude model IDs.
func (p *AnthropicProvider) Models() []string {
	return []string{
		"claude-sonnet-4-20250514",
		"claude-opus-4-20250514",
		"claude-haiku-3-5-20241022",
	}
}

// anthropicRequest is the request body for the Anthropic Messages API.
type anthropicRequest struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	System      string        `json:"system,omitempty"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature"`
}

// anthropicSSEEvent represents a parsed SSE event from the Anthropic stream.
type anthropicSSEEvent struct {
	EventType string
	Data      string
}

// anthropicContentBlockDelta is the data payload for content_block_delta events.
type anthropicContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

// anthropicMessageDelta is the data payload for message_delta events.
type anthropicMessageDelta struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// anthropicMessageStart is the data payload for message_start events.
type anthropicMessageStart struct {
	Type    string `json:"type"`
	Message struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// anthropicErrorEvent is the data payload for error events.
type anthropicErrorEvent struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// StreamChat sends a chat request to the Anthropic Messages API and streams
// response tokens back through the returned channel.
func (p *AnthropicProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	// Build request body.
	model := req.Model
	if model == "" {
		model = p.model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}
	temp := req.Temperature
	if temp == 0 {
		temp = p.temperature
	}

	body := anthropicRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		System:      req.SystemPrompt,
		Messages:    req.Messages,
		Stream:      true,
		Temperature: temp,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Build HTTP request.
	url := strings.TrimRight(p.baseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("content-type", "application/json")

	// Send request.
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Check for non-200 status.
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	// Create channel and start streaming goroutine.
	ch := make(chan StreamEvent, 32)
	go p.streamResponse(ctx, resp, ch)

	return ch, nil
}

// streamResponse reads SSE events from the response body and sends StreamEvents
// to the channel. It closes the channel and response body when done.
func (p *AnthropicProvider) streamResponse(ctx context.Context, resp *http.Response, ch chan<- StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()

	var inputTokens int

	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer size for potentially large SSE data lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentEvent string

	for scanner.Scan() {
		// Check for context cancellation.
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: "error", Error: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()

		// SSE format: "event: <type>" followed by "data: <json>" followed by empty line.
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			p.handleSSEData(currentEvent, data, &inputTokens, ch)
			currentEvent = ""
			continue
		}

		// Empty lines are SSE event separators — skip them.
		// Lines not matching event/data prefixes (like comments) are also skipped.
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Type: "error", Error: fmt.Errorf("read stream: %w", err)}
	}
}

// handleSSEData processes a single SSE data payload based on the event type.
func (p *AnthropicProvider) handleSSEData(eventType, data string, inputTokens *int, ch chan<- StreamEvent) {
	switch eventType {
	case "message_start":
		var msg anthropicMessageStart
		if err := json.Unmarshal([]byte(data), &msg); err == nil {
			*inputTokens = msg.Message.Usage.InputTokens
		}

	case "content_block_delta":
		var delta anthropicContentBlockDelta
		if err := json.Unmarshal([]byte(data), &delta); err != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("parse content_block_delta: %w", err)}
			return
		}
		if delta.Delta.Type == "text_delta" {
			ch <- StreamEvent{Type: "delta", Delta: delta.Delta.Text}
		}

	case "message_delta":
		var md anthropicMessageDelta
		if err := json.Unmarshal([]byte(data), &md); err != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("parse message_delta: %w", err)}
			return
		}
		// Store output tokens — will be sent with the done event.
		// The message_stop event follows immediately after.
		ch <- StreamEvent{
			Type: "done",
			Usage: &Usage{
				InputTokens:  *inputTokens,
				OutputTokens: md.Usage.OutputTokens,
			},
		}

	case "message_stop":
		// Stream is complete. Channel will be closed by deferred close.
		return

	case "error":
		var errEvt anthropicErrorEvent
		if err := json.Unmarshal([]byte(data), &errEvt); err != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("parse error event: %w", err)}
			return
		}
		ch <- StreamEvent{
			Type:  "error",
			Error: fmt.Errorf("anthropic error (%s): %s", errEvt.Error.Type, errEvt.Error.Message),
		}

	case "ping", "content_block_start", "content_block_stop":
		// Ignored — no action needed.

	default:
		// Unknown event types are silently ignored per Anthropic's versioning policy.
	}
}
