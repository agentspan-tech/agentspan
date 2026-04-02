# AgentSpan

[![CI](https://github.com/agentspan-tech/agentspan/actions/workflows/ci.yml/badge.svg)](https://github.com/agentspan-tech/agentspan/actions/workflows/ci.yml)
[![Test coverage](https://codecov.io/gh/agentspan-tech/agentspan/graph/badge.svg)](https://codecov.io/gh/agentspan-tech/agentspan)

Proxy + observability layer for AI agents. Point your agent's `base_url` at AgentSpan — it proxies requests to LLM providers unchanged, collects metrics, groups requests into sessions, generates narratives, and serves a real-time dashboard.

**Zero code changes required.** Just swap the base URL.

## How It Works

```
┌─────────┐         ┌──────────┐         ┌──────────────┐
│  Agent  │ ──────▸ │  Proxy   │ ──────▸ │ LLM Provider │
│ (your   │ ◂────── │ :8080    │ ◂────── │ (OpenAI,     │
│  code)  │         │          │         │  Anthropic)  │
└─────────┘         └────┬─────┘         └──────────────┘
                         │ async
                         ▼
                    ┌──────────┐     ┌────────────┐
                    │Processing│────▸│ PostgreSQL │
                    │ :8081    │     └────────────┘
                    └──────────┘
                      Dashboard
                      REST API
                      WebSocket
```

- **Proxy** — Stateless. Forwards requests, streams SSE without buffering, sends span data to Processing async. Overhead < 50ms. Fail-open: if Processing is down, the proxy keeps working.
- **Processing** — Dashboard API, WebSocket for real-time updates, async workers (narratives, classification, alerts). Runs migrations on startup. Serves the embedded frontend.
- **Frontend** — React 19 SPA embedded in the Processing binary. Dark theme.

## Quick Start

```bash
git clone https://github.com/agentspan/agentspan.git
cd agentspan
cp .env.example .env
# Edit .env — replace all changeme_ values
docker compose up -d
```

Dashboard: [http://localhost:8081](http://localhost:8081)
Proxy: [http://localhost:8080](http://localhost:8080)

## Connect Your Agent

### OpenAI SDK (Python)

```python
from openai import OpenAI

client = OpenAI(
    api_key="as-...",          # Your AgentSpan API key
    base_url="http://localhost:8080/v1"
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello"}]
)
```

### Anthropic SDK (Python)

```python
import anthropic

client = anthropic.Anthropic(
    api_key="as-...",
    base_url="http://localhost:8080"
)

message = client.messages.create(
    model="claude-sonnet-4-20250514",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello"}]
)
```

### Supported Endpoints

| Endpoint | Provider |
|----------|----------|
| `POST /v1/chat/completions` | OpenAI-compatible |
| `POST /v1/messages` | Anthropic-compatible |

## Configuration

All configuration is via environment variables. See [`.env.example`](.env.example) for the full list.

### Required

| Variable | Description |
|----------|-------------|
| `POSTGRES_PASSWORD` | Database password |
| `DATABASE_URL` | PostgreSQL connection string |
| `JWT_SECRET` | HS256 signing key (min 32 chars) |
| `HMAC_SECRET` | API key hashing secret (min 32 chars) |
| `ENCRYPTION_KEY` | AES-256-GCM key for provider keys (64 hex chars) |
| `INTERNAL_TOKEN` | Proxy ↔ Processing shared secret (min 32 chars) |

### Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `PROCESSING_PORT` | `8081` | Processing service port |
| `PROXY_PORT` | `8080` | Proxy service port |
| `JWT_TTL_DAYS` | `30` | JWT token expiration |
| `PROVIDER_TIMEOUT_SECONDS` | `120` | Upstream LLM timeout |
| `ALLOWED_ORIGINS` | — | CORS origins (comma-separated) |
| `PROCESSING_LLM_BASE_URL` | — | LLM endpoint for narratives/clustering |
| `PROCESSING_LLM_API_KEY` | — | LLM API key for narratives/clustering |
| `PROCESSING_LLM_MODEL` | — | LLM model for narratives/clustering |
| `SMTP_HOST`, `SMTP_PORT`, etc. | — | Email delivery (optional) |

Without `PROCESSING_LLM_*` configured, narratives are concatenated inputs and clustering is deterministic.

## Project Structure

```
agentspan/
├── proxy/          # Stateless proxy service (Go, chi)
├── processing/     # Stateful API + workers (Go, chi, sqlc, golang-migrate)
├── web/            # SPA dashboard (Vite, React 19, TypeScript, Tailwind v4)
├── docs/           # Architecture and design documents
├── scripts/        # Smoke tests, load tests, security audits
└── docker-compose.yml
```

## Development

### Prerequisites

- Go 1.26+
- Node.js 22+
- PostgreSQL 17

### Run Locally

```bash
# Start PostgreSQL (or use docker compose up postgres)

# Processing service
cd processing
DATABASE_URL="postgres://..." go run ./cmd/processing

# Proxy service
cd proxy
PROCESSING_URL="http://localhost:8081" go run ./cmd/proxy

# Frontend dev server
cd web
npm install
npm run dev
```

### Scripts

| Script | Description |
|--------|-------------|
| `scripts/smoke-test.sh` | End-to-end test: register → login → create org → ingest span → verify |
| `scripts/test-isolation.sh` | Verify organization isolation (22 checks) |
| `scripts/test-load.sh` | Load testing with goroutine bounds checking |
| `scripts/audit-logs.sh` | Security audit for leaked secrets in logs |
| `scripts/reset.sh` | Destroy all data and rebuild from scratch |

### Tech Stack

| Component | Stack |
|-----------|-------|
| Proxy | Go, chi |
| Processing | Go, chi, sqlc, golang-migrate, pgx/v5 |
| Frontend | Vite, React 19, TypeScript, Tailwind v4, TanStack Query, Zustand, Recharts, shadcn/ui |
| Database | PostgreSQL 17 |
| Auth | JWT (HS256) + API keys (HMAC-SHA256) |

## Security

- API keys stored as HMAC-SHA256 digests (never plaintext)
- Provider API keys encrypted at rest with AES-256-GCM
- All resources scoped to organization — no cross-org access
- Internal API secured by shared secret + IP firewall
- API keys, passwords, full LLM I/O, and JWT secrets are never logged

## Self-Hosting

See [docs/SELF-HOST.md](docs/SELF-HOST.md) for the full self-hosting guide, including:

- System requirements (1 CPU, 1GB RAM minimum)
- Update procedures
- Debug tools (pprof, health checks)
- Backup and restore

## License

MIT
