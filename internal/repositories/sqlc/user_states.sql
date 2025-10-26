-- Video 用户态投影相关 SQL

-- name: UpsertVideoUserState :exec
INSERT INTO catalog.video_user_states (
    user_id,
    video_id,
    has_liked,
    has_bookmarked,
    has_watched,
    occurred_at,
    updated_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    now()
)
ON CONFLICT (user_id, video_id) DO UPDATE
SET has_liked = EXCLUDED.has_liked,
    has_bookmarked = EXCLUDED.has_bookmarked,
    has_watched = EXCLUDED.has_watched,
    occurred_at = GREATEST(catalog.video_user_states.occurred_at, EXCLUDED.occurred_at),
    updated_at = now();

-- name: DeleteVideoUserState :exec
DELETE FROM catalog.video_user_states
WHERE user_id = $1
  AND video_id = $2;

-- name: GetVideoUserState :one
SELECT
    user_id,
    video_id,
    has_liked,
    has_bookmarked,
    has_watched,
    occurred_at,
    updated_at
FROM catalog.video_user_states
WHERE user_id = $1
  AND video_id = $2;
