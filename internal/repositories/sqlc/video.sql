-- Video 业务相关的 SQL 查询定义
-- 由 sqlc 生成类型安全的 Go 代码

-- name: FindVideoByID :one
-- 根据 video_id 查询视频详情
SELECT
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
    error_message
FROM catalog.videos
WHERE video_id = $1;
