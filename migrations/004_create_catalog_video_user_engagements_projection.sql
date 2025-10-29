create table if not exists catalog.video_user_engagements_projection (
  user_id         uuid not null,
  video_id        uuid not null,
  has_liked       boolean not null default false,
  has_bookmarked  boolean not null default false,
  liked_occurred_at      timestamptz,
  bookmarked_occurred_at timestamptz,
  updated_at      timestamptz not null default now(),
  primary key (user_id, video_id)
);

comment on table catalog.video_user_engagements_projection
  is '用户对视频的互动状态：由 Engagement 投影消费者维护的 liked/bookmarked 标记';
comment on column catalog.video_user_engagements_projection.user_id        is '用户主键';
comment on column catalog.video_user_engagements_projection.video_id       is '视频主键';
comment on column catalog.video_user_engagements_projection.has_liked      is '是否点赞';
comment on column catalog.video_user_engagements_projection.has_bookmarked is '是否收藏';
comment on column catalog.video_user_engagements_projection.liked_occurred_at is '最近一次点赞事件发生时间';
comment on column catalog.video_user_engagements_projection.bookmarked_occurred_at is '最近一次收藏事件发生时间';
comment on column catalog.video_user_engagements_projection.updated_at     is '该状态最后一次更新的时间';

create index if not exists video_user_engagements_projection_video_idx
  on catalog.video_user_engagements_projection (video_id);
