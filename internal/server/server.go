// Package server — HTTP server setup, routes, middleware, and helpers.
package server

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Gsirawan/ifs-kiseki/internal/chat"
	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/crisis"
	"github.com/Gsirawan/ifs-kiseki/internal/memory"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
)

// Server holds all dependencies needed by HTTP handlers.
type Server struct {
	db          *sql.DB
	cfg         *config.Config
	staticFS    http.FileSystem
	engine      *chat.Engine
	memoryStore memory.Store                // may be nil — memory is optional
	provider    provider.Provider           // may be nil — required for briefing
	crisis      *crisis.RegexCrisisDetector // may be nil — crisis detection optional
	version     string
}

// NewServer creates a Server with the given dependencies.
// memoryStore, p, and crisisDetector may be nil — each degrades gracefully
// when absent. Crisis detection is disabled when crisisDetector is nil.
func NewServer(db *sql.DB, cfg *config.Config, staticFS http.FileSystem, engine *chat.Engine, memoryStore memory.Store, p provider.Provider, crisisDetector *crisis.RegexCrisisDetector) *Server {
	return &Server{
		db:          db,
		cfg:         cfg,
		staticFS:    staticFS,
		engine:      engine,
		memoryStore: memoryStore,
		provider:    p,
		crisis:      crisisDetector,
		version:     "0.1.0", // default; override with SetVersion()
	}
}

// SetVersion sets the version string reported by the health endpoint.
func (s *Server) SetVersion(v string) {
	s.version = v
}

// SetupRoutes registers all HTTP routes and returns the top-level handler.
func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// ── API routes ──────────────────────────────────────────────
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/briefing", s.handleBriefing)
	mux.HandleFunc("GET /api/sessions", s.handleSessions)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleSessionDelete)
	mux.HandleFunc("GET /api/settings", s.handleSettingsGet)
	mux.HandleFunc("PUT /api/settings", s.handleSettingsPut)
	mux.HandleFunc("GET /api/providers", s.handleProviders)
	mux.HandleFunc("POST /api/accept-disclaimer", s.handleAcceptDisclaimer)
	mux.HandleFunc("POST /api/test-provider", s.handleTestProvider)

	// ── WebSocket — NO middleware wrapping ────────────────────
	// WebSocket upgrade requires http.Hijacker on the ResponseWriter.
	// Logging middleware wraps ResponseWriter and strips that interface.
	// Solution: register the WebSocket route directly on the mux,
	// and apply logging middleware only to non-WebSocket routes.
	wsHandler := NewWebSocketHandler(s.engine, s.crisis)
	mux.HandleFunc("/ws", wsHandler.HandleWebSocket)

	// ── Static files (SPA) ──────────────────────────────────────
	fileServer := http.FileServer(s.staticFS)
	mux.Handle("/", fileServer)

	// Wrap the entire mux with middleware, but the middleware
	// skips logging for WebSocket upgrade requests.
	return s.loggingMiddleware(mux)
}

// ── Middleware ───────────────────────────────────────────────────

// loggingMiddleware logs HTTP requests. It skips wrapping the
// ResponseWriter for WebSocket upgrade requests to preserve the
// http.Hijacker interface that nhooyr.io/websocket needs.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Detect WebSocket upgrade — do NOT wrap the ResponseWriter.
		// Wrapping strips http.Hijacker, which breaks websocket.Accept().
		if isWebSocketUpgrade(r) {
			next.ServeHTTP(w, r)
			log.Printf("[http] %s %s (websocket upgrade) %s",
				r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
			return
		}

		// For normal HTTP requests, wrap to capture status code.
		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lw, r)

		log.Printf("[http] %s %s %d %s",
			r.Method, r.URL.Path, lw.statusCode, time.Since(start).Round(time.Millisecond))
	})
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// loggingResponseWriter wraps http.ResponseWriter to capture the status code.
// It does NOT implement http.Hijacker — that's intentional. WebSocket
// requests bypass this wrapper entirely (see loggingMiddleware).
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	if !w.written {
		w.statusCode = code
		w.written = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// ── JSON helpers ────────────────────────────────────────────────

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[http] failed to encode JSON response: %v", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
