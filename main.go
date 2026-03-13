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

	"github.com/Gsirawan/ifs-kiseki/internal/config"
	"github.com/Gsirawan/ifs-kiseki/internal/db"
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

	// ── HTTP server ──────────────────────────────────────────────
	mux := http.NewServeMux()

	// Serve embedded static files at root.
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Start server ─────────────────────────────────────────────
	log.Printf("IFS-Kiseki v%s (%s, %s)", Version, Commit, Date)
	log.Printf("config: %s", config.ConfigPath())
	log.Printf("database: %s", config.DBPath())
	log.Printf("listening on http://%s", addr)

	// Open browser if configured.
	if cfg.Server.OpenBrowser {
		go openBrowser(fmt.Sprintf("http://%s", addr))
	}

	// Start listening in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
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

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("server stopped")
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
