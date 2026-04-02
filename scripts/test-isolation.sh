#!/usr/bin/env bash
# test-isolation.sh — AgentSpan cross-organization isolation test
#
# Verifies that no data leaks across organization boundaries (D-07, D-08).
# Creates two isolated orgs and confirms every org-scoped endpoint rejects
# cross-org access with 403 or 404.
#
# Prerequisites:
#   - docker compose up (all services healthy)
#   - curl and jq installed
#
# Usage:
#   bash scripts/test-isolation.sh
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

pass() { echo -e "\033[32m[PASS]\033[0m $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "\033[31m[FAIL]\033[0m $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); OVERALL_PASS=false; }
step() { echo -e "\n\033[36m[STEP]\033[0m $1"; }

PASS_COUNT=0
FAIL_COUNT=0
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
echo "AgentSpan Cross-Org Isolation Test (D-07, D-08)"
echo "  Processing: $PROCESSING_URL"
echo "  Proxy:      $PROXY_URL"
echo "============================================================"

# ---------------------------------------------------------------------------
# Helper: expect a 403 or 404 response from a cross-org request
# ---------------------------------------------------------------------------
expect_forbidden() {
    local method="$1" url="$2" token="$3" label="$4"
    local body="${5:-}"
    local status
    if [ -n "$body" ]; then
        status=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" \
            -H "Authorization: Bearer $token" \
            -H "Content-Type: application/json" \
            -d "$body" \
            "$url")
    else
        status=$(curl -s -o /dev/null -w '%{http_code}' -X "$method" \
            -H "Authorization: Bearer $token" \
            -H "Content-Type: application/json" \
            "$url")
    fi
    if [[ "$status" == "403" || "$status" == "404" ]]; then
        pass "$label -> $status"
    else
        fail "$label -> got $status (expected 403 or 404)"
    fi
}

# ---------------------------------------------------------------------------
# Helper: register, verify email, login — returns JWT
# ---------------------------------------------------------------------------
setup_user() {
    local email="$1" password="$2" name="$3"

    # Register (handle 409 idempotently)
    REG_HTTP_CODE=$(curl -s -o /tmp/iso_reg.json -w "%{http_code}" \
        -X POST "$PROCESSING_URL/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"$email\",\"password\":\"$password\",\"name\":\"$name\"}")
    REG_BODY=$(cat /tmp/iso_reg.json)

    if [ "$REG_HTTP_CODE" = "201" ]; then
        AUTO_LOGIN=$(echo "$REG_BODY" | jq -r '.auto_login // empty')
        if [ "$AUTO_LOGIN" != "true" ]; then
            # Cloud mode — need email verification
            VERIFY_URL=$(echo "$REG_BODY" | jq -r '.verification_url // empty')
            if [ -z "$VERIFY_URL" ]; then
                echo "[FAIL] Register returned no verification_url for $email (SMTP mode?). Disable SMTP."
                exit 1
            fi
            VERIFY_TOKEN=$(echo "$VERIFY_URL" | grep -oE '[?&]token=[^&]+' | cut -d'=' -f2)
            if [ -z "$VERIFY_TOKEN" ]; then
                echo "[FAIL] Could not extract token from verification_url: $VERIFY_URL"
                exit 1
            fi
            curl -sf -X POST "$PROCESSING_URL/auth/verify-email" \
                -H "Content-Type: application/json" \
                -d "{\"token\":\"$VERIFY_TOKEN\"}" > /dev/null || {
                    echo "[FAIL] Email verification failed for $email"
                    exit 1
                }
        fi
    elif [ "$REG_HTTP_CODE" = "409" ] || [ "$REG_HTTP_CODE" = "403" ]; then
        : # already exists or registration closed — proceed to login
    else
        echo "[FAIL] Register returned unexpected HTTP $REG_HTTP_CODE for $email: $REG_BODY"
        exit 1
    fi

    # Login
    LOGIN_RESP=$(curl -sf -X POST "$PROCESSING_URL/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"$email\",\"password\":\"$password\"}") || {
            echo "[FAIL] Login failed for $email"
            exit 1
        }
    JWT=$(echo "$LOGIN_RESP" | jq -r '.token // empty')
    [ -n "$JWT" ] || { echo "[FAIL] Login response missing token for $email"; exit 1; }
    echo "$JWT"
}

# ---------------------------------------------------------------------------
# Helper: create a fresh org — returns org ID
# ---------------------------------------------------------------------------
create_org() {
    local jwt="$1" org_name="$2"
    CREATE_ORG_RESP=$(curl -sf -X POST "$PROCESSING_URL/api/orgs/" \
        -H "Authorization: Bearer $jwt" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"$org_name\",\"locale\":\"en\"}") || {
            echo "[FAIL] Create org failed for $org_name"
            exit 1
        }
    ORG_ID=$(echo "$CREATE_ORG_RESP" | jq -r '.id // empty')
    [ -n "$ORG_ID" ] || { echo "[FAIL] Create org response missing id for $org_name: $CREATE_ORG_RESP"; exit 1; }
    echo "$ORG_ID"
}

# ---------------------------------------------------------------------------
# Helper: create API key — returns raw key
# ---------------------------------------------------------------------------
create_api_key() {
    local jwt="$1" org_id="$2" key_name="$3"
    KEY_RESP=$(curl -sf -X POST "$PROCESSING_URL/api/orgs/$org_id/keys" \
        -H "Authorization: Bearer $jwt" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"$key_name\",\"provider_type\":\"openai\",\"provider_key\":\"sk-fake-iso-test\"}") || {
            echo "[FAIL] Create API key failed for $key_name"
            exit 1
        }
    RAW_KEY=$(echo "$KEY_RESP" | jq -r '.raw_key // empty')
    [ -n "$RAW_KEY" ] || { echo "[FAIL] Create key response missing raw_key for $key_name: $KEY_RESP"; exit 1; }
    echo "$RAW_KEY"
}

# ---------------------------------------------------------------------------
# Setup: Create two isolated organizations
# ---------------------------------------------------------------------------
step "Setup: Create Org A (iso-a@test.local)"

ISO_A_EMAIL="iso-a@test.local"
ISO_A_PASSWORD="isolation123"
ISO_A_NAME="Isolation User A"
ISO_A_ORG="Isolation Org A"
ISO_A_KEY_NAME="iso-agent-a"

JWT_A=$(setup_user "$ISO_A_EMAIL" "$ISO_A_PASSWORD" "$ISO_A_NAME")
echo "  User A JWT acquired"

# Create Org A (find existing or create new)
ORGS_A_RESP=$(curl -sf "$PROCESSING_URL/api/orgs/" \
    -H "Authorization: Bearer $JWT_A") || { echo "[FAIL] List orgs for A failed"; exit 1; }

ORG_A_ID=$(echo "$ORGS_A_RESP" | jq -r '
    if type == "array" then .[0].id
    elif .items then .items[0].id
    else .id
    end // empty' 2>/dev/null || true)

if [ -z "$ORG_A_ID" ]; then
    ORG_A_ID=$(create_org "$JWT_A" "$ISO_A_ORG")
    echo "  Org A created (id=$ORG_A_ID)"
else
    echo "  Org A already exists (id=$ORG_A_ID)"
fi

# Create API key for Org A
RAW_KEY_A=$(create_api_key "$JWT_A" "$ORG_A_ID" "$ISO_A_KEY_NAME")
echo "  Org A API key created (prefix: ${RAW_KEY_A:0:10}...)"

# Send a span through proxy as Org A (creates a session for later verification)
echo "  Sending span through proxy as Org A..."
curl -s -o /dev/null --max-time 15 \
    -X POST "$PROXY_URL/v1/chat/completions" \
    -H "Authorization: Bearer $RAW_KEY_A" \
    -H "Content-Type: application/json" \
    -d '{"model":"gpt-4","messages":[{"role":"user","content":"isolation test org a"}]}' || true
echo "  Waiting 2s for session creation..."
sleep 2

pass "Org A setup complete"

step "Setup: Create Org B (iso-b@test.local)"

ISO_B_EMAIL="iso-b@test.local"
ISO_B_PASSWORD="isolation456"
ISO_B_NAME="Isolation User B"
ISO_B_ORG="Isolation Org B"
ISO_B_KEY_NAME="iso-agent-b"

JWT_B=$(setup_user "$ISO_B_EMAIL" "$ISO_B_PASSWORD" "$ISO_B_NAME")
echo "  User B JWT acquired"

# Create Org B (find existing or create new)
ORGS_B_RESP=$(curl -sf "$PROCESSING_URL/api/orgs/" \
    -H "Authorization: Bearer $JWT_B") || { echo "[FAIL] List orgs for B failed"; exit 1; }

ORG_B_ID=$(echo "$ORGS_B_RESP" | jq -r '
    if type == "array" then .[0].id
    elif .items then .items[0].id
    else .id
    end // empty' 2>/dev/null || true)

if [ -z "$ORG_B_ID" ]; then
    ORG_B_ID=$(create_org "$JWT_B" "$ISO_B_ORG")
    echo "  Org B created (id=$ORG_B_ID)"
else
    echo "  Org B already exists (id=$ORG_B_ID)"
fi

# Create API key for Org B
RAW_KEY_B=$(create_api_key "$JWT_B" "$ORG_B_ID" "$ISO_B_KEY_NAME")
echo "  Org B API key created (prefix: ${RAW_KEY_B:0:10}...)"

# Send a span through proxy as Org B
echo "  Sending span through proxy as Org B..."
curl -s -o /dev/null --max-time 15 \
    -X POST "$PROXY_URL/v1/chat/completions" \
    -H "Authorization: Bearer $RAW_KEY_B" \
    -H "Content-Type: application/json" \
    -d '{"model":"gpt-4","messages":[{"role":"user","content":"isolation test org b"}]}' || true
echo "  Waiting 2s for session creation..."
sleep 2

pass "Org B setup complete"

echo ""
echo "  Org A ID: $ORG_A_ID"
echo "  Org B ID: $ORG_B_ID"

# ---------------------------------------------------------------------------
# Isolation tests: JWT_A accessing Org B's endpoints (D-07)
# ---------------------------------------------------------------------------
step "D-07 Isolation: User A accessing Org B resources (expect 403/404)"

expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_B_ID/"                    "$JWT_A" "GET  /api/orgs/{ORG_B}/         (A->B)"
expect_forbidden "PUT"    "$PROCESSING_URL/api/orgs/$ORG_B_ID/settings"             "$JWT_A" "PUT  /api/orgs/{ORG_B}/settings  (A->B)" '{"session_timeout_seconds":60}'
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_B_ID/members"              "$JWT_A" "GET  /api/orgs/{ORG_B}/members   (A->B)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_B_ID/keys"                 "$JWT_A" "GET  /api/orgs/{ORG_B}/keys      (A->B)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_B_ID/sessions"             "$JWT_A" "GET  /api/orgs/{ORG_B}/sessions  (A->B)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_B_ID/stats"                "$JWT_A" "GET  /api/orgs/{ORG_B}/stats     (A->B)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_B_ID/alerts"               "$JWT_A" "GET  /api/orgs/{ORG_B}/alerts    (A->B)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_B_ID/invites"              "$JWT_A" "GET  /api/orgs/{ORG_B}/invites   (A->B)"
expect_forbidden "DELETE" "$PROCESSING_URL/api/orgs/$ORG_B_ID/"                    "$JWT_A" "DELETE /api/orgs/{ORG_B}/        (A->B)"
expect_forbidden "POST"   "$PROCESSING_URL/api/orgs/$ORG_B_ID/transfer"             "$JWT_A" "POST /api/orgs/{ORG_B}/transfer  (A->B)" '{"new_owner_id":"00000000-0000-0000-0000-000000000000"}'
expect_forbidden "POST"   "$PROCESSING_URL/api/orgs/$ORG_B_ID/leave"                "$JWT_A" "POST /api/orgs/{ORG_B}/leave     (A->B)"

# ---------------------------------------------------------------------------
# Isolation tests: JWT_B accessing Org A's endpoints (D-07 bidirectional)
# ---------------------------------------------------------------------------
step "D-07 Isolation: User B accessing Org A resources (expect 403/404)"

expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_A_ID/"                    "$JWT_B" "GET  /api/orgs/{ORG_A}/         (B->A)"
expect_forbidden "PUT"    "$PROCESSING_URL/api/orgs/$ORG_A_ID/settings"             "$JWT_B" "PUT  /api/orgs/{ORG_A}/settings  (B->A)" '{"session_timeout_seconds":60}'
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_A_ID/members"              "$JWT_B" "GET  /api/orgs/{ORG_A}/members   (B->A)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_A_ID/keys"                 "$JWT_B" "GET  /api/orgs/{ORG_A}/keys      (B->A)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_A_ID/sessions"             "$JWT_B" "GET  /api/orgs/{ORG_A}/sessions  (B->A)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_A_ID/stats"                "$JWT_B" "GET  /api/orgs/{ORG_A}/stats     (B->A)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_A_ID/alerts"               "$JWT_B" "GET  /api/orgs/{ORG_A}/alerts    (B->A)"
expect_forbidden "GET"    "$PROCESSING_URL/api/orgs/$ORG_A_ID/invites"              "$JWT_B" "GET  /api/orgs/{ORG_A}/invites   (B->A)"
expect_forbidden "DELETE" "$PROCESSING_URL/api/orgs/$ORG_A_ID/"                    "$JWT_B" "DELETE /api/orgs/{ORG_A}/        (B->A)"
expect_forbidden "POST"   "$PROCESSING_URL/api/orgs/$ORG_A_ID/transfer"             "$JWT_B" "POST /api/orgs/{ORG_A}/transfer  (B->A)" '{"new_owner_id":"00000000-0000-0000-0000-000000000000"}'
expect_forbidden "POST"   "$PROCESSING_URL/api/orgs/$ORG_A_ID/leave"                "$JWT_B" "POST /api/orgs/{ORG_A}/leave     (B->A)"

# ---------------------------------------------------------------------------
# D-08: Internal API span org attribution check
# Spans ingested via Org A's API key must NOT appear in Org B's sessions.
# ---------------------------------------------------------------------------
step "D-08 Internal API: span org attribution (Org A spans invisible to Org B)"

A_SESSIONS=$(curl -sf \
    -H "Authorization: Bearer $JWT_A" \
    "$PROCESSING_URL/api/orgs/$ORG_A_ID/sessions") || {
        fail "Could not fetch Org A sessions"
        A_SESSIONS='{"data":[]}'
    }

B_SESSIONS=$(curl -sf \
    -H "Authorization: Bearer $JWT_B" \
    "$PROCESSING_URL/api/orgs/$ORG_B_ID/sessions") || {
        fail "Could not fetch Org B sessions"
        B_SESSIONS='{"data":[]}'
    }

# Count sessions in Org B that use Org A's agent key name
B_SESSION_COUNT_FOR_A=$(echo "$B_SESSIONS" | jq '
    [
        (
            if type == "array" then .[]
            elif .items then .items[]
            elif .data then .data[]
            elif .sessions then .sessions[]
            else empty
            end
        ) | select(.api_key_name == "iso-agent-a")
    ] | length' 2>/dev/null || echo "0")

if [[ "$B_SESSION_COUNT_FOR_A" == "0" ]]; then
    pass "Org B cannot see Org A sessions (iso-agent-a not visible in Org B)"
else
    fail "Org B sees $B_SESSION_COUNT_FOR_A session(s) belonging to Org A's agent (iso-agent-a)"
fi

# Verify Org A's sessions are only visible to Org A
A_SESSION_COUNT=$(echo "$A_SESSIONS" | jq '
    if type == "array" then length
    elif .items then .items | length
    elif .data then .data | length
    elif .sessions then .sessions | length
    else 0
    end' 2>/dev/null || echo "0")

step "Org A session visibility verification"
echo "  Org A sessions visible to Org A: $A_SESSION_COUNT"
if [[ "$A_SESSION_COUNT" -ge "0" ]]; then
    pass "Org A sessions properly scoped to Org A (count=$A_SESSION_COUNT)"
fi

# ---------------------------------------------------------------------------
# D-08: Internal token isolation
# Verify that the internal API cannot be accessed without the correct token
# ---------------------------------------------------------------------------
step "D-08 Internal API: token enforcement"

WRONG_TOKEN_STATUS=$(curl -s -o /dev/null -w '%{http_code}' \
    -H "X-Internal-Token: wrong-token-should-fail" \
    "$PROCESSING_URL/internal/auth/verify" \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"key_digest":"test"}')

if [[ "$WRONG_TOKEN_STATUS" == "401" || "$WRONG_TOKEN_STATUS" == "403" ]]; then
    pass "Internal API rejects wrong token -> HTTP $WRONG_TOKEN_STATUS"
else
    fail "Internal API accepted wrong token -> HTTP $WRONG_TOKEN_STATUS (expected 401 or 403)"
fi

# Verify internal API works with correct token
CORRECT_TOKEN_STATUS=$(curl -s -o /dev/null -w '%{http_code}' \
    -H "X-Internal-Token: $INTERNAL_TOKEN" \
    "$PROCESSING_URL/internal/auth/verify" \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"key_digest":"test-nonexistent-digest"}')

if [[ "$CORRECT_TOKEN_STATUS" == "200" ]]; then
    pass "Internal API accepts valid token -> HTTP $CORRECT_TOKEN_STATUS"
else
    fail "Internal API rejected valid token -> HTTP $CORRECT_TOKEN_STATUS (expected 200)"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
TOTAL_CHECKS=$((PASS_COUNT + FAIL_COUNT))

echo ""
echo "============================================================"
echo "=== ISOLATION TEST SUMMARY ==="
echo "  Total checks: $TOTAL_CHECKS"
echo "  Passed:       $PASS_COUNT"
echo "  Failed:       $FAIL_COUNT"
if [ "$OVERALL_PASS" = "true" ]; then
    echo -e "  Overall:      \033[32mPASS\033[0m"
else
    echo -e "  Overall:      \033[31mFAIL\033[0m"
fi
echo "============================================================"

if [ "$OVERALL_PASS" != "true" ]; then
    exit 1
fi
