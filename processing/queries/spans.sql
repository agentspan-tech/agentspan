-- name: CountSpansThisMonth :one
SELECT COUNT(*) FROM spans
WHERE organization_id = $1 AND created_at >= date_trunc('month', NOW());

-- name: InsertSpan :one
INSERT INTO spans (
    session_id, organization_id, provider_type, model,
    input, output, input_tokens, output_tokens,
    cost_usd, duration_ms, http_status, started_at, finish_reason,
    masking_applied, client_disconnected
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id;

-- name: GetModelPrice :one
SELECT input_price_per_token, output_price_per_token
FROM model_prices
WHERE provider_type = $1 AND model = $2;

-- name: GetSpansBySessionID :many
SELECT sp.id, sp.session_id, sp.organization_id, sp.provider_type, sp.model,
       sp.input, sp.output, sp.input_tokens, sp.output_tokens,
       sp.cost_usd, sp.duration_ms, sp.http_status, sp.started_at, sp.created_at,
       sp.finish_reason, sp.masking_applied, sp.anomaly_reason, sp.anomaly_category,
       ssp.system_prompt_id
FROM spans sp
LEFT JOIN span_system_prompts ssp ON ssp.span_id = sp.id
WHERE sp.session_id = $1 AND sp.organization_id = $2
ORDER BY sp.created_at ASC;

-- name: CountErrorSpansForSessions :many
SELECT session_id, COUNT(*) AS error_count
FROM spans
WHERE session_id = ANY(sqlc.arg('session_ids')::uuid[])
  AND http_status >= 400
GROUP BY session_id;

-- name: GetOrgStats :one
SELECT
    COUNT(DISTINCT s.id) AS total_sessions,
    COUNT(sp.id) AS total_spans,
    COALESCE(SUM(sp.cost_usd), 0)::numeric(12,8) AS total_cost_usd,
    COALESCE(AVG(sp.duration_ms), 0)::float8 AS avg_duration_ms,
    CASE WHEN COUNT(sp.id) > 0
         THEN COUNT(sp.id) FILTER (WHERE sp.http_status >= 400)::float8 / COUNT(sp.id)::float8
         ELSE 0::float8
    END AS error_rate
FROM sessions s
LEFT JOIN spans sp ON sp.session_id = s.id
WHERE s.organization_id = $1
  AND s.started_at >= sqlc.arg('from_time')
  AND s.started_at <= sqlc.arg('to_time');

-- name: GetAgentStats :many
SELECT
    ak.id AS api_key_id,
    ak.name AS api_key_name,
    COUNT(DISTINCT s.id) AS session_count,
    COUNT(sp.id) AS span_count,
    COALESCE(SUM(sp.cost_usd), 0)::numeric(12,8) AS total_cost_usd,
    COALESCE(AVG(sp.duration_ms), 0)::float8 AS avg_duration_ms,
    CASE WHEN COUNT(sp.id) > 0
         THEN COUNT(sp.id) FILTER (WHERE sp.http_status >= 400)::float8 / COUNT(sp.id)::float8
         ELSE 0::float8
    END AS error_rate,
    COALESCE(AVG(
        CASE WHEN sp.input_tokens > 0 AND sp.output_tokens IS NOT NULL
             THEN sp.output_tokens::float8 / sp.input_tokens::float8
        END
    ), 0)::float8 AS avg_token_ratio
FROM api_keys ak
JOIN sessions s ON s.api_key_id = ak.id
LEFT JOIN spans sp ON sp.session_id = s.id
WHERE ak.organization_id = $1
  AND s.started_at >= sqlc.arg('from_time')
  AND s.started_at <= sqlc.arg('to_time')
GROUP BY ak.id, ak.name
ORDER BY span_count DESC;

-- name: GetOrgDailyStats :many
SELECT
    date_trunc('day', s.started_at)::date AS day,
    COUNT(DISTINCT s.id) AS session_count,
    COUNT(sp.id) AS span_count,
    COALESCE(SUM(sp.cost_usd), 0)::numeric(12,8) AS cost_usd,
    COUNT(DISTINCT s.id) FILTER (WHERE s.status = 'completed') AS completed_count,
    COUNT(DISTINCT s.id) FILTER (WHERE s.status = 'completed_with_errors') AS with_errors_count,
    COUNT(DISTINCT s.id) FILTER (WHERE s.status = 'failed') AS failed_count,
    COUNT(DISTINCT s.id) FILTER (WHERE s.status = 'abandoned') AS abandoned_count,
    COUNT(DISTINCT s.id) FILTER (WHERE s.status = 'in_progress') AS in_progress_count
FROM sessions s
LEFT JOIN spans sp ON sp.session_id = s.id
WHERE s.organization_id = $1
  AND s.started_at >= CURRENT_DATE - ((sqlc.arg('days')::int - 1) * interval '1 day')
GROUP BY 1
ORDER BY 1 ASC;

-- name: PurgeOldSpans :execrows
DELETE FROM spans
WHERE id IN (
    SELECT id FROM spans
    WHERE created_at < NOW() - (sqlc.arg('retention_days')::int * interval '1 day')
    LIMIT sqlc.arg('batch_size')
);

-- name: PurgeOldSessions :execrows
DELETE FROM sessions
WHERE id IN (
    SELECT id FROM sessions
    WHERE closed_at IS NOT NULL
      AND closed_at < NOW() - (sqlc.arg('retention_days')::int * interval '1 day')
      AND NOT EXISTS (SELECT 1 FROM spans WHERE spans.session_id = sessions.id)
    LIMIT sqlc.arg('batch_size')
);

-- name: PurgeOldAlertEvents :execrows
DELETE FROM alert_events
WHERE id IN (
    SELECT id FROM alert_events
    WHERE triggered_at < NOW() - (sqlc.arg('retention_days')::int * interval '1 day')
    LIMIT sqlc.arg('batch_size')
);

-- name: SetSpanAnomaly :exec
UPDATE spans
SET anomaly_reason = $3, anomaly_category = $4
WHERE id = $1 AND organization_id = $2;

-- name: CountDisconnectedSpansForSessions :many
SELECT session_id, COUNT(*) AS disconnected_count
FROM spans
WHERE session_id = ANY(sqlc.arg('session_ids')::uuid[])
  AND client_disconnected = true
GROUP BY session_id;

-- name: CountAnomalySpansForSessions :many
SELECT session_id, COUNT(*) AS anomaly_count
FROM spans
WHERE session_id = ANY(sqlc.arg('session_ids')::uuid[])
  AND anomaly_reason IS NOT NULL
GROUP BY session_id;

-- name: GetFinishReasonDistribution :many
SELECT
    COALESCE(sp.finish_reason, 'unknown') AS finish_reason,
    COUNT(*) AS count
FROM spans sp
JOIN sessions s ON s.id = sp.session_id
WHERE s.organization_id = $1
  AND s.started_at >= sqlc.arg('from_time')
  AND s.started_at <= sqlc.arg('to_time')
GROUP BY 1
ORDER BY count DESC;
