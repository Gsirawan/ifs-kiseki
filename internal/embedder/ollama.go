// Package embedder — Ollama embedding client for IFS-Kiseki.
// Adapted from Kiseki's internal/ollama/ollama.go — Embed + IsHealthy only.
// GenerateAnswer is intentionally omitted: IFS-Kiseki uses cloud LLMs for generation.
package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Compile-time check: *OllamaClient implements HealthChecker (which embeds Embedder).
var _ HealthChecker = (*OllamaClient)(nil)

// OllamaClient calls Ollama's /api/embed endpoint to produce float32 vectors.
// It is constructed from config.EmbeddingsConfig values via NewOllamaClient.
type OllamaClient struct {
	baseURL    string // e.g. "http://localhost:11434"
	model      string // e.g. "qwen3-embedding:0.6b"
	dimension  int    // expected vector dimension, e.g. 1024
	httpClient *http.Client
}

// NewOllamaClient creates an OllamaClient.
//
// host is the Ollama host as stored in config (e.g. "localhost:11434" or
// "http://localhost:11434"). If no scheme is present, "http://" is prepended.
// model is the embedding model name. dimension is the expected vector length —
// used to validate responses so callers catch misconfiguration early.
func NewOllamaClient(host, model string, dimension int) *OllamaClient {
	baseURL := host
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	// Strip any trailing slash so URL construction is consistent.
	baseURL = strings.TrimRight(baseURL, "/")

	return &OllamaClient{
		baseURL:   baseURL,
		model:     model,
		dimension: dimension,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// embedRequest is the JSON body sent to /api/embed.
type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// embedResponse is the JSON body returned by /api/embed.
type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// Embed calls Ollama's /api/embed endpoint and returns a float32 vector.
// It validates that the returned dimension matches the configured dimension.
func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := embedRequest{
		Model: c.model,
		Input: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[embedder] marshal embed request: %v", err)
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		log.Printf("[embedder] create embed request: %v", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, wrapConnectionError(c.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		bodyStr := string(respBody)
		if resp.StatusCode == http.StatusNotFound || strings.Contains(bodyStr, "not found") {
			return nil, wrapModelNotFoundError(c.model)
		}
		return nil, fmt.Errorf("ollama embed returned status %d: %s", resp.StatusCode, bodyStr)
	}

	var respData embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		log.Printf("[embedder] decode embed response: %v", err)
		return nil, err
	}

	if len(respData.Embeddings) == 0 {
		log.Printf("[embedder] embed response has no embeddings")
		return nil, fmt.Errorf("no embeddings in response")
	}

	// Convert first embedding from float64 to float32.
	embedding := respData.Embeddings[0]

	// Validate dimension — catch model/config mismatch early.
	if c.dimension > 0 && len(embedding) != c.dimension {
		return nil, fmt.Errorf(
			"embedding dimension mismatch: got %d, expected %d (check embeddings.model and embeddings.dimension in config)",
			len(embedding), c.dimension,
		)
	}

	result := make([]float32, len(embedding))
	for i, v := range embedding {
		result[i] = float32(v)
	}

	return result, nil
}

// IsHealthy checks if Ollama is reachable by calling /api/tags.
// Returns true only when Ollama responds with HTTP 200.
func (c *OllamaClient) IsHealthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		log.Printf("[embedder] create health check request: %v", err)
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[embedder] health check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// wrapConnectionError wraps connection errors with actionable guidance.
func wrapConnectionError(baseURL string, err error) error {
	return fmt.Errorf("cannot connect to Ollama at %s — start it with: ollama serve\n  details: %w", baseURL, err)
}

// wrapModelNotFoundError wraps model-not-found errors with actionable guidance.
func wrapModelNotFoundError(model string) error {
	return fmt.Errorf("model %q not found in Ollama — pull it with: ollama pull %s", model, model)
}
