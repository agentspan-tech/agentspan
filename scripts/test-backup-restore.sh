#!/usr/bin/env bash
# test-backup-restore.sh — verifies pg_dump/pg_restore + migration on a fresh instance.
# Run from the repo root with the stack already running: ./scripts/test-backup-restore.sh
set -euo pipefail

COMPOSE="docker compose"
DB_USER="${POSTGRES_USER:-agentspan}"
DB_NAME="${POSTGRES_DB:-agentspan}"
DUMP_FILE="/tmp/agentspan_test_backup.dump"

echo "=== Step 1: Create backup from running instance ==="
$COMPOSE exec -T postgres pg_dump -U "$DB_USER" -d "$DB_NAME" --format=custom > "$DUMP_FILE"
echo "Backup created: $(du -h "$DUMP_FILE" | cut -f1)"

echo ""
echo "=== Step 2: Record row counts before restore ==="
BEFORE=$($COMPOSE exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" -t -A -c "
  SELECT 'users=' || COUNT(*) FROM users
  UNION ALL SELECT 'organizations=' || COUNT(*) FROM organizations
  UNION ALL SELECT 'sessions=' || COUNT(*) FROM sessions
  UNION ALL SELECT 'spans=' || COUNT(*) FROM spans;
")
echo "$BEFORE"

echo ""
echo "=== Step 3: Stop processing and proxy, keep postgres ==="
$COMPOSE stop processing proxy

echo ""
echo "=== Step 4: Drop and recreate database ==="
$COMPOSE exec -T postgres psql -U "$DB_USER" -d postgres -c "
  SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB_NAME' AND pid <> pg_backend_pid();
"
$COMPOSE exec -T postgres dropdb -U "$DB_USER" --if-exists "$DB_NAME"
$COMPOSE exec -T postgres createdb -U "$DB_USER" "$DB_NAME"
echo "Database recreated (empty)"

echo ""
echo "=== Step 5: Restore backup into fresh database ==="
$COMPOSE exec -T postgres pg_restore \
  -U "$DB_USER" \
  -d "$DB_NAME" \
  --no-owner \
  --no-privileges \
  < "$DUMP_FILE"
echo "Restore complete"

echo ""
echo "=== Step 6: Start processing (runs migrations on top of restored schema) ==="
$COMPOSE up -d processing
echo "Waiting for processing to be healthy..."
for i in $(seq 1 30); do
  if $COMPOSE exec -T processing wget -qO- http://localhost:8081/health > /dev/null 2>&1; then
    echo "Processing healthy after ${i}s"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: Processing did not become healthy in 30s"
    $COMPOSE logs processing --tail=20
    exit 1
  fi
  sleep 1
done

echo ""
echo "=== Step 7: Verify migration version ==="
MIGRATION_VERSION=$($COMPOSE exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" -t -A -c "
  SELECT MAX(version) FROM schema_migrations WHERE dirty = false;
")
echo "Migration version: $MIGRATION_VERSION"

echo ""
echo "=== Step 8: Verify row counts match ==="
AFTER=$($COMPOSE exec -T postgres psql -U "$DB_USER" -d "$DB_NAME" -t -A -c "
  SELECT 'users=' || COUNT(*) FROM users
  UNION ALL SELECT 'organizations=' || COUNT(*) FROM organizations
  UNION ALL SELECT 'sessions=' || COUNT(*) FROM sessions
  UNION ALL SELECT 'spans=' || COUNT(*) FROM spans;
")
echo "$AFTER"

if [ "$BEFORE" = "$AFTER" ]; then
  echo ""
  echo "SUCCESS: Row counts match after restore + migration"
else
  echo ""
  echo "FAILURE: Row counts differ"
  echo "Before: $BEFORE"
  echo "After:  $AFTER"
  exit 1
fi

echo ""
echo "=== Step 9: Restart full stack ==="
$COMPOSE up -d
echo "Stack restored. Test complete."
rm -f "$DUMP_FILE"
