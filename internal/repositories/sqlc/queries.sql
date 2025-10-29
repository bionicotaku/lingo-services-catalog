-- 读取生命周期写流程所需的完整快照，无状态限制
-- name: GetVideoLifecycleSnapshot :one
SELECT
    video_id,
    upload_user_id,
    created_at,
    updated_at,
    title,
    description,
    raw_file_reference,
    status,
    version,
    media_status,
    analysis_status,
    media_job_id,
    media_emitted_at,
    analysis_job_id,
    analysis_emitted_at,
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
    visibility_status,
    publish_at,
    raw_subtitle_url,
    error_message
FROM catalog.videos
WHERE video_id = $1;

-- 读取前台查询可见的视频（仅 ready/published），字段裁剪
-- name: FindPublishedVideo :one
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    visibility_status,
    publish_at,
    created_at,
    updated_at
FROM catalog.videos
WHERE video_id = $1
  AND status IN ('ready', 'published');

-- name: GetVideoMetadata :one
SELECT
    video_id,
    status,
    media_status,
    analysis_status,
    duration_micros,
    encoded_resolution,
    encoded_bitrate,
    thumbnail_url,
    hls_master_playlist,
    difficulty,
    summary,
    tags,
    visibility_status,
    publish_at,
    raw_subtitle_url,
    updated_at,
    version
FROM catalog.videos
WHERE video_id = $1;

-- name: ListPublicVideos :many
SELECT
    video_id,
    title,
    status,
    media_status,
    analysis_status,
    visibility_status,
    publish_at,
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
    visibility_status,
    publish_at,
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
        sqlc.narg('stage_filter') IS NULL
        OR cardinality(sqlc.narg('stage_filter')) = 0
        OR media_status = ANY(sqlc.narg('stage_filter'))
        OR analysis_status = ANY(sqlc.narg('stage_filter'))
      )
  AND (
        sqlc.narg('cursor_created_at') IS NULL
        OR created_at < sqlc.narg('cursor_created_at')
        OR (created_at = sqlc.narg('cursor_created_at') AND video_id < sqlc.narg('cursor_video_id'))
      )
ORDER BY created_at DESC, video_id DESC
LIMIT sqlc.arg('limit');
