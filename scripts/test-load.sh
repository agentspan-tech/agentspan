#!/usr/bin/env bash
# test-load.sh — AgentSpan load stability and goroutine bounds test
#
# Verifies that goroutine count stays bounded under sustained traffic (D-10, D-11).
# Runs 10 concurrent agents for 60 seconds, samples goroutines via pprof before
# and after, and asserts no goroutine leak occurs.
#
# Prerequisites:
#   - docker compose up (all services healthy)
#   - Plan 01 complete (pprof endpoint at /internal/debug/pprof/*)
#   - curl and jq installed
#
# Usage:
#   bash scripts/test-load.sh
#
# Configurable via env vars:
#   PROCESSING_URL  (default: http://localhost:8081)
#   PROXY_URL       (default: http://localhost:8080)
#   INTERNAL_TOKEN  (default: read from .env file)

set -euo pipefail

PROCESSING_URL=${PROCESSING_URL:-http://localhost:8081}
PROXY_URL=${PROXY_URL:-http://localhost:8080}

# Load test parameters (D-11)
DURATION=60       # seconds
CONCURRENT=10     # concurrent agents
INTERVAL=6        # seconds between requests per agent

# Goroutine bounds (D-10)
MAX_GOROUTINES=500
MAX_GROWTH=50

# ---------------------------------------------------------------------------
# Helper functions
# ---------------------------------------------------------------------------

pass() { echo -e "\033[32m[PASS]\033[0m $1"; }
fail() { echo -e "\033[31m[FAIL]\033[0m $1"; OVERALL_PASS=false; }
step() { echo -e "\n\033[36m[STEP]\033[0m $1"; }

OVERALL_PASS=true

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

echo "============================================================"
echo "AgentSpan Load Stability Test (D-10, D-11)"
echo "  Processing: $PROCESSING_URL"
echo "  Proxy:      $PROXY_URL"
echo "  Duration:   ${DURATION}s"
echo "  Concurrent: $CONCURRENT agents"
echo "  Max goroutines: $MAX_GOROUTINES"
echo "============================================================"

# ---------------------------------------------------------------------------
# Pre-check: verify pprof endpoint is accessible
# ---------------------------------------------------------------------------
step "Pre-check: pprof endpoint accessibility"

PPROF_CHECK=$(curl -s -o /dev/null -w '%{http_code}' \
    -H "X-Internal-Token: $INTERNAL_TOKEN" \
    "$PROCESSING_URL/internal/debug/pprof/goroutine?debug=1")

if [[ "$PPROF_CHECK" != "200" ]]; then
    echo "[FAIL] pprof endpoint returned HTTP $PPROF_CHECK."
    echo "  Check: pprof endpoint not available. Run Plan 01 first or check INTERNAL_TOKEN."
    echo "  URL: $PROCESSING_URL/internal/debug/pprof/goroutine"
    exit 1
fi
pass "pprof endpoint accessible (HTTP 200)"

# ---------------------------------------------------------------------------
# Setup: register test user, create org and API key
# ---------------------------------------------------------------------------
step "Setup: preparing load test user"

LOAD_EMAIL="load@test.local"
LOAD_PASSWORD="loadtest123"
LOAD_NAME="Load Test User"
LOAD_ORG="Load Test Org"

# Register (handle 409 idempotently)
REG_HTTP_CODE=$(curl -s -o /tmp/load_reg.json -w "%{http_code}" \
    -X POST "$PROCESSING_URL/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$LOAD_EMAIL\",\"password\":\"$LOAD_PASSWORD\",\"name\":\"$LOAD_NAME\"}")
REG_BODY=$(cat /tmp/load_reg.json)

if [ "$REG_HTTP_CODE" = "201" ]; then
    AUTO_LOGIN=$(echo "$REG_BODY" | jq -r '.auto_login // empty')
    if [ "$AUTO_LOGIN" = "true" ]; then
        echo "  User registered and auto-verified (self-host first user)"
    else
        VERIFY_URL=$(echo "$REG_BODY" | jq -r '.verification_url // empty')
        if [ -z "$VERIFY_URL" ]; then
            echo "[FAIL] Register returned no verification_url (SMTP mode?). Disable SMTP."
            exit 1
        fi
        VERIFY_TOKEN=$(echo "$VERIFY_URL" | grep -oE '[?&]token=[^&]+' | cut -d'=' -f2)
        curl -sf -X POST "$PROCESSING_URL/auth/verify-email" \
            -H "Content-Type: application/json" \
            -d "{\"token\":\"$VERIFY_TOKEN\"}" > /dev/null || { echo "[FAIL] Email verification failed"; exit 1; }
        echo "  User registered and verified"
    fi
elif [ "$REG_HTTP_CODE" = "409" ] || [ "$REG_HTTP_CODE" = "403" ]; then
    echo "  User already exists (idempotent)"
else
    echo "[FAIL] Register returned unexpected HTTP $REG_HTTP_CODE: $REG_BODY"
    exit 1
fi

# Login
LOGIN_RESP=$(curl -sf -X POST "$PROCESSING_URL/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$LOAD_EMAIL\",\"password\":\"$LOAD_PASSWORD\"}") || { echo "[FAIL] Login failed"; exit 1; }
JWT=$(echo "$LOGIN_RESP" | jq -r '.token // empty')
[ -n "$JWT" ] || { echo "[FAIL] Login response missing token"; exit 1; }
echo "  JWT acquired"

# Get or create org
ORGS_RESP=$(curl -sf "$PROCESSING_URL/api/orgs/" \
    -H "Authorization: Bearer $JWT") || { echo "[FAIL] List orgs failed"; exit 1; }
ORG_ID=$(echo "$ORGS_RESP" | jq -r '
    if type == "array" then .[0].id
    elif .items then .items[0].id
    else .id
    end // empty' 2>/dev/null || true)

if [ -z "$ORG_ID" ]; then
    CREATE_ORG_RESP=$(curl -sf -X POST "$PROCESSING_URL/api/orgs/" \
        -H "Authorization: Bearer $JWT" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"$LOAD_ORG\",\"locale\":\"en\"}") || { echo "[FAIL] Create org failed"; exit 1; }
    ORG_ID=$(echo "$CREATE_ORG_RESP" | jq -r '.id // empty')
    [ -n "$ORG_ID" ] || { echo "[FAIL] Create org response missing id"; exit 1; }
    echo "  Org created (id=$ORG_ID)"
else
    echo "  Org found (id=$ORG_ID)"
fi

# Create API key (always create fresh to get raw_key)
KEY_RESP=$(curl -sf -X POST "$PROCESSING_URL/api/orgs/$ORG_ID/keys" \
    -H "Authorization: Bearer $JWT" \
    -H "Content-Type: application/json" \
    -d '{"name":"load-agent","provider_type":"openai","provider_key":"sk-fake-load-test"}') || { echo "[FAIL] Create key failed"; exit 1; }
RAW_KEY=$(echo "$KEY_RESP" | jq -r '.raw_key // empty')
[ -n "$RAW_KEY" ] || { echo "[FAIL] Create key response missing raw_key: $KEY_RESP"; exit 1; }
echo "  API key created (prefix: ${RAW_KEY:0:10}...)"

pass "Setup complete (org=$ORG_ID)"

# ---------------------------------------------------------------------------
# Baseline goroutine count (before load)
# ---------------------------------------------------------------------------
step "Baseline goroutine count (pre-load)"

BASELINE=$(curl -sf \
    -H "X-Internal-Token: $INTERNAL_TOKEN" \
    "$PROCESSING_URL/internal/debug/pprof/goroutine?debug=1" | head -1 | grep -oE '[0-9]+' | head -1)

if [ -z "$BASELINE" ]; then
    echo "[FAIL] Could not read baseline goroutine count from pprof"
    exit 1
fi
step "Baseline goroutine count: $BASELINE"
pass "Baseline captured: $BASELINE goroutines"

# ---------------------------------------------------------------------------
# Load phase: 10 concurrent agents sending requests for 60 seconds (D-11)
# ---------------------------------------------------------------------------
step "Load phase: ${CONCURRENT} concurrent agents for ${DURATION}s"
echo "  Each agent sends 1 request every ${INTERVAL}s (~$((CONCURRENT * 60 / INTERVAL)) total requests)"
echo "  Starting load at $(date +%H:%M:%S)..."

START_EPOCH=$SECONDS

for i in $(seq 1 $CONCURRENT); do
    (
        END=$((SECONDS + DURATION))
        while [[ $SECONDS -lt $END ]]; do
            curl -s -o /dev/null -w '' \
                --max-time 15 \
                -X POST "$PROXY_URL/v1/chat/completions" \
                -H "Authorization: Bearer $RAW_KEY" \
                -H "Content-Type: application/json" \
                -d "{\"model\":\"gpt-4\",\"messages\":[{\"role\":\"user\",\"content\":\"load test agent $i at $(date +%s)\"}]}" \
                2>/dev/null || true
            sleep $INTERVAL
        done
    ) &
done

# Also simulate open HTTP connections (polling-style, approximates SSE/WebSocket load)
for i in $(seq 1 5); do
    (
        curl -s -o /dev/null \
            --max-time $DURATION \
            -H "Authorization: Bearer $JWT" \
            "$PROCESSING_URL/api/orgs/$ORG_ID/sessions?limit=1" \
            2>/dev/null || true
    ) &
done

echo "  Load running... (waiting ${DURATION}s)"
wait
echo "  Load complete at $(date +%H:%M:%S)"

ELAPSED=$((SECONDS - START_EPOCH))
echo "  Actual duration: ${ELAPSED}s"

# ---------------------------------------------------------------------------
# Post-load goroutine count (after load + settle time)
# ---------------------------------------------------------------------------
step "Post-load goroutine count (settling for 5s...)"
sleep 5

POST_LOAD=$(curl -sf \
    -H "X-Internal-Token: $INTERNAL_TOKEN" \
    "$PROCESSING_URL/internal/debug/pprof/goroutine?debug=1" | head -1 | grep -oE '[0-9]+' | head -1)

if [ -z "$POST_LOAD" ]; then
    echo "[FAIL] Could not read post-load goroutine count from pprof"
    exit 1
fi
step "Post-load goroutine count: $POST_LOAD"

GROWTH=$((POST_LOAD - BASELINE))

# ---------------------------------------------------------------------------
# Assertions (D-10)
# ---------------------------------------------------------------------------
step "Assertions: goroutine bounds"

if [[ $POST_LOAD -gt $MAX_GOROUTINES ]]; then
    fail "Goroutine count $POST_LOAD exceeds absolute cap of $MAX_GOROUTINES"
else
    pass "Goroutine count under cap: $POST_LOAD <= $MAX_GOROUTINES"
fi

if [[ $GROWTH -gt $MAX_GROWTH ]]; then
    fail "Goroutine growth $GROWTH exceeds threshold of $MAX_GROWTH (baseline=$BASELINE, post=$POST_LOAD)"
else
    pass "Goroutines bounded: baseline=$BASELINE post=$POST_LOAD growth=$GROWTH (max_growth=$MAX_GROWTH)"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "============================================================"
echo "=== LOAD TEST SUMMARY ==="
echo "  Duration:          ${DURATION}s"
echo "  Concurrent agents: $CONCURRENT"
echo "  Baseline goroutines: $BASELINE"
echo "  Post-load goroutines: $POST_LOAD"
echo "  Growth: $GROWTH"
echo "  Max allowed: $MAX_GOROUTINES (growth cap: $MAX_GROWTH)"
if [ "$OVERALL_PASS" = "true" ]; then
    echo -e "  Overall:      \033[32mPASS\033[0m"
else
    echo -e "  Overall:      \033[31mFAIL\033[0m"
fi
echo "============================================================"

if [ "$OVERALL_PASS" != "true" ]; then
    exit 1
fi
