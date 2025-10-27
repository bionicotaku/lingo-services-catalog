CREATE TABLE catalog.video_user_states (
    user_id        uuid    NOT NULL,
    video_id       uuid    NOT NULL,
    has_liked      boolean NOT NULL DEFAULT false,
    has_bookmarked boolean NOT NULL DEFAULT false,
    has_watched    boolean NOT NULL DEFAULT false,
    occurred_at    timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, video_id)
);

CREATE INDEX video_user_states_video_idx
    ON catalog.video_user_states (video_id);
