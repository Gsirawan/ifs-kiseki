// Package provider — Provider registry: maps provider names to constructor functions.
package provider

import (
	"fmt"
	"sort"

	"github.com/Gsirawan/ifs-kiseki/internal/config"
)

// ProviderFactory creates a Provider from connection parameters.
type ProviderFactory func(apiKey, baseURL, model string, maxTokens int, temperature float64) Provider

// registry maps provider names (e.g. "claude", "grok") to their factory functions.
var registry = map[string]ProviderFactory{}

// Register adds a provider factory to the registry.
// Panics if a factory with the same name is already registered (programming error).
func Register(name string, factory ProviderFactory) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("provider: duplicate registration for %q", name))
	}
	registry[name] = factory
}

// Get retrieves a provider factory by name.
func Get(name string) (ProviderFactory, bool) {
	f, ok := registry[name]
	return f, ok
}

// Available returns a sorted list of registered provider names.
func Available() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NewFromConfig creates a Provider from a registry name and config entry.
// Returns an error if the provider name is not registered.
func NewFromConfig(name string, entry config.ProviderEntry) (Provider, error) {
	factory, ok := Get(name)
	if !ok {
		return nil, fmt.Errorf("provider: unknown provider %q (available: %v)", name, Available())
	}
	return factory(entry.APIKey, entry.BaseURL, entry.Model, entry.MaxTokens, entry.Temperature), nil
}

func init() {
	Register("claude", func(apiKey, baseURL, model string, maxTokens int, temperature float64) Provider {
		return NewAnthropicProvider(apiKey, baseURL, model, maxTokens, temperature)
	})

	Register("grok", func(apiKey, baseURL, model string, maxTokens int, temperature float64) Provider {
		return NewOpenAICompatibleProvider(apiKey, baseURL, model, maxTokens, temperature)
	})
}
