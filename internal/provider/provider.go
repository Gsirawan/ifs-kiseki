// Package provider defines the LLM provider abstraction and shared types.
// V1 has two implementations: AnthropicProvider (Claude) and OpenAICompatibleProvider (Grok).
// The OpenAI-compatible client covers all future providers (Ollama, GPT, Groq, etc).
package provider

import "context"

// Provider is the LLM chat provider abstraction.
type Provider interface {
	// Name returns the provider display name (e.g. "Claude", "Grok").
	Name() string

	// StreamChat sends a multi-turn conversation and streams response tokens
	// back through the returned channel. The channel receives StreamEvent values
	// and is closed when the response is complete or an error occurs.
	StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)

	// Models returns available model IDs for this provider.
	Models() []string
}

// ChatRequest is the input to StreamChat.
type ChatRequest struct {
	SystemPrompt string
	Messages     []ChatMessage
	Model        string
	MaxTokens    int
	Temperature  float64
}

// ChatMessage is a single turn in the conversation.
type ChatMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"` // message text
}

// StreamEvent is a chunk from the streaming response.
type StreamEvent struct {
	Type  string // "delta", "done", "error"
	Delta string // text chunk (for Type="delta")
	Error error  // error details (for Type="error")
	Usage *Usage // token usage (for Type="done")
}

// Usage tracks token consumption for a single response.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
