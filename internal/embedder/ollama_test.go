package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeEmbedding returns a float64 slice of the given length filled with 0.1.
// Used to produce deterministic fake embeddings in tests.
func fakeEmbedding(dim int) []float64 {
	v := make([]float64, dim)
	for i := range v {
		v[i] = 0.1
	}
	return v
}

// serveEmbed returns an http.HandlerFunc that responds to /api/embed with a
// fake embedding of the given dimension, and to /api/tags with HTTP 200.
func serveEmbed(dim int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/embed":
			resp := embedResponse{
				Embeddings: [][]float64{fakeEmbedding(dim)},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case "/api/tags":
			w.WriteHeader(http.StatusOK)

		default:
			http.NotFound(w, r)
		}
	}
}

// ── Embed: happy path ────────────────────────────────────────────────────────

func TestEmbed_ValidResponse(t *testing.T) {
	const dim = 8
	srv := httptest.NewServer(serveEmbed(dim))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", dim)
	vec, err := client.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() unexpected error: %v", err)
	}

	if len(vec) != dim {
		t.Errorf("Embed() returned %d dimensions, want %d", len(vec), dim)
	}

	// Every value should be float32(0.1).
	for i, v := range vec {
		if v != float32(0.1) {
			t.Errorf("vec[%d] = %v, want %v", i, v, float32(0.1))
		}
	}
}

// ── Embed: Ollama unreachable ────────────────────────────────────────────────

func TestEmbed_OllamaDown(t *testing.T) {
	// Point at a port nothing is listening on.
	client := NewOllamaClient("http://127.0.0.1:19999", "test-model", 8)
	_, err := client.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("Embed() expected error when Ollama is down, got nil")
	}

	// Error must contain actionable guidance.
	if !strings.Contains(err.Error(), "ollama serve") {
		t.Errorf("error message missing 'ollama serve' hint: %v", err)
	}
}

// ── Embed: model not found ───────────────────────────────────────────────────

func TestEmbed_ModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embed" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`model "ghost-model" not found`))
		}
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "ghost-model", 8)
	_, err := client.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("Embed() expected error for missing model, got nil")
	}

	// Error must contain the model name and pull hint.
	if !strings.Contains(err.Error(), "ghost-model") {
		t.Errorf("error message missing model name: %v", err)
	}
	if !strings.Contains(err.Error(), "ollama pull") {
		t.Errorf("error message missing 'ollama pull' hint: %v", err)
	}
}

// ── Embed: dimension mismatch ────────────────────────────────────────────────

func TestEmbed_DimensionMismatch(t *testing.T) {
	// Server returns 4-dim vectors; client expects 8.
	srv := httptest.NewServer(serveEmbed(4))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 8)
	_, err := client.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("Embed() expected dimension mismatch error, got nil")
	}

	if !strings.Contains(err.Error(), "dimension mismatch") {
		t.Errorf("error message missing 'dimension mismatch': %v", err)
	}
}

// ── Embed: zero dimension skips validation ───────────────────────────────────

func TestEmbed_ZeroDimensionSkipsValidation(t *testing.T) {
	// dimension=0 means "don't validate" — any size vector is accepted.
	const serverDim = 16
	srv := httptest.NewServer(serveEmbed(serverDim))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 0)
	vec, err := client.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed() unexpected error with dimension=0: %v", err)
	}
	if len(vec) != serverDim {
		t.Errorf("Embed() returned %d dimensions, want %d", len(vec), serverDim)
	}
}

// ── Embed: empty embeddings in response ──────────────────────────────────────

func TestEmbed_EmptyEmbeddingsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embed" {
			resp := embedResponse{Embeddings: [][]float64{}}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 8)
	_, err := client.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("Embed() expected error for empty embeddings, got nil")
	}
	if !strings.Contains(err.Error(), "no embeddings") {
		t.Errorf("error message missing 'no embeddings': %v", err)
	}
}

// ── IsHealthy ────────────────────────────────────────────────────────────────

func TestIsHealthy_Reachable(t *testing.T) {
	srv := httptest.NewServer(serveEmbed(8))
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model", 8)
	if !client.IsHealthy(context.Background()) {
		t.Error("IsHealthy() returned false for reachable server, want true")
	}
}

func TestIsHealthy_Unreachable(t *testing.T) {
	client := NewOllamaClient("http://127.0.0.1:19999", "test-model", 8)
	if client.IsHealthy(context.Background()) {
		t.Error("IsHealthy() returned true for unreachable server, want false")
	}
}

// ── Host normalisation ───────────────────────────────────────────────────────

func TestNewOllamaClient_HostNormalisation(t *testing.T) {
	cases := []struct {
		input   string
		wantURL string
	}{
		{"localhost:11434", "http://localhost:11434"},
		{"http://localhost:11434", "http://localhost:11434"},
		{"https://ollama.example.com", "https://ollama.example.com"},
		{"localhost:11434/", "http://localhost:11434"}, // trailing slash stripped
	}

	for _, tc := range cases {
		c := NewOllamaClient(tc.input, "m", 8)
		if c.baseURL != tc.wantURL {
			t.Errorf("NewOllamaClient(%q).baseURL = %q, want %q", tc.input, c.baseURL, tc.wantURL)
		}
	}
}

// ── Interface compliance ─────────────────────────────────────────────────────

// TestInterfaceCompliance verifies the compile-time assertion in ollama.go
// is correct — OllamaClient satisfies both Embedder and HealthChecker.
func TestInterfaceCompliance(t *testing.T) {
	var _ Embedder = (*OllamaClient)(nil)
	var _ HealthChecker = (*OllamaClient)(nil)
}
