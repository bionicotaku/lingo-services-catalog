CREATE TABLE catalog.video_projection (
  video_id        uuid PRIMARY KEY,
  title           text NOT NULL,
  status          catalog.video_status NOT NULL,
  media_status    catalog.stage_status NOT NULL,
  analysis_status catalog.stage_status NOT NULL,
  created_at      timestamptz NOT NULL,
  updated_at      timestamptz NOT NULL,
  version         bigint NOT NULL,
  occurred_at     timestamptz NOT NULL
);

COMMENT ON TABLE catalog.video_projection IS '只读投影：事件驱动维护的视频副本';
