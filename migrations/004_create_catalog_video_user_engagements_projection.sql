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

-- catalog.video_engagement_stats_projection 记录来自 Profile 事件的聚合统计。
create table if not exists catalog.video_engagement_stats_projection (
  video_id         uuid primary key,
  like_count       bigint not null default 0 check (like_count >= 0),
  bookmark_count   bigint not null default 0 check (bookmark_count >= 0),
  watch_count      bigint not null default 0 check (watch_count >= 0),
  unique_watchers  bigint not null default 0 check (unique_watchers >= 0),
  first_watch_at   timestamptz,
  last_watch_at    timestamptz,
  updated_at       timestamptz not null default now()
);

comment on table catalog.video_engagement_stats_projection is 'Catalog 服务的 Profile 投影：每个视频的点赞/收藏/观看聚合指标';
comment on column catalog.video_engagement_stats_projection.video_id        is '视频主键（catalog.videos.video_id）';
comment on column catalog.video_engagement_stats_projection.like_count      is '点赞次数（profile.engagement.added/removed, type=like）';
comment on column catalog.video_engagement_stats_projection.bookmark_count  is '收藏次数（profile.engagement.added/removed, type=bookmark）';
comment on column catalog.video_engagement_stats_projection.watch_count     is '有效观看事件次数（profile.watch.progressed）';
comment on column catalog.video_engagement_stats_projection.unique_watchers is '累计观看的唯一用户数';
comment on column catalog.video_engagement_stats_projection.first_watch_at  is '首次观看发生时间';
comment on column catalog.video_engagement_stats_projection.last_watch_at   is '最近一次观看发生时间';
comment on column catalog.video_engagement_stats_projection.updated_at      is '最后一次聚合更新时间';

create index if not exists video_engagement_stats_projection_updated_idx
  on catalog.video_engagement_stats_projection (updated_at desc);

-- 记录已经计入 unique_watchers 的用户集合，避免重复计数。
create table if not exists catalog.video_engagement_watchers (
  video_id         uuid not null,
  user_id          uuid not null,
  first_watched_at timestamptz not null,
  last_watched_at  timestamptz not null,
  primary key (video_id, user_id)
);

comment on table catalog.video_engagement_watchers is 'Catalog 服务内部去重表：记录已统计过唯一观看用户';
comment on column catalog.video_engagement_watchers.first_watched_at is '该用户首次观看时间';
comment on column catalog.video_engagement_watchers.last_watched_at  is '该用户最近观看时间';

create index if not exists video_engagement_watchers_last_watch_idx
  on catalog.video_engagement_watchers (last_watched_at desc);
