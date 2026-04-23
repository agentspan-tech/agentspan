# Troubleshooting

## Container Won't Start

**Check logs:**
```bash
docker compose logs postgres
docker compose logs processing
docker compose logs proxy
```

**Common causes:**
- `.env` file missing: run `cp .env.example .env` and fill in values
- Port conflict: check if ports 5432, 8080, or 8081 are already in use
- Database not ready: processing waits for postgres health check; check postgres logs

## Proxy Returns 502

The proxy cannot reach the processing service.

- Verify `PROCESSING_URL` in `.env` points to the processing service (default: `http://processing:8081` for Docker)
- Check processing is healthy: `docker compose ps` should show "healthy"
- Check processing logs: `docker compose logs processing`

## WebSocket Disconnects

- Set `ALLOWED_ORIGINS` in `.env` to include your frontend URL
- If behind a reverse proxy, ensure it supports WebSocket upgrade (see deployment guide)
- Check browser console for CORS errors
- The dashboard reconnects automatically (up to 20 attempts with exponential backoff)

## Migration Fails

- Verify `DATABASE_URL` is correct and postgres is accepting connections
- Check postgres logs: `docker compose logs postgres`
- If a migration fails mid-way, processing will retry on next restart
- To check migration state: `docker compose exec postgres psql -U agentorbit -c "SELECT * FROM schema_migrations"`

## CORS Errors

Browser shows "blocked by CORS policy":

- Set `ALLOWED_ORIGINS` in `.env` to your frontend URL(s)
- Multiple origins: comma-separated list (e.g., `http://localhost:5173,http://localhost:8081`)
- Restart processing after changing: `docker compose restart processing`

## Email Not Sending

SMTP is optional. Without SMTP configuration:
- Verification links and password reset tokens are logged to stdout
- Check processing logs: `docker compose logs processing | grep "verification\|reset"`

With SMTP configured:
- Verify `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` in `.env`
- Check processing logs for SMTP errors
- Ensure your SMTP provider allows the sender address

## Agent Can't Connect

- Verify the proxy is running: `curl http://localhost:8080/health`
- Check the agent's `base_url` points to the proxy (port 8080, not 8081)
- Verify the API key is active (not deactivated) in the dashboard
- Check the `Authorization: Bearer ao-<key>` header is set correctly

## High Memory Usage

- Check Docker resource limits in `docker-compose.yml`
- Processing: reduce concurrent intelligence pipeline workers (hardcoded at 5)
- Proxy: check `spans_dropped` in health endpoint -- if high, increase `SPAN_WORKERS`
- PostgreSQL: check for long-running queries with `pg_stat_activity`

## Resetting the Database

To start fresh (destroys all data):

```bash
docker compose down -v
docker compose up -d
```

The `-v` flag removes the PostgreSQL volume. Migrations will re-run on startup.
