#!/usr/bin/env bash
# smoke-test.sh — AgentSpan E2E smoke test
#
# Exercises the full user journey: register, verify, login, create org,
# create API key, send span through proxy, check session, frontend, pprof.
#
# Prerequisites:
#   - docker compose up (all services healthy)
#   - curl and jq installed
#
# Usage:
#   bash scripts/smoke-test.sh
#
# Configurable via env vars:
#   PROCESSING_URL  (default: http://localhost:8081)
#   PROXY_URL       (default: http://localhost:8080)
#   INTERNAL_TOKEN  (default: read from .env file)

set -euo pipefail

PROCESSING_URL=${PROCESSING_URL:-http://localhost:8081}
PROXY_URL=${PROXY_URL:-http://localhost:8080}

# ---------------------------------------------------------------------------
# Helper functions
# ---------------------------------------------------------------------------

pass() { echo -e "\033[32m[PASS]\033[0m $1"; }
fail() { echo -e "\033[31m[FAIL]\033[0m $1"; exit 1; }
step() { echo -e "\n\033[36m[STEP]\033[0m $1"; }

# Read INTERNAL_TOKEN from .env if not set in environment
if [ -z "${INTERNAL_TOKEN:-}" ]; then
  ENV_FILE="$(dirname "$(dirname "$(realpath "$0")")")/.env"
  if [ -f "$ENV_FILE" ]; then
    INTERNAL_TOKEN=$(grep -E '^INTERNAL_TOKEN=' "$ENV_FILE" | cut -d'=' -f2- | tr -d '"' | tr -d "'")
  fi
  if [ -z "${INTERNAL_TOKEN:-}" ]; then
    INTERNAL_TOKEN="changeme_internal_token_min_32_chars"
  fi
fi

SMOKE_EMAIL="smoke@test.local"
SMOKE_PASSWORD="smoketest123"
SMOKE_NAME="Smoke Test"
SMOKE_ORG="Smoke Org"

echo "============================================================"
echo "AgentSpan E2E Smoke Test"
echo "  Processing: $PROCESSING_URL"
echo "  Proxy:      $PROXY_URL"
echo "============================================================"

# ---------------------------------------------------------------------------
# Step 1: Health checks
# ---------------------------------------------------------------------------
step "1/10 Health checks"

PROC_HEALTH=$(curl -sf "$PROCESSING_URL/health") || fail "Processing /health request failed"
echo "  processing: $PROC_HEALTH"
echo "$PROC_HEALTH" | jq -e '.status == "ok"' > /dev/null || fail "Processing health returned unexpected response: $PROC_HEALTH"
pass "Processing service healthy"

PROXY_HEALTH=$(curl -sf "$PROXY_URL/health") || fail "Proxy /health request failed"
echo "  proxy:      $PROXY_HEALTH"
echo "$PROXY_HEALTH" | jq -e '.status == "ok"' > /dev/null || fail "Proxy health returned unexpected response: $PROXY_HEALTH"
pass "Proxy service healthy"

# ---------------------------------------------------------------------------
# Step 2: Register user (idempotent — skip gracefully if already exists)
# ---------------------------------------------------------------------------
step "2/10 Register user ($SMOKE_EMAIL)"

REG_HTTP_CODE=$(curl -s -o /tmp/smoke_reg.json -w "%{http_code}" \
  -X POST "$PROCESSING_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$SMOKE_EMAIL\",\"password\":\"$SMOKE_PASSWORD\",\"name\":\"$SMOKE_NAME\"}")

REG_BODY=$(cat /tmp/smoke_reg.json)

if [ "$REG_HTTP_CODE" = "201" ]; then
  AUTO_LOGIN=$(echo "$REG_BODY" | jq -r '.auto_login // empty')
  if [ "$AUTO_LOGIN" = "true" ]; then
    pass "User registered (201) — auto-verified (self-host first user)"
    NEEDS_VERIFY=false
  else
    pass "User registered (201)"
    NEEDS_VERIFY=true
  fi
elif [ "$REG_HTTP_CODE" = "409" ]; then
  pass "User already exists (409) — skipping registration"
  NEEDS_VERIFY=false
elif [ "$REG_HTTP_CODE" = "403" ]; then
  pass "Registration closed (403) — self-host, user already exists"
  NEEDS_VERIFY=false
else
  fail "Register returned unexpected HTTP $REG_HTTP_CODE: $REG_BODY"
fi

# ---------------------------------------------------------------------------
# Step 3: Verify email
# ---------------------------------------------------------------------------
step "3/10 Verify email"

if [ "$NEEDS_VERIFY" = "true" ]; then
  VERIFY_URL=$(echo "$REG_BODY" | jq -r '.verification_url // empty')
  if [ -z "$VERIFY_URL" ]; then
    # SMTP mode — email was sent, cannot verify headlessly
    fail "Register returned no verification_url (SMTP mode?). Disable SMTP to use smoke test."
  fi

  # Extract token from URL query param ?token=...
  VERIFY_TOKEN=$(echo "$VERIFY_URL" | grep -oE '[?&]token=[^&]+' | cut -d'=' -f2)
  if [ -z "$VERIFY_TOKEN" ]; then
    fail "Could not extract token from verification_url: $VERIFY_URL"
  fi

  VERIFY_RESP=$(curl -sf -X POST "$PROCESSING_URL/auth/verify-email" \
    -H "Content-Type: application/json" \
    -d "{\"token\":\"$VERIFY_TOKEN\"}") || fail "Verify email request failed"

  echo "$VERIFY_RESP" | jq -e '.verified == true' > /dev/null || fail "Verify email returned unexpected response: $VERIFY_RESP"
  pass "Email verified"
else
  pass "Skipping email verification (user already verified)"
fi

# ---------------------------------------------------------------------------
# Step 4: Login
# ---------------------------------------------------------------------------
step "4/10 Login"

LOGIN_RESP=$(curl -sf -X POST "$PROCESSING_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$SMOKE_EMAIL\",\"password\":\"$SMOKE_PASSWORD\"}") || fail "Login request failed"

JWT=$(echo "$LOGIN_RESP" | jq -r '.token // empty')
[ -n "$JWT" ] || fail "Login response missing token: $LOGIN_RESP"
pass "Login successful, JWT acquired"

# ---------------------------------------------------------------------------
# Step 5: Create or find organization
# ---------------------------------------------------------------------------
step "5/10 Create/find organization"

# Check if an org already exists
ORGS_RESP=$(curl -sf "$PROCESSING_URL/api/orgs/" \
  -H "Authorization: Bearer $JWT") || fail "List orgs request failed"

ORG_ID=$(echo "$ORGS_RESP" | jq -r '
  if type == "array" then .[0].id
  elif .items then .items[0].id
  else .id
  end // empty' 2>/dev/null || true)

if [ -n "$ORG_ID" ]; then
  pass "Org already exists (id=$ORG_ID)"
else
  CREATE_ORG_RESP=$(curl -sf -X POST "$PROCESSING_URL/api/orgs/" \
    -H "Authorization: Bearer $JWT" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$SMOKE_ORG\",\"locale\":\"en\"}") || fail "Create org request failed"

  ORG_ID=$(echo "$CREATE_ORG_RESP" | jq -r '.id // empty')
  [ -n "$ORG_ID" ] || fail "Create org response missing id: $CREATE_ORG_RESP"
  pass "Organization created (id=$ORG_ID)"
fi

# ---------------------------------------------------------------------------
# Step 6: Create API key
# ---------------------------------------------------------------------------
step "6/10 Create API key"

KEY_RESP_CODE=$(curl -s -o /tmp/smoke_key.json -w "%{http_code}" \
  -X POST "$PROCESSING_URL/api/orgs/$ORG_ID/keys" \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"smoke-agent","provider_type":"openai","provider_key":"sk-fake-for-smoke-test"}')

KEY_RESP_BODY=$(cat /tmp/smoke_key.json)

if [ "$KEY_RESP_CODE" = "201" ]; then
  RAW_KEY=$(echo "$KEY_RESP_BODY" | jq -r '.raw_key // empty')
  [ -n "$RAW_KEY" ] || fail "Create key response missing raw_key: $KEY_RESP_BODY"
  pass "API key created (raw_key prefix: ${RAW_KEY:0:10}...)"
elif [ "$KEY_RESP_CODE" = "200" ]; then
  RAW_KEY=$(echo "$KEY_RESP_BODY" | jq -r '.raw_key // empty')
  [ -n "$RAW_KEY" ] || fail "Create key response missing raw_key: $KEY_RESP_BODY"
  pass "API key returned (raw_key prefix: ${RAW_KEY:0:10}...)"
else
  # Key may already exist (raw_key only shown at creation) — list and use existing for proxy test
  KEYS_RESP=$(curl -sf "$PROCESSING_URL/api/orgs/$ORG_ID/keys" \
    -H "Authorization: Bearer $JWT") || fail "List keys request failed"
  KEY_COUNT=$(echo "$KEYS_RESP" | jq 'if type=="array" then length elif .items then .items|length else 0 end')
  if [ "$KEY_COUNT" -gt 0 ]; then
    # raw_key not available after creation — create a fresh key under a different name
    NEW_KEY_RESP=$(curl -sf -X POST "$PROCESSING_URL/api/orgs/$ORG_ID/keys" \
      -H "Authorization: Bearer $JWT" \
      -H "Content-Type: application/json" \
      -d '{"name":"smoke-agent-retry","provider_type":"openai","provider_key":"sk-fake-for-smoke-test"}') || fail "Create retry key request failed"
    RAW_KEY=$(echo "$NEW_KEY_RESP" | jq -r '.raw_key // empty')
    [ -n "$RAW_KEY" ] || fail "Retry key response missing raw_key: $NEW_KEY_RESP"
    pass "Fresh API key created for proxy test (raw_key prefix: ${RAW_KEY:0:10}...)"
  else
    fail "Create API key returned HTTP $KEY_RESP_CODE: $KEY_RESP_BODY"
  fi
fi

# ---------------------------------------------------------------------------
# Step 7: Send span through proxy
# ---------------------------------------------------------------------------
step "7/10 Send span through proxy"

PROXY_HTTP_CODE=$(curl -s -o /tmp/smoke_proxy.json -w "%{http_code}" \
  --max-time 15 \
  -X POST "$PROXY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $RAW_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}')

PROXY_RESP_BODY=$(cat /tmp/smoke_proxy.json)
echo "  proxy response HTTP $PROXY_HTTP_CODE"

# Proxy must NOT return 5xx — provider 4xx (e.g., 401 invalid key) is acceptable
if [ "$PROXY_HTTP_CODE" -ge 500 ]; then
  fail "Proxy returned 5xx ($PROXY_HTTP_CODE) — unexpected server error: $PROXY_RESP_BODY"
fi
pass "Proxy forwarded request (HTTP $PROXY_HTTP_CODE — provider error expected with fake key)"

# ---------------------------------------------------------------------------
# Step 8: Wait and check session was created
# ---------------------------------------------------------------------------
step "8/10 Check session created"

echo "  waiting 3s for span ingestion..."
sleep 3

SESSIONS_RESP=$(curl -sf "$PROCESSING_URL/api/orgs/$ORG_ID/sessions" \
  -H "Authorization: Bearer $JWT") || fail "List sessions request failed"

SESSION_COUNT=$(echo "$SESSIONS_RESP" | jq '
  if type == "array" then length
  elif .items then .items | length
  elif .data then .data | length
  elif .sessions then .sessions | length
  else 0
  end' 2>/dev/null || echo "0")

[ "$SESSION_COUNT" -gt 0 ] || fail "No sessions found after proxy span (count=$SESSION_COUNT). Response: $SESSIONS_RESP"
pass "Session created (count=$SESSION_COUNT)"

# ---------------------------------------------------------------------------
# Step 9: Frontend check
# ---------------------------------------------------------------------------
step "9/10 Frontend serves HTML with div#root"

FRONTEND_BODY=$(curl -sf "$PROCESSING_URL/") || fail "Frontend request failed"
echo "$FRONTEND_BODY" | grep -q '<div id="root">' || fail "Frontend HTML missing <div id=\"root\">"
pass "Frontend serves HTML with div#root"

# ---------------------------------------------------------------------------
# Step 10: pprof check
# ---------------------------------------------------------------------------
step "10/10 pprof accessible on internal API"

PPROF_OUTPUT=$(curl -sf \
  -H "X-Internal-Token: $INTERNAL_TOKEN" \
  "$PROCESSING_URL/internal/debug/pprof/goroutine?debug=1" | head -5) || fail "pprof request failed (check INTERNAL_TOKEN)"

echo "  pprof output: $(echo "$PPROF_OUTPUT" | head -1)"
[ -n "$PPROF_OUTPUT" ] || fail "pprof returned empty response"
pass "pprof goroutine profile accessible via internal token"

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
echo ""
echo "============================================================"
echo -e "\033[32mAll 10 smoke test steps passed.\033[0m"
echo "AgentSpan is functioning correctly from a clean boot."
echo "============================================================"
