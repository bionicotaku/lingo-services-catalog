-- Video 业务相关的 SQL 查询定义
-- 由 sqlc 生成类型安全的 Go 代码

-- name: CreateVideo :one
-- 创建新视频记录，video_id 由数据库自动生成
INSERT INTO catalog.videos (
    upload_user_id,
    created_at,
    updated_at,
    title,
    description,
    raw_file_reference,
    status,
    media_status,
    analysis_status
) VALUES (
    $1,
    now(),
    now(),
    $2,
    sqlc.narg('description'),
    $3,
    'pending_upload',
    'pending',
    'pending'
)
RETURNING
    video_id,
    upload_user_id,
    created_at,
    updated_at,
    title,
    description,
    raw_file_reference,
    status,
    media_status,
    analysis_status,
    raw_file_size,
    raw_resolution,
    raw_bitrate,
    duration_micros,
    encoded_resolution,
    encoded_bitrate,
    thumbnail_url,
    hls_master_playlist,
    difficulty,
    summary,
    tags,
    raw_subtitle_url,
    error_message;

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
