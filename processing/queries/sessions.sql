-- name: CreateSession :one
INSERT INTO sessions (organization_id, api_key_id, external_id, agent_name, status)
VALUES ($1, $2, $3, $4, 'in_progress')
RETURNING id;

-- name: FindActiveExplicitSession :one
SELECT id FROM sessions
WHERE api_key_id = $1 AND external_id = $2 AND status = 'in_progress'
LIMIT 1;

-- name: FindOrCreateExplicitSession :one
INSERT INTO sessions (organization_id, api_key_id, external_id, agent_name, status)
VALUES ($1, $2, $3, $4, 'in_progress')
ON CONFLICT (api_key_id, external_id) WHERE status = 'in_progress' AND external_id IS NOT NULL
DO UPDATE SET updated_at = NOW()
RETURNING id;

-- name: FindActiveSessionForAPIKey :one
SELECT id FROM sessions
WHERE api_key_id = $1
  AND status = 'in_progress'
  AND last_span_at >= NOW() - (sqlc.arg('timeout_seconds')::int * interval '1 second')
ORDER BY last_span_at DESC
LIMIT 1;

-- name: UpdateSessionAfterSpan :exec
UPDATE sessions
SET span_count = span_count + 1,
    last_span_at = NOW(),
    total_cost_usd = total_cost_usd + sqlc.arg('cost_usd')::numeric(12,8),
    updated_at = NOW()
WHERE id = $1;

-- name: GetIdleSessionsForClosure :many
SELECT s.id, s.organization_id, s.span_count
FROM sessions s
JOIN organizations o ON o.id = s.organization_id
WHERE s.status = 'in_progress'
  AND s.last_span_at < NOW() - (o.session_timeout_seconds * interval '1 second')
LIMIT 200;

-- name: CloseSessionsWithStatus :exec
UPDATE sessions
SET status = sqlc.arg('status'),
    closed_at = NOW(),
    updated_at = NOW()
WHERE id = ANY(sqlc.arg('session_ids')::uuid[])
  AND status = 'in_progress';

-- name: GetSessionByID :one
SELECT s.id, s.organization_id, s.api_key_id, s.external_id, s.agent_name,
       s.status, s.narrative, s.total_cost_usd, s.span_count,
       s.started_at, s.last_span_at, s.closed_at, s.created_at, s.updated_at,
       k.name AS api_key_name
FROM sessions s
JOIN api_keys k ON k.id = s.api_key_id
WHERE s.id = $1 AND s.organization_id = $2;

-- name: ListSessions :many
SELECT s.id, s.organization_id, s.api_key_id, s.external_id, s.agent_name,
       s.status, s.total_cost_usd, s.span_count,
       s.started_at, s.last_span_at, s.closed_at, s.created_at, s.updated_at,
       k.name AS api_key_name
FROM sessions s
JOIN api_keys k ON k.id = s.api_key_id
WHERE s.organization_id = sqlc.arg('org_id')
  AND (sqlc.narg('cursor_started_at')::timestamptz IS NULL
       OR s.started_at < sqlc.narg('cursor_started_at')
       OR (s.started_at = sqlc.narg('cursor_started_at') AND s.id < sqlc.narg('cursor_id')::uuid))
  AND (sqlc.narg('status')::text IS NULL OR s.status = sqlc.narg('status'))
  AND (sqlc.narg('api_key_id')::uuid IS NULL OR s.api_key_id = sqlc.narg('api_key_id'))
  AND (sqlc.narg('agent_name')::text IS NULL OR s.agent_name = sqlc.narg('agent_name'))
  AND (sqlc.narg('from_time')::timestamptz IS NULL OR s.started_at >= sqlc.narg('from_time'))
  AND (sqlc.narg('to_time')::timestamptz IS NULL OR s.started_at <= sqlc.narg('to_time'))
  AND (sqlc.narg('provider_type')::text IS NULL
       OR EXISTS (SELECT 1 FROM spans WHERE spans.session_id = s.id AND spans.provider_type = sqlc.narg('provider_type')))
ORDER BY s.started_at DESC, s.id DESC
LIMIT sqlc.arg('page_limit');

-- name: GetSessionTimeoutForOrg :one
SELECT session_timeout_seconds FROM organizations WHERE id = $1;

-- name: GetSessionOrgID :one
SELECT organization_id FROM sessions WHERE id = $1;
