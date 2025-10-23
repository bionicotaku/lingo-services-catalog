-- -----------------------------------------------------------------------------
-- Migration: 001 - Create Catalog Schema & Videos Table
-- Description: 创建 catalog schema、枚举类型、videos 主表及相关索引/触发器
-- Author: Catalog Service Team
-- Date: 2025-01-22
--
-- 依赖项：
--   - PostgreSQL 扩展：pgcrypto（UUID 生成）
--   - Supabase Auth: auth.users 表必须存在
--
-- 幂等性：所有语句使用 IF NOT EXISTS/IF EXISTS 检查，支持重复执行
-- -----------------------------------------------------------------------------

-- ============================================
-- 0) 扩展与命名空间
-- ============================================
-- pgcrypto: 提供 gen_random_uuid() 函数，生成 UUID v4
-- 用途：video_id 主键的默认值
create extension if not exists pgcrypto;               -- 提供 gen_random_uuid()

-- catalog schema: 视频目录服务的独立命名空间
-- 设计原则：微服务数据主权，每个服务独占 schema，禁止跨服务表访问
create schema if not exists catalog;
comment on schema catalog is '领域：视频目录/元数据（videos 等表）';

-- ============================================
-- 1) 枚举类型定义
-- ============================================
-- 设计原则：
--   - 枚举类型保证数据完整性，避免魔法字符串
--   - PostgreSQL 枚举限制：不可删除值，只能添加（ALTER TYPE ADD VALUE）
--   - 状态机流转需在应用层实现校验

-- video_status: 视频总体生命周期状态
-- 正常流程：pending_upload → processing → ready → published
-- 失败分支：任意阶段 → failed
-- 审核拒绝：ready/processing → rejected
-- 归档：published → archived
create type catalog.video_status as enum (
  'pending_upload',  -- 记录已创建但上传未完成（初始状态）
  'processing',      -- 媒体或分析阶段仍在进行（至少一个阶段 status=processing）
  'ready',           -- 媒体与分析阶段均完成（可发布状态）
  'published',       -- 已上架对外可见（需满足可见性策略）
  'failed',          -- 任一阶段失败（需人工介入或重试）
  'rejected',        -- 审核拒绝或强制下架（版权/安全问题）
  'archived'         -- 主动归档或长期下架（软删除替代）
);

-- stage_status: 分阶段处理状态（媒体/AI 独立追踪）
-- 用途：细粒度监控转码队列和 AI 分析队列
create type catalog.stage_status as enum (
  'pending',         -- 尚未开始该阶段（初始状态）
  'processing',      -- 阶段执行中（已提交任务，等待完成）
  'ready',           -- 阶段完成（产物已生成）
  'failed'           -- 阶段失败（需重试或跳过）
);

comment on type catalog.video_status is '视频总体生命周期状态：pending_upload/processing/ready/published/failed/rejected/archived';
comment on type catalog.stage_status is '分阶段执行状态：pending/processing/ready/failed';

-- ============================================
-- 2) 主表：videos（含"留空自动生成/显式传入"两用主键）
-- ============================================
-- 表设计原则：
--   1. 单一写入者：仅 Catalog 服务可写，其他服务通过 gRPC 调用
--   2. 分层字段：基础层（RegisterUpload）→ 原始媒体层（UpdateOriginalMedia）
--                → 转码层（UpdateMediaInfo）→ AI层（UpdateAIAttributes）
--   3. 事件驱动：所有变更触发 Outbox 事件（后续 MVP 实现）
--   4. 可演进性：新字段用 NULL，破坏性变更需迁移脚本
create table if not exists catalog.videos (
  -- 核心标识：UUID v4 主键
  -- 支持两种使用模式：
  --   1. 应用层生成 UUID 并显式传入（推荐，便于幂等性实现）
  --   2. 数据库自动生成（留空时触发 gen_random_uuid()）
  video_id             uuid primary key default gen_random_uuid(),         -- 支持留空自动生成或显式传入

  -- 外键：关联 Supabase Auth 用户表
  -- 约束：ON DELETE RESTRICT 防止级联删除导致孤儿视频
  upload_user_id       uuid not null,                                      -- 上传者（auth.users.id）

  -- 时间戳：UTC 时区，自动维护
  created_at           timestamptz not null default now(),                 -- 默认 UTC
  updated_at           timestamptz not null default now(),                 -- 由触发器更新

  -- ========================================
  -- 基础层字段（RegisterUpload 用例写入）
  -- ========================================
  title                text not null,                                      -- 标题
  description          text,                                               -- 描述（可选，支持 Markdown）

  -- 原始文件引用：GCS 对象路径
  -- 格式要求：必须含扩展名（.mp4/.mov等），用于推断编解码器
  -- 安全要求：GCS IAM 策略应绑定 video_id，防止未注册上传
  raw_file_reference   text not null,                                      -- 原始对象位置/键（如 GCS 路径 + 扩展名）

  -- 状态字段：枚举类型映射
  status               catalog.video_status not null default 'pending_upload', -- 总体状态
  media_status         catalog.stage_status  not null default 'pending',   -- 媒体阶段
  analysis_status      catalog.stage_status  not null default 'pending',   -- AI 阶段

  -- ========================================
  -- 原始媒体属性（UpdateOriginalMedia 用例补写）
  -- ========================================
  -- 时机：客户端完成直传后，Upload 服务回调写入
  -- 用途：配额计费、转码策略选择
  raw_file_size        bigint check (raw_file_size > 0),                   -- 字节（CHECK 约束防止负数）
  raw_resolution       text,                                               -- 如 3840x2160（宽x高）
  raw_bitrate          integer,                                            -- kbps（影响转码层级选择）

  -- ========================================
  -- 媒体转码产物（UpdateMediaInfo 用例补写）
  -- ========================================
  -- 时机：Media 服务转码完成后写入
  duration_micros      bigint,                                             -- 微秒（高精度，避免秒级累计误差）
  encoded_resolution   text,                                               -- 主转码分辨率（通常 1080p 或 720p）
  encoded_bitrate      integer,                                            -- 主转码码率（kbps）

  -- 缩略图：GCS 路径，支持多张（后续可改为 text[] 数组）
  thumbnail_url        text,

  -- HLS 主清单：master.m3u8 路径
  -- 目录结构：同目录下含子清单（720p.m3u8, 1080p.m3u8）和 TS 分片
  hls_master_playlist  text,

  -- ========================================
  -- AI 分析产物（UpdateAIAttributes 用例补写）
  -- ========================================
  -- 时机：AI 服务分析完成后写入
  -- 难度评估：自由文本（后续可改为枚举或数值 0-100）
  difficulty           text,

  -- AI 摘要：1-3 句话，用于搜索与推荐卡片
  summary              text,

  -- 标签数组：PostgreSQL text[] 类型
  -- 查询示例：WHERE tags @> ARRAY['grammar']::text[]
  -- 索引：GIN 索引支持高效数组包含查询
  tags                 text[],                                             -- 标签数组（配 GIN 索引）

  -- 原始字幕/ASR 输出：SRT 格式
  -- 用途：后续语义切片、全文检索
  raw_subtitle_url     text,                                               -- 原始字幕/ASR 输出

  -- 错误信息：最近一次失败原因
  -- 示例："transcode_failed: unsupported codec (h265)"
  --       "analysis_failed: audio track missing"
  error_message        text                                                -- 最近失败/拒绝原因
);

comment on table catalog.videos is '视频主表：记录上传者、状态流转、媒体与AI分析产物等';

-- ========================================
-- 字段级详细注释
-- ========================================
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
-- 幂等性处理：先检查约束是否存在，避免重复执行报错
-- 策略：ON DELETE RESTRICT 防止误删用户导致孤儿视频
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
      on update cascade      -- 用户ID更新时级联更新
      on delete restrict;    -- 禁止删除有视频的用户
  end if;
end$$;

comment on constraint videos_upload_user_fkey on catalog.videos
  is '外键：绑定到 auth.users(id)；更新级联，删除限制（不随用户删除而删除视频）';

-- ============================================
-- 4) 索引（含显式 schema 前缀的注释，避免 42P01）
-- ============================================
-- 索引设计原则：
--   1. 覆盖高频查询（状态过滤、用户列表、时间排序）
--   2. GIN 索引支持数组包含查询
--   3. 避免过度索引（影响写入性能）

-- 状态索引：用于监控队列统计（processing/failed 数量）
create index if not exists videos_status_idx
  on catalog.videos (status);
comment on index catalog.videos_status_idx            is '按总体状态过滤（队列/面板）';

create index if not exists videos_media_status_idx
  on catalog.videos (media_status);
comment on index catalog.videos_media_status_idx      is '按媒体阶段状态过滤（监控转码队列）';

create index if not exists videos_analysis_status_idx
  on catalog.videos (analysis_status);
comment on index catalog.videos_analysis_status_idx   is '按分析阶段状态过滤（监控AI队列）';

-- GIN 索引：支持标签数组包含查询
-- 查询示例：WHERE tags @> ARRAY['grammar']::text[]
-- 性能：O(log n) 查找，适合多标签过滤
create index if not exists videos_tags_gin_idx
  on catalog.videos using gin (tags);
comment on index catalog.videos_tags_gin_idx          is '标签数组的 GIN 索引，支持多标签检索';

-- 用户索引：支持 ListMyUploads 用例（按用户查全部视频）
create index if not exists videos_upload_user_idx
  on catalog.videos (upload_user_id);
comment on index catalog.videos_upload_user_idx       is '按上传者查找其视频列表';

-- 时间索引：支持 Feed 流分页（按创建时间倒序）
create index if not exists videos_created_at_idx
  on catalog.videos (created_at);
comment on index catalog.videos_created_at_idx        is '按创建时间排序/分页（Feed/归档）';

-- ============================================
-- 5) 触发器：自动维护 updated_at
-- ============================================
-- 用途：
--   1. 自动更新 updated_at 字段，应用层无需手动设置
--   2. 用于 ETag 生成（If-None-Match 并发控制）
--   3. 用于缓存失效判断

-- 触发器函数：设置 updated_at 为当前时间
create or replace function catalog.tg_set_updated_at()
returns trigger
language plpgsql
as $$
begin
  new.updated_at := now();  -- 每次 UPDATE 时写入当前 UTC 时间
  return new;
end;
$$;
comment on function catalog.tg_set_updated_at() is '触发器函数：在 UPDATE 时把 updated_at 写为 now()';

-- 先删除旧触发器（幂等性）
drop trigger if exists set_updated_at_on_videos on catalog.videos;

-- 创建触发器：BEFORE UPDATE 执行
create trigger set_updated_at_on_videos
  before update on catalog.videos
  for each row execute function catalog.tg_set_updated_at();
comment on trigger set_updated_at_on_videos on catalog.videos
  is '更新 catalog.videos 任意列时自动刷新 updated_at';

-- ============================================
-- Migration 完成
-- ============================================
-- 验证步骤：
--   1. \d catalog.videos     -- 查看表结构
--   2. \di catalog.*         -- 查看索引
--   3. \df catalog.*         -- 查看函数/触发器
--
-- 后续步骤：
--   1. 运行 sqlc generate 生成 DAO 层代码
--   2. 实现 Repository 层（基于 sqlc 生成的代码）
--   3. 编写集成测试（使用 Testcontainers 或 Supabase 测试库）
-- ============================================
