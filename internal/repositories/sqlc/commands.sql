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

-- name: UpdateVideo :one
UPDATE catalog.videos
SET
    title = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    status = COALESCE(sqlc.narg('status')::catalog.video_status, status),
    media_status = COALESCE(sqlc.narg('media_status')::catalog.stage_status, media_status),
    analysis_status = COALESCE(sqlc.narg('analysis_status')::catalog.stage_status, analysis_status),
    duration_micros = COALESCE(sqlc.narg('duration_micros'), duration_micros),
    thumbnail_url = COALESCE(sqlc.narg('thumbnail_url'), thumbnail_url),
    hls_master_playlist = COALESCE(sqlc.narg('hls_master_playlist'), hls_master_playlist),
    difficulty = COALESCE(sqlc.narg('difficulty'), difficulty),
    summary = COALESCE(sqlc.narg('summary'), summary),
    raw_subtitle_url = COALESCE(sqlc.narg('raw_subtitle_url'), raw_subtitle_url),
    error_message = COALESCE(sqlc.narg('error_message'), error_message)
WHERE video_id = sqlc.arg('video_id')
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

-- name: DeleteVideo :one
DELETE FROM catalog.videos
WHERE video_id = $1
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
