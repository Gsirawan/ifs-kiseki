package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Gsirawan/ifs-kiseki/internal/chat"
	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/db"
	"github.com/Gsirawan/ifs-kiseki/internal/provider"
	"github.com/Gsirawan/ifs-kiseki/internal/server"
	"github.com/Gsirawan/ifs-kiseki/web"
)

// Build-time variables set via ldflags.
var (
	Version = "0.1.0"
	Commit  = "unknown"
	Date    = "unknown"
)

func main() {
	// ── Load config ──────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if config.IsFirstRun() {
		log.Printf("first run detected — saving default config to %s", config.ConfigPath())
		if err := config.Save(cfg); err != nil {
			log.Fatalf("failed to save default config: %v", err)
		}
	}

	// ── Init database ────────────────────────────────────────────
	database, err := db.InitDB(config.DBPath(), cfg.Embeddings.Dimension)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	defer database.Close()

	// ── Create LLM provider ─────────────────────────────────────
	var activeProvider provider.Provider

	providerEntry := resolveProviderEntry(cfg)
	if providerEntry.APIKey == "" {
		log.Printf("WARNING: no API key configured for provider %q — chat will not work until a key is set", cfg.Provider)
	} else {
		p, err := provider.NewFromConfig(cfg.Provider, providerEntry)
		if err != nil {
			log.Printf("ERROR: failed to create provider %q: %v — starting without LLM", cfg.Provider, err)
		} else {
			activeProvider = p
			log.Printf("provider: %s (model: %s)", activeProvider.Name(), providerEntry.Model)
		}
	}

	// ── Create chat engine ──────────────────────────────────────
	var engine *chat.Engine
	if activeProvider != nil {
		engine = chat.NewEngine(activeProvider, cfg)
	}

	// ── Create server ───────────────────────────────────────────
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
	}

	srv := server.NewServer(database, cfg, http.FS(staticFS), engine)
	srv.SetVersion(Version)
	handler := srv.SetupRoutes()

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:        addr,
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout is 0 — WebSocket and SSE connections are long-lived.
		IdleTimeout: 120 * time.Second,
	}

	// ── Start server ─────────────────────────────────────────────
	log.Printf("IFS-Kiseki v%s (%s, %s)", Version, Commit, Date)
	log.Printf("config: %s", config.ConfigPath())
	log.Printf("database: %s", config.DBPath())
	log.Printf("companion: %s", cfg.Companion.Name)
	log.Printf("listening on http://%s", addr)

	// Open browser if configured.
	if cfg.Server.OpenBrowser {
		go openBrowser(fmt.Sprintf("http://%s", addr))
	}

	// Start listening in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	// ── Graceful shutdown ────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("received %s, shutting down...", sig)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}

// resolveProviderEntry returns the ProviderEntry for the active provider name.
func resolveProviderEntry(cfg *config.Config) config.ProviderEntry {
	switch cfg.Provider {
	case "claude":
		return cfg.Providers.Claude
	case "grok":
		return cfg.Providers.Grok
	default:
		log.Printf("WARNING: unknown provider %q, falling back to claude", cfg.Provider)
		return cfg.Providers.Claude
	}
}

// openBrowser attempts to open the given URL in the default browser.
func openBrowser(url string) {
	// Small delay to let the server start accepting connections.
	time.Sleep(500 * time.Millisecond)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		log.Printf("cannot open browser on %s — visit %s manually", runtime.GOOS, url)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("failed to open browser: %v (visit %s manually)", err, url)
	}
}
