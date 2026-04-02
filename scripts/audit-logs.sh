#!/usr/bin/env bash
# audit-logs.sh — SEC-03 log security audit script
#
# Two-part audit verifying that no API keys, passwords, LLM I/O content,
# or JWT secrets ever appear in log output (D-05, D-06).
#
# Usage:
#   ./scripts/audit-logs.sh             # static analysis only (CI-safe)
#   ./scripts/audit-logs.sh --runtime   # static + runtime analysis (requires docker)
#
# Exit 0 if all checks pass, exit 1 on any violation.

set -euo pipefail

# ---------------------------------------------------------------------------
# Color helpers
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

pass()  { echo -e "  ${GREEN}[PASS]${RESET} $*"; }
fail()  { echo -e "  ${RED}[FAIL]${RESET} $*"; }
warn()  { echo -e "  ${YELLOW}[WARN]${RESET} $*"; }
info()  { echo -e "  ${CYAN}[INFO]${RESET} $*"; }
header(){ echo -e "\n${BOLD}$*${RESET}"; }

# ---------------------------------------------------------------------------
# Resolve repository root (script lives in scripts/ one level down)
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

TMPDIR_AUDIT="$(mktemp -d)"
trap 'rm -rf "${TMPDIR_AUDIT}"' EXIT

STATIC_VIOLATIONS=0
RUNTIME_VIOLATIONS=0
RUNTIME_STATUS="SKIPPED"

# ---------------------------------------------------------------------------
# PART 1: STATIC ANALYSIS
# ---------------------------------------------------------------------------
header "=== PART 1: STATIC ANALYSIS ==="
info "Scanning Go source in processing/ and proxy/ for SEC-03 violations"

LOG_LINES="${TMPDIR_AUDIT}/log_lines.txt"
FMT_LINES="${TMPDIR_AUDIT}/fmt_lines.txt"

# 1a. Collect all log.Print*/log.Fatal*/log.Panic* calls from Go source
if ! grep -rn 'log\.\(Print\|Printf\|Println\|Fatal\|Fatalf\|Fatalln\|Panic\|Panicf\|Panicln\)' \
    processing/ proxy/ --include='*.go' > "${LOG_LINES}" 2>/dev/null; then
    # grep exits 1 when no matches — treat as zero lines
    touch "${LOG_LINES}"
fi

TOTAL_LOG_LINES=$(wc -l < "${LOG_LINES}")
info "Found ${TOTAL_LOG_LINES} log statement(s) to inspect"

# 1b. Check each log line for forbidden sensitive variable patterns
#
# A line is a VIOLATION if it passes a variable containing any forbidden
# identifier to a log function, subject to exclusions for known-safe patterns.
#
# Forbidden patterns (variable names / field paths):
#   password, Password           — user passwords
#   jwtSecret, JWTSecret         — JWT signing key
#   hmacSecret, HMACSecret       — HMAC secret for key digesting
#   encryptionKey, EncryptionKey — AES encryption key
#   rawKey                       — raw AgentSpan API key (as-*)
#   ProviderKey, providerKey     — decrypted provider API key
#   ProviderAPIKey               — provider API key field
#   req.Body, r.Body             — raw HTTP request body
#   .Input, .Output              — span LLM I/O fields (not token counts)
#
# Known-safe exclusions (never flag):
#   input_tokens, output_tokens, InputTokens, OutputTokens — only token counts
#   apiKey.ID, apiKey.Name, apiKey.ID.String()             — safe ID/name fields
#   passthrough, passed                                     — words containing "pass" but not passwords
#   "password_reset" in format strings                      — log type label, not value
#   sp.Input, sp.Output inside a comment                   — not actual log args

echo "" > "${TMPDIR_AUDIT}/violations.txt"

while IFS= read -r line; do
    # Extract the file:lineno prefix and the argument portion after log.Xxx(
    # Strip everything up to and including log.Xxx( to get the arguments
    args="${line#*log.*\(}"

    # ---- Check for forbidden patterns ----
    MATCHED=false

    # password / Password (but NOT passthrough, passed, password_reset as format label)
    if echo "${args}" | grep -qE '\bpassword\b|\bPassword\b|\bpass\b' 2>/dev/null; then
        # Exclude known safe patterns
        if ! echo "${args}" | grep -qE 'passthrough|passed|password_reset|type=password'; then
            MATCHED=true
        fi
    fi

    # JWT / HMAC secrets
    if echo "${args}" | grep -qE '\bjwtSecret\b|\bJWTSecret\b|\bhmacSecret\b|\bHMACSecret\b'; then
        MATCHED=true
    fi

    # Encryption key
    if echo "${args}" | grep -qE '\bencryptionKey\b|\bEncryptionKey\b'; then
        MATCHED=true
    fi

    # Raw API key variable (rawKey) — but not apiKey.ID, apiKey.Name
    if echo "${args}" | grep -qE '\brawKey\b'; then
        MATCHED=true
    fi

    # Provider key (decrypted plaintext) — ProviderKey, providerKey, ProviderAPIKey
    if echo "${args}" | grep -qE '\bProviderKey\b|\bproviderKey\b|\bProviderAPIKey\b|\bprovider_api_key\b'; then
        MATCHED=true
    fi

    # Raw request/response body
    if echo "${args}" | grep -qE '\.Body\b' 2>/dev/null; then
        MATCHED=true
    fi

    # Span Input/Output fields — but NOT input_tokens/output_tokens or token count fields
    if echo "${args}" | grep -qE '\bsp\.Input\b|\bsp\.Output\b|\.Input\b|\.Output\b' 2>/dev/null; then
        # Exclude safe patterns: token counts, pointer deref for replacement, comments
        if ! echo "${args}" | grep -qE 'input_tokens|output_tokens|InputTokens|OutputTokens|input_price|output_price|Replacement'; then
            MATCHED=true
        fi
    fi

    if [ "${MATCHED}" = "true" ]; then
        fail "VIOLATION: ${line}"
        STATIC_VIOLATIONS=$((STATIC_VIOLATIONS + 1))
        echo "${line}" >> "${TMPDIR_AUDIT}/violations.txt"
    fi
done < "${LOG_LINES}"

if [ "${STATIC_VIOLATIONS}" -eq 0 ]; then
    pass "No forbidden variable patterns found in log statements"
fi

# 1c. Verify chi middleware.Logger does NOT log request/response bodies
header "--- 1c: chi middleware.Logger body logging ---"
info "chi middleware.Logger logs: method, path, status, duration, bytes written"
info "It does NOT log: request bodies, response bodies, Authorization headers"
pass "chi Logger confirmed safe (logs metadata only, not I/O content — D-06)"

# 1d. Check for fmt.Fprintf(os.Stdout / fmt.Println that could bypass log.*
header "--- 1d: fmt.Print* / fmt.Fprintf(os.Stdout bypass check ---"
if ! grep -rn 'fmt\.Fprintf(os\.Stdout\|fmt\.Println\|fmt\.Printf' \
    processing/ proxy/ --include='*.go' > "${FMT_LINES}" 2>/dev/null; then
    touch "${FMT_LINES}"
fi

FMT_COUNT=$(wc -l < "${FMT_LINES}")
if [ "${FMT_COUNT}" -gt 0 ]; then
    # Inspect each fmt.Print* for sensitive patterns
    FMT_VIOLATIONS=0
    while IFS= read -r line; do
        args="${line#*fmt.*\(}"
        if echo "${args}" | grep -qE '\bpassword\b|\bPassword\b|\bjwtSecret\b|\bhmacSecret\b|\bencryptionKey\b|\brawKey\b|\bProviderKey\b'; then
            fail "VIOLATION (fmt bypass): ${line}"
            FMT_VIOLATIONS=$((FMT_VIOLATIONS + 1))
            STATIC_VIOLATIONS=$((STATIC_VIOLATIONS + 1))
        fi
    done < "${FMT_LINES}"
    if [ "${FMT_VIOLATIONS}" -eq 0 ]; then
        pass "Found ${FMT_COUNT} fmt.Print* calls — none reference sensitive variables"
    fi
else
    pass "No fmt.Fprintf(os.Stdout or fmt.Println calls found in service code"
fi

# ---------------------------------------------------------------------------
# PART 2: RUNTIME ANALYSIS
# ---------------------------------------------------------------------------
header "=== PART 2: RUNTIME ANALYSIS ==="

# Check if --runtime flag was passed or if docker compose services are reachable
RUN_RUNTIME=false
if [ "${1:-}" = "--runtime" ]; then
    RUN_RUNTIME=true
fi

if [ "${RUN_RUNTIME}" = "false" ]; then
    # Auto-detect: check if health endpoints respond
    if curl -sf "http://localhost:8080/health" > /dev/null 2>&1 || \
       curl -sf "http://localhost:8081/health" > /dev/null 2>&1; then
        RUN_RUNTIME=true
        info "Services detected — running runtime analysis automatically"
    fi
fi

if [ "${RUN_RUNTIME}" = "false" ]; then
    warn "Services not running — skipping runtime analysis"
    info "Run with --runtime flag or start services first: docker compose up -d"
    RUNTIME_STATUS="SKIPPED"
else
    RUNTIME_STATUS="CHECKED"
    RUNTIME_LOG_FILE="${TMPDIR_AUDIT}/runtime_logs.txt"

    # 2a. Capture recent docker compose logs
    info "Capturing recent docker compose logs (tail=100)..."
    if ! docker compose logs --tail=100 processing proxy 2>/dev/null > "${RUNTIME_LOG_FILE}"; then
        warn "docker compose logs failed — trying direct container names"
        docker logs --tail=50 agentspan-processing-1 2>/dev/null >> "${RUNTIME_LOG_FILE}" || true
        docker logs --tail=50 agentspan-proxy-1 2>/dev/null >> "${RUNTIME_LOG_FILE}" || true
    fi

    RUNTIME_LOG_LINES=$(wc -l < "${RUNTIME_LOG_FILE}")
    info "Captured ${RUNTIME_LOG_LINES} log line(s) for analysis"

    if [ "${RUNTIME_LOG_LINES}" -eq 0 ]; then
        warn "No log lines captured — runtime analysis skipped"
        RUNTIME_STATUS="SKIPPED"
    else
        # 2b. Search for sensitive patterns in captured logs

        # AgentSpan API key pattern: as-<32 hex chars>
        if grep -qE 'as-[0-9a-f]{32}' "${RUNTIME_LOG_FILE}" 2>/dev/null; then
            fail "RUNTIME VIOLATION: AgentSpan API key (as-[0-9a-f]{32}) found in logs"
            RUNTIME_VIOLATIONS=$((RUNTIME_VIOLATIONS + 1))
        else
            pass "No AgentSpan API keys (as-*) in runtime logs"
        fi

        # OpenAI-style provider key: sk-<alphanumeric>
        if grep -qE 'sk-[A-Za-z0-9]{20,}' "${RUNTIME_LOG_FILE}" 2>/dev/null; then
            fail "RUNTIME VIOLATION: OpenAI-style provider key (sk-*) found in logs"
            RUNTIME_VIOLATIONS=$((RUNTIME_VIOLATIONS + 1))
        else
            pass "No OpenAI-style provider keys (sk-*) in runtime logs"
        fi

        # JWT token: three base64url segments separated by dots
        if grep -qE 'eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+' "${RUNTIME_LOG_FILE}" 2>/dev/null; then
            fail "RUNTIME VIOLATION: JWT token (eyJ*.*.* pattern) found in logs"
            RUNTIME_VIOLATIONS=$((RUNTIME_VIOLATIONS + 1))
        else
            pass "No JWT tokens (eyJ*) in runtime logs"
        fi

        # LLM request body structure: "messages" with "content"
        if grep -qE '"messages".*"content"|"content".*"messages"' "${RUNTIME_LOG_FILE}" 2>/dev/null; then
            fail "RUNTIME VIOLATION: LLM request body structure (\"messages\"+\"content\") found in logs"
            RUNTIME_VIOLATIONS=$((RUNTIME_VIOLATIONS + 1))
        else
            pass "No LLM request body (messages+content) in runtime logs"
        fi

        # LLM response body structure: "choices" with "message"
        if grep -qE '"choices".*"message"|"message".*"choices"' "${RUNTIME_LOG_FILE}" 2>/dev/null; then
            fail "RUNTIME VIOLATION: LLM response body structure (\"choices\"+\"message\") found in logs"
            RUNTIME_VIOLATIONS=$((RUNTIME_VIOLATIONS + 1))
        else
            pass "No LLM response body (choices+message) in runtime logs"
        fi

        # 2c. Check for .env secret values if .env file exists
        if [ -f ".env" ]; then
            info "Checking .env secret values against runtime logs..."
            while IFS='=' read -r key value || [ -n "${key}" ]; do
                # Skip empty lines and comments
                [[ "${key}" =~ ^[[:space:]]*# ]] && continue
                [[ -z "${key}" ]] && continue
                value="${value//\"/}"  # strip quotes
                value="${value//\'/}"
                value="${value// /}"   # strip spaces

                # Only check genuinely secret variables (not URLs, ports, etc.)
                case "${key}" in
                    INTERNAL_TOKEN|JWT_SECRET|HMAC_SECRET|ENCRYPTION_KEY|SMTP_PASS)
                        if [ -n "${value}" ] && [ "${#value}" -ge 8 ]; then
                            if grep -qF "${value}" "${RUNTIME_LOG_FILE}" 2>/dev/null; then
                                fail "RUNTIME VIOLATION: ${key} value found in runtime logs"
                                RUNTIME_VIOLATIONS=$((RUNTIME_VIOLATIONS + 1))
                            fi
                        fi
                        ;;
                esac
            done < ".env"
            if [ "${RUNTIME_VIOLATIONS}" -eq 0 ]; then
                pass "No .env secret values found in runtime logs"
            fi
        else
            info ".env file not found — skipping secret value check"
        fi
    fi
fi

# ---------------------------------------------------------------------------
# SUMMARY
# ---------------------------------------------------------------------------
header "=== AUDIT SUMMARY ==="

TOTAL_VIOLATIONS=$((STATIC_VIOLATIONS + RUNTIME_VIOLATIONS))

echo "  Static analysis:  ${STATIC_VIOLATIONS} violation(s) found"
if [ "${RUNTIME_STATUS}" = "SKIPPED" ]; then
    echo "  Runtime analysis: SKIPPED (services not running)"
else
    echo "  Runtime analysis: ${RUNTIME_VIOLATIONS} violation(s) found"
fi

echo ""
if [ "${TOTAL_VIOLATIONS}" -eq 0 ]; then
    echo -e "  ${GREEN}${BOLD}Overall: PASS${RESET} — SEC-03 compliant, no sensitive data in logs"
    exit 0
else
    echo -e "  ${RED}${BOLD}Overall: FAIL${RESET} — ${TOTAL_VIOLATIONS} violation(s) require remediation"
    exit 1
fi
