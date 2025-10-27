-- name: FindVideoByID :one
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    created_at,
    updated_at
FROM catalog.videos
WHERE video_id = $1
  AND status IN ('ready', 'published');

-- name: ListPublicVideos :many
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    created_at,
    updated_at
FROM catalog.videos
WHERE status IN ('ready', 'published')
  AND (
        sqlc.narg('cursor_created_at') IS NULL
        OR created_at < sqlc.narg('cursor_created_at')
        OR (created_at = sqlc.narg('cursor_created_at') AND video_id < sqlc.narg('cursor_video_id'))
      )
ORDER BY created_at DESC, video_id DESC
LIMIT sqlc.arg('limit');

-- name: ListUserUploads :many
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    version,
    created_at,
    updated_at
FROM catalog.videos
WHERE upload_user_id = sqlc.arg('upload_user_id')
  AND (
        sqlc.narg('status_filter') IS NULL
        OR cardinality(sqlc.narg('status_filter')) = 0
        OR status = ANY(sqlc.narg('status_filter'))
      )
  AND (
        sqlc.narg('cursor_created_at') IS NULL
        OR created_at < sqlc.narg('cursor_created_at')
        OR (created_at = sqlc.narg('cursor_created_at') AND video_id < sqlc.narg('cursor_video_id'))
      )
ORDER BY created_at DESC, video_id DESC
LIMIT sqlc.arg('limit');
