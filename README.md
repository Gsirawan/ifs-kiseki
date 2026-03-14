# IFS-Kiseki

A standalone self-exploration companion powered by Internal Family Systems (IFS) principles.

Single Go binary. Local data. Cloud LLM. No subscriptions, no servers, no accounts.

---

## What It Is

IFS-Kiseki is a private, local-first chat companion for IFS-informed self-exploration. It runs entirely on your machine — a small HTTP server that opens in your browser. Your conversation history is stored in a local SQLite database. Nothing is sent anywhere except your messages to the LLM provider you configure.

It is not therapy. It is a tool for self-reflection using IFS language and principles.

---

## Features

- **Streaming chat** — responses appear word-by-word via WebSocket
- **IFS-informed system prompt** — the companion uses parts language, asks about body sensations, checks for Self-energy, and does not rush toward exile work
- **Session memory** — past sessions are summarized and surfaced at the start of new conversations
- **Crisis safety** — keyword detection triggers a resource overlay that cannot be dismissed for 5 seconds
- **Onboarding flow** — first-launch disclaimer and API key setup
- **Settings** — change provider, companion name, and preferences at any time
- **Two providers** — Claude (recommended) and Grok (premium alternative)
- **Single binary** — no runtime dependencies beyond CGO (SQLite)

---

## Quick Start

### 1. Build

Requires Go 1.22+ and a C compiler (for SQLite via CGO).

```bash
make build
# or
CGO_ENABLED=1 go build -o ifs-kiseki .
```

### 2. Configure

Copy the example config and add your API key:

```bash
cp config.example.json ~/.config/ifs-kiseki/config.json
```

Edit `~/.config/ifs-kiseki/config.json` and set your API key under `providers.claude.api_key` (or use an environment variable — see below).

### 3. Run

```bash
./ifs-kiseki
```

The binary starts a local server and opens your browser to `http://127.0.0.1:3737`. On first launch, you will be shown a disclaimer and asked to accept it before proceeding.

---

## Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Compile the binary |
| `make run` | Build and run |
| `make test` | Run all tests |
| `make dev` | Run without compiling to disk (`go run`) |
| `make clean` | Remove binary and local database |

---

## Configuration

Config is stored at `~/.config/ifs-kiseki/config.json` (respects `XDG_CONFIG_HOME`).

### Top-Level Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | `1` | Config schema version |
| `provider` | string | `"claude"` | Active provider: `"claude"` or `"grok"` |
| `disclaimer_accepted` | bool | `false` | Set to `true` after first-launch acceptance |
| `disclaimer_accepted_at` | string | `""` | RFC3339 timestamp of acceptance |

### `providers.claude`

| Field | Default | Description |
|-------|---------|-------------|
| `model` | `"claude-sonnet-4-20250514"` | Model ID |
| `base_url` | `"https://api.anthropic.com"` | API base URL |
| `max_tokens` | `4096` | Max tokens per response |
| `temperature` | `0.7` | Sampling temperature |
| `api_key` | `""` | Your Anthropic API key (or use env var) |

### `providers.grok`

| Field | Default | Description |
|-------|---------|-------------|
| `model` | `"grok-4-1-fast-reasoning"` | Model ID |
| `base_url` | `"https://api.x.ai"` | API base URL |
| `max_tokens` | `4096` | Max tokens per response |
| `temperature` | `0.7` | Sampling temperature |
| `api_key` | `""` | Your xAI API key (or use env var) |

### `embeddings`

| Field | Default | Description |
|-------|---------|-------------|
| `ollama_host` | `"localhost:11434"` | Ollama server address |
| `model` | `"qwen3-embedding:0.6b"` | Embedding model name |
| `dimension` | `1024` | Embedding vector dimension |

### `server`

| Field | Default | Description |
|-------|---------|-------------|
| `host` | `"127.0.0.1"` | Bind address (localhost only by default) |
| `port` | `3737` | HTTP port |
| `open_browser` | `true` | Auto-open browser on start |

### `companion`

| Field | Default | Description |
|-------|---------|-------------|
| `name` | `"Kira"` | Companion name used in responses |
| `focus_areas` | `["anxiety", "perfectionism"]` | Areas to emphasize in the IFS prompt |
| `user_name` | `""` | Your name (optional, used in responses) |
| `custom_instructions` | `""` | Additional instructions appended to the companion section of the system prompt |

### `crisis`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `true` | Enable crisis keyword detection |
| `hotline_country` | `"US"` | Country code for crisis resource display |

### `memory`

| Field | Default | Description |
|-------|---------|-------------|
| `auto_save` | `true` | Automatically save sessions on disconnect |
| `briefing_on_start` | `true` | Generate a summary of past sessions at the start of each new session |
| `max_context_chunks` | `5` | Number of memory chunks to include in context |

### `ui`

| Field | Default | Description |
|-------|---------|-------------|
| `theme` | `"warm"` | UI theme |
| `font_size` | `"medium"` | Font size: `"small"`, `"medium"`, `"large"` |

---

## Providers

### Claude (Recommended)

Claude is the default provider and the recommended choice for IFS self-exploration work.

1. Get an API key at [console.anthropic.com](https://console.anthropic.com/)
2. Set it in config: `providers.claude.api_key`
   — or via environment variable: `ANTHROPIC_API_KEY=sk-ant-...`

### Grok

Grok (xAI) is a premium alternative provider. It uses an OpenAI-compatible API.

1. Get an API key at [console.x.ai](https://console.x.ai/)
2. Set it in config: `providers.grok.api_key`
   — or via environment variable: `XAI_API_KEY=xai-...`

To switch providers, change `provider` in config.json to `"grok"`, or use the Settings page in the UI.

---

## Environment Variables

API keys and the Ollama host can be set via environment variables. These take precedence over config.json values.

```bash
# Copy and fill in
cp .env.example .env
```

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key for Claude |
| `XAI_API_KEY` | xAI API key for Grok |
| `OLLAMA_HOST` | Ollama server address (default: `localhost:11434`) |

---

## Embeddings (Optional)

Session memory works without embeddings — sessions are saved and retrieved by recency. Embeddings enable semantic search across past sessions, surfacing relevant context even from older conversations.

To enable semantic memory:

1. Install [Ollama](https://ollama.com/)
2. Pull the embedding model:
   ```bash
   ollama pull qwen3-embedding:0.6b
   ```
3. Ollama runs at `localhost:11434` by default — no further config needed

If Ollama is not running, IFS-Kiseki falls back to recency-based memory retrieval.

---

## Crisis Safety

IFS-Kiseki includes keyword-based crisis detection. When certain phrases are detected in your messages, a resource overlay appears with crisis hotline information. The overlay cannot be dismissed for 5 seconds.

Crisis detection is enabled by default (`crisis.enabled: true`). It can be disabled in config, but this is not recommended.

This feature is not a substitute for professional crisis support. If you are in crisis, please contact a qualified professional or emergency services.

---

## Data & Privacy

- All data is stored locally in `~/.config/ifs-kiseki/ifs-kiseki.db`
- Nothing is stored on any server
- The only outbound connections are to your configured LLM provider's API
- Your API key is stored in `~/.config/ifs-kiseki/config.json` (permissions: 0600)

---

## Disclaimer

**IFS-Kiseki is not therapy and is not a substitute for professional mental health care.**

It is a self-exploration tool that uses IFS-informed language and principles. It cannot diagnose, treat, or provide clinical support. If you are experiencing a mental health crisis, please contact a licensed professional or emergency services.

By using IFS-Kiseki, you acknowledge that you are using it as a personal reflection tool, not as a therapeutic intervention.

---

## License

MIT License — see [LICENSE](LICENSE)
