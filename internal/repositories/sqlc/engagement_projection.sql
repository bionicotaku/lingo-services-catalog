-- Video 用户态投影相关 SQL

-- name: UpsertVideoUserState :exec
INSERT INTO catalog.video_user_engagements_projection (
    user_id,
    video_id,
    has_liked,
    has_bookmarked,
    liked_occurred_at,
    bookmarked_occurred_at,
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
    liked_occurred_at = EXCLUDED.liked_occurred_at,
    bookmarked_occurred_at = EXCLUDED.bookmarked_occurred_at,
    updated_at = now();

-- name: DeleteVideoUserState :exec
DELETE FROM catalog.video_user_engagements_projection
WHERE user_id = $1
  AND video_id = $2;

-- name: GetVideoUserState :one
SELECT
    user_id,
    video_id,
    has_liked,
    has_bookmarked,
    liked_occurred_at,
    bookmarked_occurred_at,
    updated_at
FROM catalog.video_user_engagements_projection
WHERE user_id = $1
  AND video_id = $2;
