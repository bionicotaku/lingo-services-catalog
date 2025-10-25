-- Video projection (read model) SQL

-- name: UpsertVideoProjection :exec
INSERT INTO catalog.video_projection (
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    created_at,
    updated_at,
    version,
    occurred_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9
)
ON CONFLICT (video_id) DO UPDATE
SET title = EXCLUDED.title,
    status = EXCLUDED.status,
    media_status = EXCLUDED.media_status,
    analysis_status = EXCLUDED.analysis_status,
    updated_at = EXCLUDED.updated_at,
    version = EXCLUDED.version,
    occurred_at = EXCLUDED.occurred_at
WHERE catalog.video_projection.version < EXCLUDED.version;

-- name: DeleteVideoProjection :exec
DELETE FROM catalog.video_projection
WHERE video_id = $1
  AND version <= $2;

-- name: GetVideoProjection :one
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    created_at,
    updated_at,
    version,
    occurred_at
FROM catalog.video_projection
WHERE video_id = $1;
