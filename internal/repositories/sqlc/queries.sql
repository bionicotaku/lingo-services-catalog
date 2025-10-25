-- Video 只读视图查询相关 SQL

-- name: FindVideoByID :one
-- 根据 video_id 从只读视图查询视频详情（仅返回 ready/published 状态的视频）
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    created_at,
    updated_at
FROM catalog.videos_ready_view
WHERE video_id = $1;

-- name: ListReadyVideosForTest :many
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    created_at,
    updated_at
FROM catalog.videos_ready_view
ORDER BY created_at DESC;
