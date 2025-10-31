-- name: UpsertUpload :one
WITH upsert AS (
  INSERT INTO catalog.uploads AS u (
    video_id,
    user_id,
    bucket,
    object_name,
    content_type,
    expected_size,
    content_md5,
    title,
    description,
    signed_url,
    signed_url_expires_at,
    status
  )
  VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12
  )
  ON CONFLICT (user_id, content_md5)
  DO UPDATE
  SET bucket = EXCLUDED.bucket,
      object_name = EXCLUDED.object_name,
      content_type = EXCLUDED.content_type,
      expected_size = EXCLUDED.expected_size,
      title = EXCLUDED.title,
      description = EXCLUDED.description,
      signed_url = EXCLUDED.signed_url,
      signed_url_expires_at = EXCLUDED.signed_url_expires_at,
      status = CASE
        WHEN catalog.uploads.status = 'completed' THEN catalog.uploads.status
        ELSE EXCLUDED.status
      END,
      updated_at = now()
  RETURNING u.video_id,
            u.user_id,
            u.bucket,
            u.object_name,
            u.content_type,
            u.expected_size,
            u.size_bytes,
            u.content_md5,
            u.title,
            u.description,
            u.signed_url,
            u.signed_url_expires_at,
            u.status,
            u.gcs_generation,
            u.gcs_etag,
            u.md5_hash,
            u.crc32c,
            u.error_code,
            u.error_message,
            u.created_at,
            u.updated_at,
            (xmax = 0)::bool AS inserted
)
SELECT * FROM upsert;

-- name: GetUploadByVideoID :one
SELECT *
FROM catalog.uploads
WHERE video_id = $1
LIMIT 1;

-- name: GetUploadByObject :one
SELECT *
FROM catalog.uploads
WHERE bucket = $1
  AND object_name = $2
LIMIT 1;

-- name: GetUploadByUserMd5 :one
SELECT *
FROM catalog.uploads
WHERE user_id = $1
  AND content_md5 = $2
LIMIT 1;

-- name: MarkUploadCompleted :one
UPDATE catalog.uploads
SET status = 'completed',
    size_bytes = sqlc.arg(size_bytes),
    md5_hash = sqlc.arg(md5_hash),
    crc32c = sqlc.arg(crc32c),
    gcs_generation = sqlc.arg(gcs_generation),
    gcs_etag = sqlc.arg(gcs_etag),
    content_type = COALESCE(sqlc.narg(content_type), content_type),
    signed_url = NULL,
    signed_url_expires_at = NULL,
    error_code = NULL,
    error_message = NULL,
    updated_at = now()
WHERE video_id = sqlc.arg(video_id)
RETURNING *;

-- name: MarkUploadFailed :one
UPDATE catalog.uploads
SET status = 'failed',
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    updated_at = now()
WHERE video_id = sqlc.arg(video_id)
RETURNING *;

-- name: ListExpiredUploads :many
SELECT *
FROM catalog.uploads
WHERE status = 'uploading'
  AND signed_url_expires_at IS NOT NULL
  AND signed_url_expires_at < sqlc.arg('cutoff')
ORDER BY signed_url_expires_at ASC
LIMIT sqlc.arg('limit');
