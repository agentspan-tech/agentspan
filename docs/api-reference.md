# API Reference

AgentSpan exposes three API surfaces: the public Dashboard API, the Proxy endpoints, and the Internal API (proxy-to-processing).

## Authentication

Two authentication methods:

- **JWT cookie** (browser sessions): Set automatically on login. HttpOnly, SameSite=Lax.
- **API key** (agents): `Authorization: Bearer as-<key>` header. Created via the dashboard.

All authenticated endpoints require one of these methods.

## Public Endpoints (no auth)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/register` | Create a new user account |
| POST | `/api/auth/login` | Log in, returns JWT cookie |
| POST | `/api/auth/logout` | Clear JWT cookie |
| POST | `/api/auth/request-reset` | Request password reset email |
| POST | `/api/auth/reset-password` | Confirm password reset with token |
| POST | `/api/auth/verify-email` | Verify email address with token |
| GET | `/health` | Health check (returns `{"status":"ok"}`) |

## Authenticated Endpoints

### Organizations

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/orgs` | Create organization |
| GET | `/api/orgs` | List user's organizations |
| GET | `/api/orgs/{orgID}` | Get organization details |
| PUT | `/api/orgs/{orgID}` | Update organization |
| DELETE | `/api/orgs/{orgID}` | Schedule organization deletion (14-day grace) |
| GET | `/api/orgs/{orgID}/members` | List organization members |
| DELETE | `/api/orgs/{orgID}/members/{userID}` | Remove member |

### Invites

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/orgs/{orgID}/invites` | Send invite |
| GET | `/api/orgs/{orgID}/invites` | List pending invites |
| DELETE | `/api/orgs/{orgID}/invites/{inviteID}` | Revoke invite |
| POST | `/api/invites/accept` | Accept invite (via token) |

### API Keys

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/orgs/{orgID}/keys` | Create API key |
| GET | `/api/orgs/{orgID}/keys` | List API keys |
| DELETE | `/api/orgs/{orgID}/keys/{keyID}` | Deactivate API key |

### Dashboard

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orgs/{orgID}/sessions` | List sessions (filterable) |
| GET | `/api/orgs/{orgID}/sessions/{sessionID}` | Get session with spans |
| GET | `/api/orgs/{orgID}/stats` | KPI statistics |
| GET | `/api/orgs/{orgID}/daily-stats` | Daily activity chart data |
| GET | `/api/orgs/{orgID}/agent-stats` | Per-agent statistics |

### Alert Rules

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/orgs/{orgID}/alerts` | Create alert rule |
| GET | `/api/orgs/{orgID}/alerts` | List alert rules |
| PUT | `/api/orgs/{orgID}/alerts/{alertID}` | Update alert rule |
| DELETE | `/api/orgs/{orgID}/alerts/{alertID}` | Delete alert rule |

### WebSocket

Connect to `ws(s)://host/cable` with a valid JWT cookie. Subscriptions are managed via JSON messages after connection.

**Subscribe to events:**
```json
{"type": "subscribe", "topic": "sessions", "org_id": "uuid"}
{"type": "subscribe", "topic": "spans", "session_id": "uuid"}
{"type": "subscribe", "topic": "stats", "org_id": "uuid"}
{"type": "subscribe", "topic": "alerts", "org_id": "uuid"}
```

## Proxy Endpoints

These are the endpoints your agents call. Point your agent's `base_url` at the proxy.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/chat/completions` | OpenAI-compatible chat completion |
| POST | `/v1/messages` | Anthropic-compatible messages |

**Required header:** `Authorization: Bearer as-<your-api-key>`

**Optional headers:**
- `X-AgentSpan-Session: <id>` -- explicit session grouping
- `X-AgentSpan-Agent: <name>` -- override agent name for this request

Requests and responses are forwarded unchanged. SSE streams are passed through without buffering.

## Internal API

Used by the proxy to communicate with processing. Secured by `X-Internal-Token` header.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/internal/auth/verify` | Validate an API key |
| POST | `/internal/spans/ingest` | Submit span data |
