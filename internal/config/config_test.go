package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigHasSaneValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
	if cfg.Provider != "claude" {
		t.Errorf("expected provider 'claude', got %q", cfg.Provider)
	}
	if cfg.Server.Port != 3737 {
		t.Errorf("expected port 3737, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host '127.0.0.1', got %q", cfg.Server.Host)
	}
	if cfg.Embeddings.Dimension != 1024 {
		t.Errorf("expected embed dim 1024, got %d", cfg.Embeddings.Dimension)
	}
	if cfg.Companion.Name != "Kira" {
		t.Errorf("expected companion name 'Kira', got %q", cfg.Companion.Name)
	}
	if !cfg.Crisis.Enabled {
		t.Error("expected crisis detection enabled by default")
	}
	if !cfg.Memory.AutoSave {
		t.Error("expected auto_save enabled by default")
	}
	if cfg.UI.Theme != "warm" {
		t.Errorf("expected theme 'warm', got %q", cfg.UI.Theme)
	}
	if len(cfg.Companion.FocusAreas) != 2 {
		t.Errorf("expected 2 focus areas, got %d", len(cfg.Companion.FocusAreas))
	}
	if cfg.Providers.Claude.Model == "" {
		t.Error("expected Claude model to be set")
	}
	if cfg.Providers.Grok.Model == "" {
		t.Error("expected Grok model to be set")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	// Use a temp dir to avoid touching real config.
	tmpDir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	cfg := DefaultConfig()
	cfg.Companion.Name = "TestCompanion"
	cfg.Server.Port = 9999

	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Companion.Name != "TestCompanion" {
		t.Errorf("expected companion name 'TestCompanion', got %q", loaded.Companion.Name)
	}
	if loaded.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", loaded.Server.Port)
	}
	// Verify defaults survived the round-trip.
	if loaded.Version != 1 {
		t.Errorf("expected version 1 after round-trip, got %d", loaded.Version)
	}
	if loaded.Embeddings.Dimension != 1024 {
		t.Errorf("expected embed dim 1024 after round-trip, got %d", loaded.Embeddings.Dimension)
	}
}

func TestIsFirstRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	if !IsFirstRun() {
		t.Error("expected IsFirstRun=true before any config is saved")
	}

	cfg := DefaultConfig()
	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if IsFirstRun() {
		t.Error("expected IsFirstRun=false after config is saved")
	}
}

func TestXDGConfigHomeOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	expected := filepath.Join(tmpDir, "ifs-kiseki")
	got := ConfigDir()
	if got != expected {
		t.Errorf("expected ConfigDir=%q, got %q", expected, got)
	}
}

func TestConfigPathAndDBPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := ConfigDir()
	if ConfigPath() != filepath.Join(dir, "config.json") {
		t.Errorf("ConfigPath mismatch: %q", ConfigPath())
	}
	if DBPath() != filepath.Join(dir, "ifs-kiseki.db") {
		t.Errorf("DBPath mismatch: %q", DBPath())
	}
}

func TestLoadReturnsDefaultsWhenNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Provider != "claude" {
		t.Errorf("expected default provider 'claude', got %q", cfg.Provider)
	}
	if cfg.Server.Port != 3737 {
		t.Errorf("expected default port 3737, got %d", cfg.Server.Port)
	}
}
