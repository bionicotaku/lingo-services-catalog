-- 统计投影读取
-- name: GetVideoEngagementStats :one
SELECT
    video_id,
    like_count,
    bookmark_count,
    watch_count,
    unique_watchers,
    first_watch_at,
    last_watch_at,
    updated_at
FROM catalog.video_engagement_stats_projection
WHERE video_id = $1;

-- 增量更新统计计数
-- name: IncrementVideoEngagementStats :one
INSERT INTO catalog.video_engagement_stats_projection (
    video_id,
    like_count,
    bookmark_count,
    watch_count,
    unique_watchers,
    first_watch_at,
    last_watch_at,
    updated_at
) VALUES (
    sqlc.arg('video_id'),
    GREATEST(sqlc.arg('like_delta')::bigint, 0),
    GREATEST(sqlc.arg('bookmark_delta')::bigint, 0),
    GREATEST(sqlc.arg('watch_delta')::bigint, 0),
    GREATEST(sqlc.arg('unique_watcher_delta')::bigint, 0),
    sqlc.narg('first_watch_at'),
    sqlc.narg('last_watch_at'),
    now()
)
ON CONFLICT (video_id) DO UPDATE
SET
    like_count = GREATEST(0, catalog.video_engagement_stats_projection.like_count + sqlc.arg('like_delta')::bigint),
    bookmark_count = GREATEST(0, catalog.video_engagement_stats_projection.bookmark_count + sqlc.arg('bookmark_delta')::bigint),
    watch_count = GREATEST(0, catalog.video_engagement_stats_projection.watch_count + sqlc.arg('watch_delta')::bigint),
    unique_watchers = GREATEST(0, catalog.video_engagement_stats_projection.unique_watchers + sqlc.arg('unique_watcher_delta')::bigint),
    first_watch_at = CASE
        WHEN sqlc.narg('first_watch_at') IS NULL THEN catalog.video_engagement_stats_projection.first_watch_at
        WHEN catalog.video_engagement_stats_projection.first_watch_at IS NULL THEN sqlc.narg('first_watch_at')
        ELSE LEAST(catalog.video_engagement_stats_projection.first_watch_at, sqlc.narg('first_watch_at'))
    END,
    last_watch_at = CASE
        WHEN sqlc.narg('last_watch_at') IS NULL THEN catalog.video_engagement_stats_projection.last_watch_at
        WHEN catalog.video_engagement_stats_projection.last_watch_at IS NULL THEN sqlc.narg('last_watch_at')
        ELSE GREATEST(catalog.video_engagement_stats_projection.last_watch_at, sqlc.narg('last_watch_at'))
    END,
    updated_at = now()
RETURNING
    video_id,
    like_count,
    bookmark_count,
    watch_count,
    unique_watchers,
    first_watch_at,
    last_watch_at,
    updated_at;

-- 记录唯一观看者
-- name: UpsertVideoWatcher :one
INSERT INTO catalog.video_engagement_watchers (
    video_id,
    user_id,
    first_watched_at,
    last_watched_at
) VALUES (
    sqlc.arg('video_id'),
    sqlc.arg('user_id'),
    sqlc.arg('watch_time'),
    sqlc.arg('watch_time')
)
ON CONFLICT (video_id, user_id) DO UPDATE
SET last_watched_at = GREATEST(catalog.video_engagement_watchers.last_watched_at, EXCLUDED.last_watched_at)
RETURNING
    video_id,
    user_id,
    first_watched_at,
    last_watched_at,
    (xmax = 0) AS inserted;
