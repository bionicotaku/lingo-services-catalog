-- Video 主表写入相关 SQL

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
