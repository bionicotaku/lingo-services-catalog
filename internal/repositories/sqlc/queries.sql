-- Video 只读投影查询相关 SQL

-- name: FindVideoByID :one
-- 根据 video_id 从投影表查询视频详情（仅返回 ready/published 状态的视频）
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    created_at,
    updated_at
FROM catalog.video_projection
WHERE video_id = $1
  AND status IN ('ready', 'published');

-- name: ListReadyVideosForTest :many
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    created_at,
    updated_at
FROM catalog.video_projection
WHERE status IN ('ready', 'published')
ORDER BY created_at DESC;
