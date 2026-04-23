-- name: FindOrCreateFailureClusterByLabel :one
INSERT INTO failure_clusters (organization_id, label)
VALUES ($1, $2)
ON CONFLICT (organization_id, label) DO UPDATE SET updated_at = NOW()
RETURNING *;

-- name: GetFailureClusterByID :one
SELECT * FROM failure_clusters
WHERE id = $1 AND organization_id = $2;

-- name: GetFailureClusterByLabel :one
SELECT * FROM failure_clusters
WHERE organization_id = $1 AND label = $2;

-- name: InsertSessionFailureCluster :exec
INSERT INTO session_failure_clusters (session_id, failure_cluster_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: IncrementFailureClusterCount :exec
UPDATE failure_clusters
SET session_count = session_count + 1, updated_at = NOW()
WHERE id = $1 AND organization_id = $2;

-- name: ListFailureClusterLabels :many
SELECT id, label FROM failure_clusters
WHERE organization_id = $1
ORDER BY session_count DESC
LIMIT 50;

-- name: UpdateSessionNarrative :exec
UPDATE sessions SET narrative = $2, updated_at = NOW() WHERE id = $1 AND organization_id = $3;

-- name: CreateFailureCluster :one
INSERT INTO failure_clusters (organization_id, label)
VALUES ($1, $2)
RETURNING *;

-- name: ListFailureClusters :many
SELECT id, label, session_count, created_at, updated_at
FROM failure_clusters
WHERE organization_id = $1
ORDER BY session_count DESC;

-- name: ListSessionsByCluster :many
SELECT s.id, s.api_key_id, s.status, s.span_count,
       s.started_at, s.last_span_at, s.closed_at,
       k.name AS api_key_name,
       s.agent_name
FROM session_failure_clusters sfc
JOIN sessions s ON s.id = sfc.session_id
JOIN api_keys k ON k.id = s.api_key_id
WHERE sfc.failure_cluster_id = $1 AND s.organization_id = $2
ORDER BY s.started_at DESC
LIMIT 100;

-- name: GetOrganizationByIDForIntel :one
SELECT id, plan, locale FROM organizations WHERE id = $1;
