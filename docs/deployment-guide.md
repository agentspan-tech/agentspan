# Deployment Guide

## Quick Start

### Prerequisites

- Docker 24+ and Docker Compose v2
- At least 2 GB RAM and 2 CPU cores available

### Steps

```bash
git clone https://github.com/agentorbit-tech/agentorbit.git
cd agentorbit
cp .env.example .env
```

Edit `.env` and replace all `changeme_` values with secure random strings:

```bash
# Generate secrets (Linux/macOS)
openssl rand -hex 16   # for JWT_SECRET, HMAC_SECRET, INTERNAL_TOKEN (min 32 chars)
openssl rand -hex 32   # for ENCRYPTION_KEY (exactly 64 hex chars)
```

Start the stack:

```bash
docker compose up -d
```

Dashboard: http://localhost:8081

## Configuration

All configuration is via environment variables in `.env`.

### Required Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `POSTGRES_PASSWORD` | Database password | Random string |
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@postgres:5432/agentorbit?sslmode=disable` |
| `JWT_SECRET` | JWT signing key (min 32 chars) | `openssl rand -hex 16` |
| `HMAC_SECRET` | API key digest secret (min 32 chars) | `openssl rand -hex 16` |
| `ENCRYPTION_KEY` | AES-256 key for provider keys (64 hex chars) | `openssl rand -hex 32` |
| `INTERNAL_TOKEN` | Proxy-to-processing auth (min 32 chars) | `openssl rand -hex 16` |

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PROCESSING_PORT` | `8081` | Processing service port |
| `PROXY_PORT` | `8080` | Proxy service port |
| `DEPLOYMENT_MODE` | `self_host` | Deployment mode (`cloud`, `self_host`) |
| `JWT_TTL_DAYS` | `30` | JWT token lifetime in days |
| `ALLOWED_ORIGINS` | (empty) | Comma-separated frontend URLs for CORS |
| `PROVIDER_TIMEOUT_SECONDS` | `120` | Timeout for LLM provider requests |
| `SPAN_WORKERS` | `3` | Number of async span dispatch workers |
| `SMTP_HOST` | (none) | SMTP server for email delivery |
| `SMTP_PORT` | (none) | SMTP port (typically 587) |
| `SMTP_USER` | (none) | SMTP username |
| `SMTP_PASS` | (none) | SMTP password |
| `SMTP_FROM` | (none) | Sender address for emails |
| `DATA_RETENTION_DAYS` | `0` (keep forever) | Auto-delete spans/sessions older than N days |
| `LOG_LEVEL` | `INFO` | Log verbosity (`DEBUG`, `INFO`, `WARN`, `ERROR`) |
| `PROCESSING_LLM_BASE_URL` | (none) | LLM API URL for narratives/clustering |
| `PROCESSING_LLM_API_KEY` | (none) | LLM API key |
| `PROCESSING_LLM_MODEL` | (none) | LLM model name |

### CORS Configuration

Set `ALLOWED_ORIGINS` to the URL(s) where your frontend is served:

```bash
# Single origin
ALLOWED_ORIGINS=https://agentorbit.example.com

# Multiple origins
ALLOWED_ORIGINS=https://agentorbit.example.com,https://app.example.com
```

If empty, CORS headers are not sent. Browsers will block cross-origin requests to the API.

## Security Checklist

Before exposing to the internet:

- [ ] Replace all `changeme_` values in `.env`
- [ ] Set `ALLOWED_ORIGINS` to your frontend URL(s)
- [ ] Place behind a reverse proxy (nginx, Caddy) with HTTPS
- [ ] Ensure `.env` is not accessible from the web
- [ ] Keep `POSTGRES_PORT` bound to `127.0.0.1` (default)
- [ ] Review Docker resource limits in `docker-compose.yml`

## Upgrading

```bash
git pull
docker compose build
docker compose up -d
```

Database migrations run automatically when the processing service starts. No manual migration step is needed.

## Backup and Restore

### Creating a Backup

Use `pg_dump` from the PostgreSQL container (or any host with `pg_dump` installed):

```bash
# From the Docker host — dumps the database inside the postgres container
docker compose exec postgres pg_dump \
  -U ${POSTGRES_USER:-agentorbit} \
  -d ${POSTGRES_DB:-agentorbit} \
  --format=custom \
  --compress=zstd \
  > agentorbit_$(date +%Y%m%d_%H%M%S).dump
```

The `--format=custom` flag produces a compressed archive that supports selective restore and parallel jobs. The `--compress=zstd` flag uses zstd compression (PostgreSQL 16+; omit for older versions or use `--compress=9` for gzip).

For automated daily backups, add a cron entry on the Docker host:

```cron
0 3 * * * docker compose -f /path/to/docker-compose.yml exec -T postgres pg_dump -U agentorbit -d agentorbit --format=custom --compress=zstd > /backups/agentorbit_$(date +\%Y\%m\%d).dump 2>> /var/log/agentorbit-backup.log
```

### Restoring a Backup

To restore onto a fresh instance (new `docker compose up` with empty database):

```bash
# 1. Start only postgres (processing would auto-migrate and create tables)
docker compose up -d postgres

# 2. Wait for postgres to be healthy
docker compose exec postgres pg_isready -U agentorbit

# 3. Restore the dump (--clean drops existing objects first)
docker compose exec -T postgres pg_restore \
  -U ${POSTGRES_USER:-agentorbit} \
  -d ${POSTGRES_DB:-agentorbit} \
  --clean --if-exists \
  --no-owner \
  --no-privileges \
  < agentorbit_20260330.dump

# 4. Start the full stack — processing will run any newer migrations on top
docker compose up -d
```

Key flags:
- `--clean --if-exists`: drops existing objects before restoring, ignores missing ones
- `--no-owner --no-privileges`: avoids errors when the target user differs from the original

### Verifying a Restore

After restoring, verify data integrity:

```bash
docker compose exec postgres psql -U agentorbit -d agentorbit -c "
  SELECT 'users' AS table_name, COUNT(*) FROM users
  UNION ALL SELECT 'organizations', COUNT(*) FROM organizations
  UNION ALL SELECT 'sessions', COUNT(*) FROM sessions
  UNION ALL SELECT 'spans', COUNT(*) FROM spans
  UNION ALL SELECT 'schema_migrations', MAX(version)::text FROM schema_migrations;
"
```

Check that the processing service starts without migration errors:

```bash
docker compose logs processing | grep -E 'migrations complete|migrate'
```

### Restore + Migration on Fresh Instance

When restoring a backup from an older version onto a newer AgentOrbit release:

1. The dump contains the schema at the time of backup (e.g., migration 17)
2. `pg_restore --clean` recreates that exact schema
3. Processing starts and `golang-migrate` detects the current version from the `schema_migrations` table
4. Any newer migrations (e.g., 18, 19, 20) are applied automatically

This is safe because all migrations are idempotent and use `IF NOT EXISTS` / `IF EXISTS` guards.

## Data Retention

Set `DATA_RETENTION_DAYS` to automatically purge old data. When set to a positive integer, a daily cron inside the processing service deletes:

1. **Spans** older than N days (by `created_at`)
2. **Sessions** that are closed and older than N days, with no remaining spans
3. **Alert events** older than N days

Deletion runs in batches of 1000 rows to avoid long locks. Organizations, users, API keys, and alert rules are never affected.

| Use Case | Recommended Setting |
|----------|---------------------|
| Development / testing | `DATA_RETENTION_DAYS=7` |
| Production (cost-conscious) | `DATA_RETENTION_DAYS=90` |
| Production (compliance) | `DATA_RETENTION_DAYS=365` |
| Keep everything | Omit or set to `0` |

Combined with backups, a typical strategy is: daily `pg_dump` retained for 30 days + `DATA_RETENTION_DAYS=90` for live data.

## Resource Requirements

| Service | Memory | CPU | Purpose |
|---------|--------|-----|---------|
| PostgreSQL | 512 MB | 1.0 | Database |
| Processing | 512 MB | 1.0 | API, WebSocket, workers |
| Proxy | 256 MB | 0.5 | Request forwarding |
| **Total** | **~1.3 GB** | **2.5** | Minimum for the full stack |

For production workloads, increase limits based on your agent traffic volume.

## Reverse Proxy Setup

### nginx

```nginx
server {
    listen 443 ssl;
    server_name agentorbit.example.com;

    location / {
        proxy_pass http://localhost:8081;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /cable {
        proxy_pass http://localhost:8081;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### Agent Proxy

For agents, point at the proxy service (default port 8080):

```nginx
server {
    listen 443 ssl;
    server_name proxy.agentorbit.example.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_http_version 1.1;
        proxy_buffering off;  # Required for SSE streaming
    }
}
```
