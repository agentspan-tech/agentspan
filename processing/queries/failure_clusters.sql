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

-- name: SeedFailureClusters :exec
INSERT INTO failure_clusters (organization_id, label)
VALUES ($1, $2)
ON CONFLICT (organization_id, label) DO NOTHING;

-- name: UpdateSessionNarrative :exec
UPDATE sessions SET narrative = $2, updated_at = NOW() WHERE id = $1 AND organization_id = $3;

-- name: CreateFailureCluster :one
INSERT INTO failure_clusters (organization_id, label)
VALUES ($1, $2)
RETURNING *;

-- name: GetOrganizationByIDForIntel :one
SELECT id, plan, locale FROM organizations WHERE id = $1;
