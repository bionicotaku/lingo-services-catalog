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

CREATE TABLE catalog.outbox_events (
  event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  aggregate_type TEXT NOT NULL,
  aggregate_id UUID NOT NULL,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  headers JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  delivery_attempts INTEGER NOT NULL DEFAULT 0 CHECK (delivery_attempts >= 0),
  last_error TEXT
);

CREATE TABLE catalog.inbox_events (
  event_id UUID PRIMARY KEY,
  source_service TEXT NOT NULL,
  event_type TEXT NOT NULL,
  aggregate_type TEXT,
  aggregate_id TEXT,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at TIMESTAMPTZ,
  last_error TEXT
);

CREATE VIEW catalog.videos_ready_view (
  video_id,
  title,
  status,
  media_status,
  analysis_status,
  created_at,
  updated_at
) AS
SELECT
  video_id,
  title,
  status,
  media_status,
  analysis_status,
  created_at,
  updated_at
FROM catalog.videos
WHERE status IN ('ready', 'published');
