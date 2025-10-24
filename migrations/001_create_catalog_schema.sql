-- ============================================
-- 0) 扩展与命名空间
-- ============================================
create extension if not exists pgcrypto;               -- 提供 gen_random_uuid()
create schema if not exists catalog;
comment on schema catalog is '领域：视频目录/元数据（videos 等表）';

-- ============================================
-- 1) 枚举类型（存在性检测后创建）
-- ============================================
do $$
begin
  if not exists (
    select 1
      from pg_type t
      join pg_namespace n on n.oid = t.typnamespace
     where n.nspname = 'catalog' and t.typname = 'video_status'
  ) then
    create type catalog.video_status as enum (
      'pending_upload',  -- 记录已创建但上传未完成
      'processing',      -- 媒体或分析阶段仍在进行
      'ready',           -- 媒体与分析阶段均完成
      'published',       -- 已上架对外可见
      'failed',          -- 任一阶段失败
      'rejected',        -- 审核拒绝或强制下架
      'archived'         -- 主动归档或长期下架
    );
  end if;

  if not exists (
    select 1
      from pg_type t
      join pg_namespace n on n.oid = t.typnamespace
     where n.nspname = 'catalog' and t.typname = 'stage_status'
  ) then
    create type catalog.stage_status as enum (
      'pending',         -- 尚未开始该阶段
      'processing',      -- 阶段执行中
      'ready',           -- 阶段完成
      'failed'           -- 阶段失败
    );
  end if;
end$$;

comment on type catalog.video_status is '视频总体生命周期状态：pending_upload/processing/ready/published/failed/rejected/archived';
comment on type catalog.stage_status is '分阶段执行状态：pending/processing/ready/failed';

-- ============================================
-- 2) 主表：videos（含“留空自动生成/显式传入”两用主键）
-- ============================================
create table if not exists catalog.videos (
  video_id             uuid primary key default gen_random_uuid(),         -- 支持留空自动生成或显式传入
  upload_user_id       uuid not null,                                      -- 上传者（auth.users.id）
  created_at           timestamptz not null default now(),                 -- 默认 UTC
  updated_at           timestamptz not null default now(),                 -- 由触发器更新

  title                text not null,                                      -- 标题
  description          text,                                               -- 描述
  raw_file_reference   text not null,                                      -- 原始对象位置/键（如 GCS 路径 + 扩展名）
  status               catalog.video_status not null default 'pending_upload', -- 总体状态
  media_status         catalog.stage_status  not null default 'pending',   -- 媒体阶段
  analysis_status      catalog.stage_status  not null default 'pending',   -- AI 阶段

  -- 上传完成后补写的原始媒体属性
  raw_file_size        bigint check (raw_file_size > 0),                   -- 字节
  raw_resolution       text,                                               -- 如 3840x2160
  raw_bitrate          integer,                                            -- kbps

  -- 媒体转码完成后补写
  duration_micros      bigint,                                             -- 微秒
  encoded_resolution   text,
  encoded_bitrate      integer,
  thumbnail_url        text,
  hls_master_playlist  text,

  -- AI 分析完成后补写
  difficulty           text,
  summary              text,
  tags                 text[],                                             -- 标签数组（配 GIN 索引）

  raw_subtitle_url     text,                                               -- 原始字幕/ASR 输出
  error_message        text                                                -- 最近失败/拒绝原因
);

comment on table catalog.videos is '视频主表：记录上传者、状态流转、媒体与AI分析产物等';

-- 字段注释（逐列）
comment on column catalog.videos.video_id            is '主键：UUID（默认 gen_random_uuid()）。可显式传入自生成 UUID 覆盖默认';
comment on column catalog.videos.upload_user_id      is '上传者用户ID（auth.users.id），受 RLS 策略约束';
comment on column catalog.videos.created_at          is '记录创建时间（timestamptz, 默认 now()）';
comment on column catalog.videos.updated_at          is '最近更新时间（timestamptz），由触发器在 UPDATE 时写入 now()';

comment on column catalog.videos.title               is '视频标题（必填）';
comment on column catalog.videos.description         is '视频描述（可选，长文本）';
comment on column catalog.videos.raw_file_reference  is '原始对象位置（如 gs://bucket/path/file.mp4）';
comment on column catalog.videos.status              is '总体状态：pending_upload→processing→ready/published 或 failed/rejected/archived';
comment on column catalog.videos.media_status        is '媒体阶段状态：pending/processing/ready/failed（转码/封面等）';
comment on column catalog.videos.analysis_status     is 'AI 阶段状态：pending/processing/ready/failed（ASR/标签/摘要等）';

comment on column catalog.videos.raw_file_size       is '原始文件大小（字节，>0）';
comment on column catalog.videos.raw_resolution      is '原始分辨率（如 3840x2160）';
comment on column catalog.videos.raw_bitrate         is '原始码率（kbps）';

comment on column catalog.videos.duration_micros     is '视频时长（微秒）';
comment on column catalog.videos.encoded_resolution  is '主转码分辨率（如 1920x1080）';
comment on column catalog.videos.encoded_bitrate     is '主转码码率（kbps）';
comment on column catalog.videos.thumbnail_url       is '主缩略图 URL/路径';
comment on column catalog.videos.hls_master_playlist is 'HLS 主清单（master.m3u8）URL/路径';

comment on column catalog.videos.difficulty          is 'AI 评估难度（自由文本，可后续枚举化）';
comment on column catalog.videos.summary             is 'AI 生成摘要';
comment on column catalog.videos.tags                is 'AI 生成标签（text[]，使用 GIN 索引提升包含查询）';

comment on column catalog.videos.raw_subtitle_url    is '原始字幕/ASR 输出 URL/路径';
comment on column catalog.videos.error_message       is '最近一次失败/拒绝原因（排障/审计）';

-- ============================================
-- 3) 外键（引用 Supabase Auth 用户，禁止级联删除）
-- ============================================
do $$
begin
  if not exists (
    select 1
      from pg_constraint
     where conname = 'videos_upload_user_fkey'
       and conrelid = 'catalog.videos'::regclass
  ) then
    alter table catalog.videos
      add constraint videos_upload_user_fkey
      foreign key (upload_user_id)
      references auth.users(id)
      on update cascade
      on delete restrict;
  end if;
end$$;

comment on constraint videos_upload_user_fkey on catalog.videos
  is '外键：绑定到 auth.users(id)；更新级联，删除限制（不随用户删除而删除视频）';

-- ============================================
-- 4) 索引（含显式 schema 前缀的注释，避免 42P01）
-- ============================================
create index if not exists videos_status_idx
  on catalog.videos (status);
comment on index catalog.videos_status_idx            is '按总体状态过滤（队列/面板）';

create index if not exists videos_media_status_idx
  on catalog.videos (media_status);
comment on index catalog.videos_media_status_idx      is '按媒体阶段状态过滤（监控转码队列）';

create index if not exists videos_analysis_status_idx
  on catalog.videos (analysis_status);
comment on index catalog.videos_analysis_status_idx   is '按分析阶段状态过滤（监控AI队列）';

create index if not exists videos_tags_gin_idx
  on catalog.videos using gin (tags);
comment on index catalog.videos_tags_gin_idx          is '标签数组的 GIN 索引，支持多标签检索';

create index if not exists videos_upload_user_idx
  on catalog.videos (upload_user_id);
comment on index catalog.videos_upload_user_idx       is '按上传者查找其视频列表';

create index if not exists videos_created_at_idx
  on catalog.videos (created_at);
comment on index catalog.videos_created_at_idx        is '按创建时间排序/分页（Feed/归档）';

-- ============================================
-- 5) 更新时间戳触发器（自动维护 updated_at = now()）
-- ============================================
create or replace function catalog.tg_set_updated_at()
returns trigger
language plpgsql
as $$
begin
  new.updated_at := now();
  return new;
end;
$$;
comment on function catalog.tg_set_updated_at() is '触发器函数：在 UPDATE 时把 updated_at 写为 now()';

do $$
begin
  if not exists (
    select 1 from pg_trigger where tgname = 'set_updated_at_on_videos'
  ) then
    create trigger set_updated_at_on_videos
      before update on catalog.videos
      for each row execute function catalog.tg_set_updated_at();
  end if;
end$$;
comment on trigger set_updated_at_on_videos on catalog.videos
  is '更新 catalog.videos 任意列时自动刷新 updated_at';

-- ============================================
-- 6) Outbox 表：catalog.outbox_events（用于事件发布的 Outbox 模式）
-- ============================================
create table if not exists catalog.outbox_events (
  event_id            uuid primary key default gen_random_uuid(),  -- 事件唯一标识
  aggregate_type      text not null,                               -- 聚合根类型，如 video
  aggregate_id        uuid not null,                               -- 聚合根主键（通常对应业务表主键）
  event_type          text not null,                               -- 领域事件名，如 catalog.video.ready
  payload             jsonb not null,                              -- 事件负载
  headers             jsonb not null default '{}'::jsonb,          -- 追踪/幂等等头信息
  occurred_at         timestamptz not null default now(),          -- 事件产生时间
  available_at        timestamptz not null default now(),          -- 可发布时间（延迟投递时使用）
  published_at        timestamptz,                                 -- 发布成功时间
  delivery_attempts   integer not null default 0 check (delivery_attempts >= 0), -- 投递尝试次数
  last_error          text                                         -- 最近一次失败原因
);

comment on table catalog.outbox_events is 'Outbox 表：与业务事务同库写入，后台扫描发布到事件总线';
comment on column catalog.outbox_events.aggregate_type    is '聚合根类型，限定于 catalog 服务内的实体（如 video）';
comment on column catalog.outbox_events.aggregate_id      is '聚合根主键，保持与业务表一致的 UUID';
comment on column catalog.outbox_events.event_type        is '事件名，使用过去式（如 catalog.video.ready）';
comment on column catalog.outbox_events.payload           is '事件负载（JSON），包含业务数据快照';
comment on column catalog.outbox_events.headers           is '事件头部（JSON），用于 trace/idempotency 等';
comment on column catalog.outbox_events.available_at      is '事件可被 Relay 选择的时间，支持延迟投递';
comment on column catalog.outbox_events.published_at      is '事件成功发布到消息通道的时间戳';
comment on column catalog.outbox_events.delivery_attempts is 'Outbox Relay 重试次数的累积值';
comment on column catalog.outbox_events.last_error        is '最近一次投递失败/异常的描述';

create index if not exists outbox_events_available_idx
  on catalog.outbox_events (available_at)
  where published_at is null;
comment on index catalog.outbox_events_available_idx is '扫描未发布事件时按 available_at 排序';

create index if not exists outbox_events_published_idx
  on catalog.outbox_events (published_at);
comment on index catalog.outbox_events_published_idx is '按发布状态过滤或审计事件';

-- ============================================
-- 7) Inbox 表：catalog.inbox_events（用于消费者幂等处理）
-- ============================================
create table if not exists catalog.inbox_events (
  event_id         uuid primary key,                     -- 来源事件唯一标识
  source_service   text not null,                        -- 事件来源服务，例如 media
  event_type       text not null,                        -- 事件名
  aggregate_type   text,                                 -- 来源聚合根类型
  aggregate_id     text,                                 -- 来源聚合根主键（文本以兼容多种类型）
  payload          jsonb not null default '{}'::jsonb,   -- 原始事件载荷快照
  received_at      timestamptz not null default now(),   -- 收到事件时间
  processed_at     timestamptz,                          -- 本服务处理完成时间
  last_error       text                                  -- 最近一次处理失败信息
);

comment on table catalog.inbox_events is 'Inbox 表：记录已消费的外部事件，保障处理幂等性';
comment on column catalog.inbox_events.event_id       is '来源事件的唯一标识，保证消费幂等';
comment on column catalog.inbox_events.source_service is '事件产生的服务上下文';
comment on column catalog.inbox_events.aggregate_type is '来源聚合根类型（可选，便于排查）';
comment on column catalog.inbox_events.aggregate_id   is '来源聚合根标识（文本化，兼容多类型主键）';
comment on column catalog.inbox_events.processed_at   is '事件处理成功的时间戳，NULL 表示仍待处理';

create index if not exists inbox_events_processed_idx
  on catalog.inbox_events (processed_at);
comment on index catalog.inbox_events_processed_idx is '按处理状态/时间过滤 Inbox 记录';

-- ============================================
-- 8) 测试只读视图：catalog.videos_ready_view
-- ============================================
create or replace view catalog.videos_ready_view as
select
  v.video_id,
  v.title,
  v.status,
  v.media_status,
  v.analysis_status,
  v.created_at,
  v.updated_at
from catalog.videos v
where v.status in ('ready', 'published');

comment on view catalog.videos_ready_view
  is '测试用只读视图：展示状态为 ready/published 的视频基础信息';
