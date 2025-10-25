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
