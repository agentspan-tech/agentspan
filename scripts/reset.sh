#!/usr/bin/env bash
# reset.sh — Reset AgentSpan to a clean state (wipes all data)
#
# Usage:
#   bash scripts/reset.sh          # interactive confirmation
#   bash scripts/reset.sh --yes    # skip confirmation

set -euo pipefail

RED='\033[31m'
YELLOW='\033[33m'
GREEN='\033[32m'
RESET='\033[0m'

echo ""
echo -e "${RED}WARNING: This will destroy ALL data:${RESET}"
echo "  - All users, organizations, API keys"
echo "  - All sessions, spans, and metrics"
echo "  - All invites, alerts, and settings"
echo "  - PostgreSQL volume will be deleted"
echo ""

if [ "${1:-}" != "--yes" ]; then
  read -rp "Type 'reset' to confirm: " CONFIRM
  if [ "$CONFIRM" != "reset" ]; then
    echo -e "${YELLOW}Aborted.${RESET}"
    exit 1
  fi
fi

echo ""
echo "Stopping containers..."
docker compose down -v 2>&1

echo ""
echo "Starting fresh..."
docker compose up -d --build 2>&1

echo ""
echo -e "${GREEN}Reset complete.${RESET} AgentSpan is running with a clean database."
echo "Open http://localhost:8081 to create your first account."
