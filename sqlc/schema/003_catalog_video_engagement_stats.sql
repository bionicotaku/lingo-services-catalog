CREATE TABLE catalog.video_engagement_stats_projection (
  video_id         uuid PRIMARY KEY,
  like_count       bigint NOT NULL DEFAULT 0 CHECK (like_count >= 0),
  bookmark_count   bigint NOT NULL DEFAULT 0 CHECK (bookmark_count >= 0),
  watch_count      bigint NOT NULL DEFAULT 0 CHECK (watch_count >= 0),
  unique_watchers  bigint NOT NULL DEFAULT 0 CHECK (unique_watchers >= 0),
  first_watch_at   timestamptz,
  last_watch_at    timestamptz,
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE catalog.video_engagement_watchers (
  video_id         uuid NOT NULL,
  user_id          uuid NOT NULL,
  first_watched_at timestamptz NOT NULL,
  last_watched_at  timestamptz NOT NULL,
  PRIMARY KEY (video_id, user_id)
);
