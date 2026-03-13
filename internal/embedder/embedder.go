// Package embedder defines the embedding interface and Ollama client.
// Adapted from Kiseki's internal/ollama package — Embed + IsHealthy only.
package embedder

import "context"

// Embedder is the interface for embedding text into vectors.
// OllamaClient implements this. Tests can use a mock.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// HealthChecker extends Embedder with a health check.
type HealthChecker interface {
	Embedder
	IsHealthy(ctx context.Context) bool
}
