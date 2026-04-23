-- name: CreateAlertRule :one
INSERT INTO alert_rules (organization_id, name, alert_type, threshold, window_minutes, cooldown_minutes, notify_roles, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetAlertRule :one
SELECT * FROM alert_rules WHERE id = $1 AND organization_id = $2;

-- name: ListAlertRules :many
SELECT * FROM alert_rules WHERE organization_id = $1 ORDER BY created_at DESC LIMIT 200;

-- name: UpdateAlertRule :one
UPDATE alert_rules
SET name = $3, threshold = $4, window_minutes = $5, cooldown_minutes = $6, notify_roles = $7, enabled = $8, updated_at = NOW()
WHERE id = $1 AND organization_id = $2
RETURNING *;

-- name: DeleteAlertRule :exec
DELETE FROM alert_rules WHERE id = $1 AND organization_id = $2;

-- name: CountAlertRules :one
SELECT COUNT(*) FROM alert_rules WHERE organization_id = $1;

-- name: GetEnabledAlertRulesByOrg :many
SELECT ar.* FROM alert_rules ar
JOIN organizations o ON o.id = ar.organization_id
WHERE ar.enabled = TRUE AND o.plan != 'free' AND ar.organization_id = $1
ORDER BY ar.created_at;

-- name: ListNonFreeOrgIDs :many
SELECT DISTINCT ar.organization_id FROM alert_rules ar
JOIN organizations o ON o.id = ar.organization_id
WHERE ar.enabled = TRUE AND o.plan != 'free';

-- name: TryTriggerAlert :one
UPDATE alert_rules
SET last_triggered_at = NOW(), updated_at = NOW()
WHERE id = $1
  AND enabled = TRUE
  AND (last_triggered_at IS NULL
       OR last_triggered_at < NOW() - (cooldown_minutes * interval '1 minute'))
RETURNING id;

-- name: InsertAlertEvent :one
INSERT INTO alert_events (organization_id, alert_rule_id, payload)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListAlertEvents :many
SELECT * FROM alert_events WHERE organization_id = $1 ORDER BY triggered_at DESC LIMIT $2;

-- name: EvaluateFailureRate :one
SELECT
    COUNT(*) AS total_count,
    COUNT(*) FILTER (WHERE http_status >= 400) AS error_count
FROM spans
WHERE organization_id = $1
  AND created_at >= NOW() - (sqlc.arg('window_minutes')::int * interval '1 minute');

-- name: EvaluateAvgLatency :one
SELECT COALESCE(AVG(duration_ms), 0)::float8 AS avg_duration_ms
FROM spans
WHERE organization_id = $1
  AND created_at >= NOW() - (sqlc.arg('window_minutes')::int * interval '1 minute');

-- name: EvaluateErrorSpike :one
SELECT COUNT(*) AS error_count
FROM spans
WHERE organization_id = $1
  AND http_status >= 400
  AND created_at >= NOW() - (sqlc.arg('window_minutes')::int * interval '1 minute');

-- name: GetEnabledAlertRulesByOrgAndType :many
SELECT * FROM alert_rules
WHERE organization_id = $1 AND alert_type = $2 AND enabled = TRUE;

-- name: GetMemberEmailsByRoles :many
SELECT u.email, u.name
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = $1
  AND m.role = ANY(sqlc.arg('roles')::text[]);
