CREATE TABLE catalog.uploads (
  video_id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  bucket TEXT NOT NULL,
  object_name TEXT NOT NULL,
  content_type TEXT,
  expected_size BIGINT NOT NULL DEFAULT 0,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  content_md5 CHAR(32) NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  signed_url TEXT,
  signed_url_expires_at TIMESTAMPTZ,
  status TEXT NOT NULL,
  gcs_generation TEXT,
  gcs_etag TEXT,
  md5_hash TEXT,
  crc32c TEXT,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uploads_user_md5_unique ON catalog.uploads (user_id, content_md5);

CREATE UNIQUE INDEX uploads_bucket_object_unique ON catalog.uploads (bucket, object_name);
