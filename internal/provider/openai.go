// Package provider — OpenAI-compatible streaming client.
// Works with Grok (api.x.ai), OpenAI, Groq, Ollama, and any provider
// that implements the OpenAI chat completions API format.
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
	"time"
)

// OpenAICompatibleProvider implements Provider for any OpenAI-compatible API.
type OpenAICompatibleProvider struct {
	apiKey      string
	baseURL     string
	model       string
	maxTokens   int
	temperature float64
	name        string
	httpClient  *http.Client
}

// Compile-time check: *OpenAICompatibleProvider implements Provider.
var _ Provider = (*OpenAICompatibleProvider)(nil)

// NewOpenAICompatibleProvider creates a new OpenAI-compatible provider.
// baseURL should NOT include the /v1/chat/completions path — just the host
// (e.g. "https://api.x.ai" or "https://api.openai.com").
func NewOpenAICompatibleProvider(apiKey, baseURL, model string, maxTokens int, temperature float64) *OpenAICompatibleProvider {
	// Derive a display name from the base URL.
	name := deriveProviderName(baseURL)

	return &OpenAICompatibleProvider{
		apiKey:      apiKey,
		baseURL:     strings.TrimRight(baseURL, "/"),
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		name:        name,
		httpClient: &http.Client{
			// No timeout — streaming responses can take a long time.
			// Context cancellation handles timeouts instead.
			Timeout: 0,
		},
	}
}

// deriveProviderName guesses a display name from the base URL.
func deriveProviderName(baseURL string) string {
	switch {
	case strings.Contains(baseURL, "x.ai"):
		return "Grok"
	case strings.Contains(baseURL, "openai.com"):
		return "OpenAI"
	case strings.Contains(baseURL, "groq.com"):
		return "Groq"
	case strings.Contains(baseURL, "localhost"), strings.Contains(baseURL, "127.0.0.1"):
		return "Local"
	default:
		return "OpenAI-Compatible"
	}
}

// Name returns the provider display name.
func (p *OpenAICompatibleProvider) Name() string {
	return p.name
}

// Models returns available model IDs.
func (p *OpenAICompatibleProvider) Models() []string {
	return []string{p.model}
}

// ── OpenAI API request/response types ────────────────────────────────

// openaiMessage is a single message in the OpenAI chat format.
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openaiRequest is the request body for /v1/chat/completions.
type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
	Stream      bool            `json:"stream"`
}

// openaiStreamChunk is a single SSE chunk from the streaming response.
type openaiStreamChunk struct {
	Choices []openaiChoice `json:"choices"`
	Usage   *openaiUsage   `json:"usage,omitempty"`
}

// openaiChoice is a single choice in a streaming chunk.
type openaiChoice struct {
	Index        int         `json:"index"`
	Delta        openaiDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason,omitempty"`
}

// openaiDelta is the incremental content in a streaming choice.
type openaiDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// openaiUsage is the token usage from the API response.
type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openaiErrorResponse is the error format returned by OpenAI-compatible APIs.
type openaiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// ── StreamChat ───────────────────────────────────────────────────────

// StreamChat sends a chat request and streams response tokens back.
// System prompt is injected as the first message with role "system".
func (p *OpenAICompatibleProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	// Build messages array: system prompt first, then conversation messages.
	messages := p.buildMessages(req)

	body := openaiRequest{
		Model:       p.resolveModel(req.Model),
		Messages:    messages,
		MaxTokens:   p.resolveMaxTokens(req.MaxTokens),
		Temperature: p.resolveTemperature(req.Temperature),
		Stream:      true,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", p.name, err)
	}

	endpoint := p.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("%s: create request: %w", p.name, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", p.name, err)
	}

	// Check for HTTP errors before starting the stream.
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, p.parseHTTPError(resp)
	}

	ch := make(chan StreamEvent, 64)

	go p.readSSEStream(ctx, resp, ch)

	return ch, nil
}

// buildMessages constructs the OpenAI messages array.
// System prompt becomes the first message with role "system".
func (p *OpenAICompatibleProvider) buildMessages(req ChatRequest) []openaiMessage {
	var messages []openaiMessage

	// System prompt as first message — this is the key difference from Anthropic.
	if req.SystemPrompt != "" {
		messages = append(messages, openaiMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	// Append conversation messages.
	for _, msg := range req.Messages {
		messages = append(messages, openaiMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return messages
}

// resolveModel returns the request model or falls back to the provider default.
func (p *OpenAICompatibleProvider) resolveModel(model string) string {
	if model != "" {
		return model
	}
	return p.model
}

// resolveMaxTokens returns the request max tokens or falls back to the provider default.
func (p *OpenAICompatibleProvider) resolveMaxTokens(maxTokens int) int {
	if maxTokens > 0 {
		return maxTokens
	}
	return p.maxTokens
}

// resolveTemperature returns the request temperature or falls back to the provider default.
func (p *OpenAICompatibleProvider) resolveTemperature(temperature float64) float64 {
	if temperature > 0 {
		return temperature
	}
	return p.temperature
}

// parseHTTPError reads an error response body and returns a descriptive error.
func (p *OpenAICompatibleProvider) parseHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp openaiErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		return fmt.Errorf("%s: API error %d: %s (type: %s)",
			p.name, resp.StatusCode, errResp.Error.Message, errResp.Error.Type)
	}

	return fmt.Errorf("%s: API error %d: %s", p.name, resp.StatusCode, string(body))
}

// readSSEStream reads the SSE stream from the HTTP response and sends
// StreamEvents to the channel. Closes both the response body and channel
// when done.
func (p *OpenAICompatibleProvider) readSSEStream(ctx context.Context, resp *http.Response, ch chan<- StreamEvent) {
	defer resp.Body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer size for potentially large SSE lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	var usage *Usage

	for scanner.Scan() {
		// Check for context cancellation.
		select {
		case <-ctx.Done():
			p.sendEvent(ch, StreamEvent{
				Type:  "error",
				Error: ctx.Err(),
			})
			return
		default:
		}

		line := scanner.Text()

		// Skip empty lines (SSE separator).
		if line == "" {
			continue
		}

		// Only process lines starting with "data: ".
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream termination.
		if data == "[DONE]" {
			p.sendEvent(ch, StreamEvent{
				Type:  "done",
				Usage: usage,
			})
			return
		}

		// Parse the JSON chunk.
		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			p.sendEvent(ch, StreamEvent{
				Type:  "error",
				Error: fmt.Errorf("%s: parse SSE chunk: %w", p.name, err),
			})
			return
		}

		// Extract usage if present (some providers send it in the final chunk).
		if chunk.Usage != nil {
			usage = &Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
		}

		// Extract content delta from the first choice.
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if delta.Content != "" {
				p.sendEvent(ch, StreamEvent{
					Type:  "delta",
					Delta: delta.Content,
				})
			}
			// Role-only deltas (e.g. {"role":"assistant"}) are silently skipped —
			// they carry no content for the user.
		}
	}

	// Scanner error — connection dropped or read failure.
	if err := scanner.Err(); err != nil {
		p.sendEvent(ch, StreamEvent{
			Type:  "error",
			Error: fmt.Errorf("%s: read stream: %w", p.name, err),
		})
		return
	}

	// Stream ended without [DONE] — unusual but not necessarily an error.
	// Send done with whatever usage we collected.
	p.sendEvent(ch, StreamEvent{
		Type:  "done",
		Usage: usage,
	})
}

// sendEvent sends an event to the channel, respecting context cancellation.
func (p *OpenAICompatibleProvider) sendEvent(ch chan<- StreamEvent, event StreamEvent) {
	// Use a short timeout to avoid goroutine leaks if the consumer is gone.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case ch <- event:
	case <-timer.C:
		// Consumer is gone — drop the event.
	}
}
