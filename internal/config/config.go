// Package config — Configuration loading, saving, and defaults for IFS-Kiseki.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config is the top-level configuration for IFS-Kiseki.
type Config struct {
	Version    int              `json:"version"`
	Provider   string           `json:"provider"`
	Providers  ProvidersConfig  `json:"providers"`
	Embeddings EmbeddingsConfig `json:"embeddings"`
	Server     ServerConfig     `json:"server"`
	Companion  CompanionConfig  `json:"companion"`
	Crisis     CrisisConfig     `json:"crisis"`
	Memory     MemoryConfig     `json:"memory"`
	UI         UIConfig         `json:"ui"`
}

// ProvidersConfig holds all provider entries keyed by name.
type ProvidersConfig struct {
	Claude ProviderEntry `json:"claude"`
	Grok   ProviderEntry `json:"grok"`
}

// ProviderEntry is the configuration for a single LLM provider.
type ProviderEntry struct {
	Model       string  `json:"model"`
	BaseURL     string  `json:"base_url"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
}

// EmbeddingsConfig holds Ollama embedding settings.
type EmbeddingsConfig struct {
	OllamaHost string `json:"ollama_host"`
	Model      string `json:"model"`
	Dimension  int    `json:"dimension"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	OpenBrowser bool   `json:"open_browser"`
}

// CompanionConfig holds the IFS companion personality settings.
type CompanionConfig struct {
	Name               string   `json:"name"`
	FocusAreas         []string `json:"focus_areas"`
	UserName           string   `json:"user_name"`
	CustomInstructions string   `json:"custom_instructions"`
}

// CrisisConfig holds crisis detection settings.
type CrisisConfig struct {
	Enabled        bool   `json:"enabled"`
	HotlineCountry string `json:"hotline_country"`
}

// MemoryConfig holds memory/session persistence settings.
type MemoryConfig struct {
	AutoSave         bool `json:"auto_save"`
	BriefingOnStart  bool `json:"briefing_on_start"`
	MaxContextChunks int  `json:"max_context_chunks"`
}

// UIConfig holds frontend display settings.
type UIConfig struct {
	Theme    string `json:"theme"`
	FontSize string `json:"font_size"`
}

func init() {
	// Load .env file if present (ignore error — file is optional).
	_ = godotenv.Load()
}

// ConfigDir returns the IFS-Kiseki configuration directory.
// Respects XDG_CONFIG_HOME; falls back to ~/.config/ifs-kiseki/.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ifs-kiseki")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Last resort fallback — should never happen in practice.
		return filepath.Join(".", ".config", "ifs-kiseki")
	}
	return filepath.Join(home, ".config", "ifs-kiseki")
}

// ConfigPath returns the full path to config.json.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

// DBPath returns the full path to the SQLite database.
func DBPath() string {
	return filepath.Join(ConfigDir(), "ifs-kiseki.db")
}

// DefaultConfig returns a Config with sensible defaults matching config.example.json.
func DefaultConfig() *Config {
	return &Config{
		Version:  1,
		Provider: "claude",
		Providers: ProvidersConfig{
			Claude: ProviderEntry{
				Model:       "claude-sonnet-4-20250514",
				BaseURL:     "https://api.anthropic.com",
				MaxTokens:   4096,
				Temperature: 0.7,
			},
			Grok: ProviderEntry{
				Model:       "grok-3",
				BaseURL:     "https://api.x.ai",
				MaxTokens:   4096,
				Temperature: 0.7,
			},
		},
		Embeddings: EmbeddingsConfig{
			OllamaHost: "localhost:11434",
			Model:      "qwen3-embedding:0.6b",
			Dimension:  1024,
		},
		Server: ServerConfig{
			Host:        "127.0.0.1",
			Port:        3737,
			OpenBrowser: true,
		},
		Companion: CompanionConfig{
			Name:               "Kira",
			FocusAreas:         []string{"anxiety", "perfectionism"},
			UserName:           "",
			CustomInstructions: "",
		},
		Crisis: CrisisConfig{
			Enabled:        true,
			HotlineCountry: "US",
		},
		Memory: MemoryConfig{
			AutoSave:         true,
			BriefingOnStart:  true,
			MaxContextChunks: 5,
		},
		UI: UIConfig{
			Theme:    "warm",
			FontSize: "medium",
		},
	}
}

// IsFirstRun returns true if the config file does not yet exist.
func IsFirstRun() bool {
	_, err := os.Stat(ConfigPath())
	return os.IsNotExist(err)
}

// Load reads config from ConfigPath. If the file doesn't exist, returns DefaultConfig.
func Load() (*Config, error) {
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	cfg := DefaultConfig() // start from defaults so missing fields get sane values
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the config to ConfigPath, creating the directory if needed.
func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigPath(), data, 0o600)
}
