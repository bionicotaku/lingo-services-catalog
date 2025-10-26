create table if not exists catalog.video_user_states (
  user_id         uuid not null,
  video_id        uuid not null,
  has_liked       boolean not null default false,
  has_bookmarked  boolean not null default false,
  has_watched     boolean not null default false,
  occurred_at     timestamptz not null default now(),
  updated_at      timestamptz not null default now(),
  primary key (user_id, video_id)
);

comment on table catalog.video_user_states
  is '用户对视频的互动状态：由 Engagement 投影消费者维护的 liked/bookmarked/watched 标记';
comment on column catalog.video_user_states.user_id        is '用户主键';
comment on column catalog.video_user_states.video_id       is '视频主键';
comment on column catalog.video_user_states.has_liked      is '是否点赞';
comment on column catalog.video_user_states.has_bookmarked is '是否收藏';
comment on column catalog.video_user_states.has_watched    is '是否已观看';
comment on column catalog.video_user_states.occurred_at    is '来源 Engagement 事件的发生时间';
comment on column catalog.video_user_states.updated_at     is '该状态最后一次更新的时间';

create index if not exists video_user_states_video_idx
  on catalog.video_user_states (video_id);
