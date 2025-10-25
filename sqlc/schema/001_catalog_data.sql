CREATE SCHEMA catalog;

CREATE TYPE catalog.video_status AS ENUM (
  'pending_upload',
  'processing',
  'ready',
  'published',
  'failed',
  'rejected',
  'archived'
);

CREATE TYPE catalog.stage_status AS ENUM (
  'pending',
  'processing',
  'ready',
  'failed'
);

CREATE TABLE catalog.videos (
  video_id UUID PRIMARY KEY,
  upload_user_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  title TEXT NOT NULL,
  description TEXT,
  raw_file_reference TEXT NOT NULL,
  status catalog.video_status NOT NULL,
  media_status catalog.stage_status NOT NULL,
  analysis_status catalog.stage_status NOT NULL,
  raw_file_size BIGINT,
  raw_resolution TEXT,
  raw_bitrate INTEGER,
  duration_micros BIGINT,
  encoded_resolution TEXT,
  encoded_bitrate INTEGER,
  thumbnail_url TEXT,
  hls_master_playlist TEXT,
  difficulty TEXT,
  summary TEXT,
  tags TEXT[],
  raw_subtitle_url TEXT,
  error_message TEXT
);
