CREATE TABLE catalog.video_user_engagements_projection (
    user_id        uuid    NOT NULL,
    video_id       uuid    NOT NULL,
    has_liked      boolean NOT NULL DEFAULT false,
    has_bookmarked boolean NOT NULL DEFAULT false,
    liked_occurred_at      timestamptz,
    bookmarked_occurred_at timestamptz,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, video_id)
);

CREATE INDEX video_user_engagements_projection_video_idx
    ON catalog.video_user_engagements_projection (video_id);
