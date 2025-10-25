create table if not exists catalog.video_projection (
  video_id        uuid primary key,
  title           text not null,
  status          catalog.video_status not null,
  media_status    catalog.stage_status not null,
  analysis_status catalog.stage_status not null,
  created_at      timestamptz not null,
  updated_at      timestamptz not null,
  version         bigint not null,
  occurred_at     timestamptz not null
);

comment on table catalog.video_projection
  is '只读投影表：基于事件驱动维护的 catalog.videos 副本';
comment on column catalog.video_projection.video_id        is '视频主键，对应 catalog.videos.video_id';
comment on column catalog.video_projection.status          is '视频总体状态';
comment on column catalog.video_projection.media_status    is '媒体处理阶段状态';
comment on column catalog.video_projection.analysis_status is '分析阶段状态';
comment on column catalog.video_projection.version         is '事件版本（用于幂等与乱序保护）';
comment on column catalog.video_projection.occurred_at     is '事件发生时间（供滞后分析）';

create index if not exists video_projection_status_idx
  on catalog.video_projection (status);
