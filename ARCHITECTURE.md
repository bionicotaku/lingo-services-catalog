# Catalog Service Detailed Design (v0.1 · 2025-10-19)

> 本文作为 Catalog 模块的工程实现说明，用于指导 MVP → 可演进阶段的开发与评审。覆盖领域职责、数据主权、gRPC 契约、事件、非功能需求与工程落地要求。阅读本档前请先熟悉《1 项目概述》《2 对外 API 概述》《4DDD-lite 架构》《6 语言范式与骨架》。

---

## 1. 使命与边界

- **核心使命**：维护视频的权威元数据与可见性真相，协调上传 → 转码 → AI → 上架的流程，为下游提供一致且可信的读取视图。
- **关键原则**
- **单一写入者**：Catalog 负责 `videos` 表的所有写入；其他服务只能通过 Catalog 端口提交更新。
- **分层字段**：基础层、媒体层、AI 语义层、可见性层分别由各责任方生产，由 Catalog 聚合。
- **事件驱动**：所有变更同步写入 Outbox，发布供 Search / Feed / RecSys 消费。
- **只读投影**：早期方案曾通过独立投影进程维护 `catalog_read` 视图；当前版本取消该进程，Catalog 直接从主表 `catalog.videos` 读取，后续若需要外部只读副本由下游服务自行维护。
- **访问控制**：外部经 Gateway；内部写接口仅接受受信服务身份，并要求幂等。

---

## 2. 领域模型

### 2.1 聚合根 `Video`

| 层级       | 字段                                                                                                                                                                                                                                           | 说明                                                                                            | 来源                     |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- | ------------------------ |
| 基础层     | `video_id`(ULID)、`upload_user_id`、`created_at`、`title`、`description`、`raw_file_reference`、`status`(`pending_upload`→`processing`→`ready`→`published`/`failed`/`rejected`/`archived`)、`media_status`、`analysis_status`、`error_message` | 上传记录、元信息、原始对象存储路径、阶段状态与错误信息；`raw_file_reference` 在注册上传时即写入 | Catalog + Upload         |
| 媒体层     | `duration_micros`、`encoded_resolution`、`encoded_bitrate`、`thumbnail_url`、`hls_master_playlist`                                                                                                                                             | 转码后 HLS 列表（同名文件夹下 `master.m3u8`）、封面与时长（微秒）、目标码率                     | Media Pipeline           |
| AI 层      | `difficulty`、`summary`、`tags`、`raw_subtitle_url`                                                                                                                                                                                            | AI 评估难度、生成摘要、标签以及原始字幕存储位置                                                 | AI Content Understanding |
| 可见性层\* | `visibility_status`(`public`/`unlisted`/`private`)、`region_restrictions[]`、`age_rating`、`publish_at`、`takedown_reason`                                                                                                                   | 上架/区域/版权裁决（Post-MVP）                                                                  | Safety & QA / 运营       |

> \* 标记为 Post-MVP 的字段在初版数据库中不落库，仅保留领域概念以便后续演进。

- **原始媒体策略**：上传用户可提交 `mp4`、`mov` 等任意受支持容器；`RegisterUpload` 写入基础元数据与 `raw_file_reference`（客户端上传前即分配对象路径），`raw_file_size` 等字段在直传完成并回执后再补写。媒体流水线完成后会在同名目录下生成 HLS (`master.m3u8` + 分片)，并将目录入口写入 `hls_master_playlist`。

- **不变量**：`status=ready` 时必须满足 `media_status=ready` 以及 `analysis_status=ready`；`status=published` 同时要求已通过可见性判定；`difficulty`、`summary` 等 AI 字段仅可在 `analysis_status=ready` 之后更新；阶段状态只能单向推进（失败除外）。
- **领域行为**：
  - `StartMediaProcessing` / `CompleteMediaProcessing`：将 `media_status` 从 `pending` 推进到 `processing` / `ready`，并同步媒体层字段。
  - `StartAnalysis` / `CompleteAnalysis`：将 `analysis_status` 从 `pending` 推进到 `processing` / `ready`，并写入 AI 层字段（难度、摘要、标签、原始字幕 URL 等）。
  - `RecomputeOverallStatus`：根据两个阶段状态刷新 `status`（任一阶段 `failed` → `status=failed`；两者 `ready` → `status=ready`；否则 `status=processing`）。
  - `Publish`：仅允许 `status=ready` 时转 `published`，填充 `publish_at` 与 `visibility_status`。
  - `Reject`：允许在 `status` ∈ `{ready, processing}` 时转 `rejected`，记录 `takedown_reason`。
  - `FailProcessing`：标记相应阶段为 `failed` 并将总体状态置为 `failed`；可通过 `UpdateProcessingStatus(stage, new_status=processing)` 将阶段回到 `processing` 并重试。
  - 所有行为都会发出领域事件（详见 §6）。

### 2.2 值对象

- `ProcessingStatus`：枚举 + 状态机校验。
- `Visibility`：包含公开等级、区域限制、年龄分级，负责规则验证。
- `Topic`：`type` + `label` + `confidence`。
- `RegionRestriction`：`country_code` + `mode`(`allow`/`deny`)，校验 ISO 3166-1。

### 2.3 状态机与并发约束（MVP）

| 当前状态                            | 触发行为                                               | 下一状态         | 触发方               | 备注                                               |
| ----------------------------------- | ------------------------------------------------------ | ---------------- | -------------------- | -------------------------------------------------- |
| `pending_upload`                    | `RegisterUpload`（初始）                               | `pending_upload` | Upload               | 创建记录，未上传完毕                               |
| `pending_upload` → `processing`     | `StartMediaProcessing` / `StartAnalysis` 任一开始      | `processing`     | Media / AI           | 阶段状态进入 `processing` 即刷新总体状态           |
| `processing` → `ready`              | `CompleteMediaProcessing` + `CompleteAnalysis` 均完成  | `ready`          | Media + AI           | 需在服务层校验两个阶段均为 `ready`                 |
| `ready` → `published`               | `Publish` / `FinalizeVisibility`                       | `published`      | Safety（MVP 手动）   | 可设置 `visibility_status`、`publish_at`         |
| 任意 → `failed`                     | `FailProcessing`（媒体/分析阶段失败）                  | `failed`         | Media / AI / Catalog | 写入 `error_message`，可触发重试                   |
| `failed` → `processing`             | `UpdateProcessingStatus(stage, new_status=processing)` | `processing`     | Media / AI / Catalog | 重置对应阶段为 `processing`，version 自增          |
| `ready` / `processing` → `rejected` | `Reject`                                               | `rejected`       | Safety / Catalog     | MVP 仅记录 `error_message`，不可恢复到 `published` |

- 阶段状态 `media_status` / `analysis_status` 仅允许 `pending → processing → ready` 或 `processing → failed`，恢复需显式调用 `UpdateProcessingStatus(stage, new_status=processing)`。
- Service 层在任何状态变更前均需持久化 version 并检查 `expected_version`；version 采用单调自增（`videos.version` 列）保证并发有序。
- `Publish` 仅在 `status=ready` 时允许；`OverrideStatus`（非 MVP）不纳入当前实现。

---

## 3. 数据模型（Postgres `catalog` schema）

### 3.1 表结构

```sql
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
  version              bigint not null default 1,                          -- 并发控制版本号（乐观锁）
  media_status         catalog.stage_status  not null default 'pending',   -- 媒体阶段
  analysis_status      catalog.stage_status  not null default 'pending',   -- AI 阶段
  media_job_id         text,                                               -- 最近一次媒体流水线任务ID
  media_emitted_at     timestamptz,                                        -- 最近一次媒体结果回写时间
  analysis_job_id      text,                                               -- 最近一次 AI 任务ID
  analysis_emitted_at  timestamptz,                                        -- 最近一次 AI 结果回写时间

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

  -- 可见性层字段（Safety 写入）
  visibility_status   text not null default 'public',                     -- 可见性状态 public/unlisted/private
  publish_at          timestamptz,                                        -- 发布时间（UTC），可为空

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
comment on column catalog.videos.version             is '乐观锁版本号：每次业务更新自增，用于并发控制与事件 version';
comment on column catalog.videos.media_status        is '媒体阶段状态：pending/processing/ready/failed（转码/封面等）';
comment on column catalog.videos.analysis_status     is 'AI 阶段状态：pending/processing/ready/failed（ASR/标签/摘要等）';
comment on column catalog.videos.media_job_id        is '最近一次媒体流水线任务ID（用于幂等与事件序）';
comment on column catalog.videos.media_emitted_at    is '最近一次媒体任务完成时间（用于拒绝旧事件）';
comment on column catalog.videos.analysis_job_id     is '最近一次 AI 任务ID（用于幂等与事件序）';
comment on column catalog.videos.analysis_emitted_at is '最近一次 AI 任务完成时间（用于拒绝旧事件）';

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
comment on column catalog.videos.visibility_status   is '可见性状态：public/unlisted/private，由 Safety 服务写入';
comment on column catalog.videos.publish_at          is '发布时间（UTC），当视频上架时写入';

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
```

> **预留字段（Post-MVP，暂不落库）**
>
> - `checksum`：原始文件校验和。
> - `language`：主语言（ISO 639-1）。
> - `topics`：主题标签数组。
> - `accent`：口音标签。
> - `segments_count`：语义片段数量。
> - `renditions`：多码率产物列表（Media 写入）。
> - `processing_progress`：处理进度百分比。
> - `visibility_status`：可见性 public/unlisted/private（Safety 写入）。
> - `publish_at`：发布时间（Safety 写入）。
> - `region_restrictions`：区域限制列表。
> - `age_rating`：年龄分级。
> - `takedown_reason`：拒绝/下架原因。

辅助表：

- `catalog.video_audit_trail(video_id, from_status, to_status, actor_type, actor_id, reason, metadata, occurred_at)`（Post-MVP 预留，当前实现未写入 actor 信息）
- `catalog.video_outbox(event_id UUID, video_id TEXT, event_type TEXT, payload JSONB, occurred_at TIMESTAMPTZ, published_at TIMESTAMPTZ NULL)`
- `catalog.idempotency_keys(key TEXT PRIMARY KEY, video_id TEXT, response JSONB, created_at TIMESTAMPTZ)`
- （可选）`catalog.tags(tag_id TEXT PRIMARY KEY, name TEXT, category TEXT)` — 仅在需要集中管理标签元数据时启用

### 3.2 读模型（可选）

- 物化视图 `catalog.video_cards_mv`：预聚合列表所需字段，定期刷新或事件驱动增量更新。
- `catalog.visibility_snapshot`：缓存 region/age 过滤结果，加速批量校验。

---

## 4. 服务结构与仓库布局

历史版本参考：Catalog 曾将读写面拆分为权威写侧（`catalog`）与只读投影消费者（`catalog-read`）。当前实现仅保留写侧，以下目录结构仅作设计档案参考：

```text
services/catalog/
├── cmd/grpc                # Catalog 主服务入口（写接口 + 查询 RPC）
├── cmd/tasks/outbox        # Outbox 发布器（二进制，可与主服务分离部署）
├── cmd/migrate             # 数据库迁移脚本执行入口
├── internal
│   ├── controllers         # gRPC Handler / Problem Details 映射
│   ├── services            # 业务用例层（RegisterUpload / UpdateMediaInfo ...）
│   ├── repositories        # pgx/sqlc DAO、GCS/Kafka 适配
│   ├── models
│   │   ├── po              # 数据库存储对象
│   │   └── vo              # 对外返回结构
│   ├── events              # Outbox 写入与发布接口
│   ├── infrastructure      # 配置加载、pgxpool、logger、OTel、idempotency
│   └── tasks               # 定时任务 / Outbox Runner / 补偿任务
├── api/proto/catalog/v1    # gRPC 契约（buf 管理）
├── api/openapi             # REST 契约（Spectral lint）
└── migrations              # Supabase Postgres 迁移脚本

<!-- 历史设计：catalog-read 投影进程在当前版本已移除，以下结构仅保留做参考。 -->
services/catalog-read/
├── cmd/consumer            # StreamingPull 消费入口（订阅 video.events）
├── internal
│   ├── consumers           # Pub/Sub 订阅处理器，实现投影更新
│   ├── repositories        # `catalog_read` 投影表与 inbox DAO
│   ├── services            # 幂等 UPSERT、重建回放逻辑
│   ├── infrastructure      # Pub/Sub 客户端、pgxpool、OTel
│   └── exporter            # 指标导出（catalog_projection_lag_seconds 等）
└── migrations              # `catalog_read` schema 迁移脚本
```

- Catalog 主服务遵循 `controllers → services → repositories` 分层；`catalog-read` 复用相同风格，聚焦事件消费与读模型维护。
- 配置项示例：`PG_DSN`, `CATALOG_READ_DSN`, `OUTBOX_POLL_INTERVAL`, `PUBSUB_PROJECT_ID`, `PUBSUB_TOPIC`, `PUBSUB_SUBSCRIPTION`, `IDEMPOTENCY_TTL`。

---

## 5. gRPC 契约（video.v1）

Catalog 暴露的对外接口全部收敛在 `video.v1` 包内的三个 gRPC Service：`UploadService`、`CatalogLifecycleService` 与 `CatalogQueryService`。所有写接口都会从请求头解析 `x-md-idempotency-key`、`If-*` 以及 `X-Apigateway-Api-Userinfo`，通过 `metadata.HandlerMetadata` 注入上下文，实现幂等与权限校验。错误码统一映射到 `video.v1.ErrorReason`（详见 §5.4）。

### 5.1 UploadService

**公开 RPC**

- `InitResumableUpload(InitResumableUploadRequest) → InitResumableUploadResponse`

**请求字段与校验**

- `size_bytes`：允许 0（客户端可能无法预估），用于写入 `catalog.uploads.expected_size`。
- `content_type`：必须为非空的 MIME；服务端会再做白名单校验（`video/mp4`、`video/quicktime` 等，见 `UploadService` 内的 allowlist）。
- `content_md5_hex`：32 位十六进制（buf-validate 保证格式），用于 `(user_id, content_md5)` 幂等约束。
- `duration_seconds`：1~300，限制移动端单条上传时长（MVP 上限 5 分钟）。
- `title`、`description`：至少 1 个字符，直接写入 `catalog.videos`。

**行为**

- 解析 `X-Apigateway-Api-Userinfo`，缺失或非法时返回 `ERROR_REASON_UPLOAD_INVALID`。
- 生成 `video_id` 与对象路径 `raw_videos/{user_id}/{video_id}`。
- `(user_id, content_md5)` 唯一：若已有进行中的会话，直接返回既有 `video_id` 和签名 URL；若会话状态为 `completed` 则返回 `ERROR_REASON_UPLOAD_ALREADY_COMPLETED`。
- 使用 GCS V4 签名生成 Resumable Session 初始化 URL，默认有效期 `signed_url_ttl`（配置为 900 秒）。URL 过期需重新调用本 RPC。
- 成功后 UPSERT `catalog.uploads`（状态 `uploading`），记录签名 URL、过期时间、预期大小、描述等。

**响应**

- `video_id`：后续生命周期 RPC 使用。
- `resumable_init_url`：客户端 `POST` + `x-goog-resumable:start` 使用的会话初始化 URL。
- `expires_at_unixms`：签名过期时间（毫秒）。

错误场景映射到 `ERROR_REASON_UPLOAD_INVALID` / `ERROR_REASON_UPLOAD_ALREADY_COMPLETED`。

### 5.2 CatalogLifecycleService

写模型统一通过 `CatalogLifecycleService` 暴露，所有 RPC 都依赖 `Idempotency-Key`（来自 `x-md-idempotency-key`），并支持 `expected_version`/`expected_status` 做并发控制。服务实现位于 `internal/services/*_service.go`，调用 `LifecycleWriter` 以事务方式写业务表与 Outbox。

| RPC | 调用方 | 输入要点 | 行为摘要 |
| --- | --- | --- | --- |
| `RegisterUpload` | Upload Service | `upload_user_id`, `title`, `raw_file_reference`, `description?` | 创建 `catalog.videos` 基础记录（`status=pending_upload`），写入 Outbox `catalog.video.created`，返回 `VideoRevision`（包含版本号、事件 ID、时间戳）。 |
| `UpdateOriginalMedia` | Upload Service | `raw_file_size?`, `raw_resolution?`, `raw_bitrate?`, `expected_version?` | 直传完成后补充原始媒体信息并自增版本；`expected_version` 不匹配返回 `FAILED_PRECONDITION`。 |
| `UpdateProcessingStatus` | Media / AI Pipeline | `stage`(`MEDIA`/`ANALYSIS`), `new_status`, `job_id`, `emitted_at`, `expected_status?`, `error_message?`, `expected_version?` | 校验阶段迁移合法性与时间戳新鲜度：`failed` 会同步将整体 `status` 置为 `failed`；重复或过期回调被拒绝。 |
| `UpdateMediaInfo` | Media Pipeline | `duration_micros?`, `encoded_resolution?`, `encoded_bitrate?`, `thumbnail_url?`, `hls_master_playlist?`, `media_status?`, `job_id`, `emitted_at`, `expected_version?` | 写入转码产物并可同步将 `media_status` 置为 `ready`/`failed`。`job_id` 与 `emitted_at` 作为幂等因子。 |
| `UpdateAIAttributes` | AI Pipeline | `difficulty?`, `summary?`, `tags[]`, `raw_subtitle_url?`, `analysis_status?`, `error_message?`, `job_id`, `emitted_at`, `expected_version?` | 写入 AI 语义层数据；当媒体和分析阶段均 `ready` 时，总体 `status` 变为 `ready`。 |
| `ArchiveVideo` | 运营 / 自动策略 | `reason?`, `expected_version?` | 将视频标记为 `archived` 并记录原因；响应包含新的版本号与事件 ID。 |

**通用约束**

- 成功响应统一返回 `VideoRevision`（`video_id`、状态/阶段状态、版本、`updated_at`、事件 ID、`occurred_at`）。
- 不存在的视频 → `NOT_FOUND`（`ERROR_REASON_VIDEO_NOT_FOUND`）。
- 参数或状态冲突 → `INVALID_ARGUMENT/FAILED_PRECONDITION`（`ERROR_REASON_VIDEO_UPDATE_INVALID`）。
- 所有写操作均会写出 `catalog.video.created/updated/deleted` 中的某一种事件（见 §6）。

### 5.3 CatalogQueryService

查询接口面向 Gateway 与内部服务，控制器会将 `X-Apigateway-Api-Userinfo` 注入，以便返回用户态布尔字段。

| RPC | 作用 | 说明 |
| --- | --- | --- |
| `GetVideoMetadata(video_id)` | 返回媒体/AI 元数据与聚合计数 | 读取 `catalog.videos` 与衍生视图，附带 `duration_micros`、`encoded_resolution`、`difficulty`、`summary`、`tags`、`raw_subtitle_url`、`version` 以及点赞/收藏/观看次数。 |
| `GetVideoDetail(video_id)` | 公共详情页补水 | 读取 ready/published 视图并补充用户态字段：`has_liked`、`has_bookmarked`（来自 `catalog.video_user_engagements_projection`），统计来自 `catalog.video_engagement_stats_projection`。当 `X-Apigateway-Api-Userinfo` 无法解析为合法 UUID 时返回 `ERROR_REASON_VIDEO_ID_INVALID`。 |
| `ListUserPublicVideos(page_size, page_token)` | 列出公开视频 | 仅返回 `status=published` 的条目，按 `created_at DESC, video_id DESC` 排序，游标编码在 `next_page_token`。`page_size` 超过 100 会被裁剪。 |
| `ListMyUploads(page_size, page_token, status_filter[], stage_filter[])` | 列出当前用户上传的全部视频 | 需要从 metadata 解析用户 ID；支持 `status_filter` 与阶段过滤（枚举值在 proto 中约束），并返回 `version` 以便前端执行乐观锁。 |

所有查询通过 `WithinReadOnlyTx` 执行，成功路径返回 `videov1.VideoDetail`、`VideoMetadata`、`VideoListItem`、`MyUploadListItem`，并在控制器层转换为 Problem Details/ETag 友好的响应格式。

### 5.4 错误码映射

`video.v1.ErrorReason` 当前定义并使用以下取值：

- `ERROR_REASON_VIDEO_NOT_FOUND`：指定视频不存在或非公开。
- `ERROR_REASON_VIDEO_ID_INVALID`：请求中的 `video_id` 或用户身份头无法解析为 UUID。
- `ERROR_REASON_QUERY_VIDEO_FAILED` / `ERROR_REASON_QUERY_TIMEOUT`：查询层内部故障或超时。
- `ERROR_REASON_VIDEO_UPDATE_INVALID`：生命周期请求的参数非法、状态冲突或分页游标失效。
- `ERROR_REASON_VIDEO_DELETE_INVALID`：保留给未来删除/强制下架流程。
- `ERROR_REASON_UPLOAD_INVALID`：上传初始化请求缺失必要字段或鉴权失败。
- `ERROR_REASON_UPLOAD_ALREADY_COMPLETED`：同 `(user_id, content_md5)` 的上传会话已成功完成。

控制器会将 gRPC `Status` 映射为 RFC 9457 Problem Details，保留 `error_reason`（enum 名称）、`status`, `title`, `detail` 与可选 `trace_id` 便于前端定位问题。

## 6. 领域事件与 Outbox

| 事件名                             | 触发条件                                 | 关键字段                                                                                          | 主要订阅方                                  |
| ---------------------------------- | ---------------------------------------- | ------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| `catalog.video.created`  | `RegisterUpload` 成功 | `video_id`, `upload_user_id`, `title`, `description?`, `status`, `media_status`, `analysis_status`, `occurred_at`, `version` | Feed、Search、Reporting 等需要新视频上线的服务 |
| `catalog.video.updated`  | 任意生命周期写接口在同一事务内更新了 `catalog.videos` | payload 仅包含发生变化的字段（标题、描述、状态、阶段状态、媒体/AI 信息、可见性等），并携带最新 `version`、`occurred_at` | Feed、Search、RecSys、自建投影服务 |
| `catalog.video.deleted`* | 归档/删除（未来扩展） | `video_id`, `deleted_at`, `reason?`, `version` | 预留给治理/审计，下游目前可忽略 |

- **事件字段约束（MVP）**
  - 每条事件至少包含：`event_id`、`aggregate_id=video_id`、`version`、`occurred_at`。（`actor` 字段 Post-MVP 规划，当前未输出）
  - `catalog.video.created` 输出完整快照，供下游构建初始投影。
  - `catalog.video.updated` 仅携带发生变化的字段；下游收到事件后应与本地状态合并，未出现的字段视为维持原值。
  - `catalog.video.deleted` 目前未在代码路径中触发，预留给后续治理流程；下游可选择忽略。
  - 所有事件 payload 使用 Protobuf，并保持向后兼容（只追加字段，禁止复用已有 tag）。

### 6.1 Outbox 发布（Pub/Sub）

- Outbox Relay 通过 `LISTEN/NOTIFY` + 指数退避轮询相结合的策略认领事件，批量（100~500 条）执行 `FOR UPDATE SKIP LOCKED`，按 `occurred_at` 顺序发布到 **Pub/Sub Topic `video.events`** 并写回 `published_at`。发布失败会设置 `next_retry_at` 并退避重试。
- 发布消息时使用 `aggregate_id` 作为 **ordering key**，携带 `event_id`、`trace_id`、`occurred_at`、`version` 等元数据（`actor_*` 头字段 Post-MVP 预留），payload 按 `kratos-gateway/只读投影方案.md` 约定的 Protobuf schema 序列化。
- 实现细节（退避参数、Exactly-once、DLQ、监控指标）统一复用《只读投影方案》中的参考实现，Catalog 服务无需单独定制。

### 6.2 读模型策略

- 当前版本取消 Catalog 内部的 `catalog-read` 投影进程，所有读流量直接访问主表 `catalog.videos` 并结合 `catalog.video_user_engagements_projection` 投影聚合用户态字段。
- Outbox 事件仍持续发布，Search / Feed / Progress 等下游服务可根据自身诉求消费事件构建各自读模型。
- 若后续需要恢复集中式投影，可在新进程中复用现有 Outbox 事件与 `video_user_engagements_projection` 表设计，独立部署并提供只读接口。

---

## 7. 集成契约

### 7.1 Upload → Catalog

- 调用 `RegisterUpload` 创建记录，返回 `video_id`，并初始化 `status=pending_upload`、`media_status=pending`、`analysis_status=pending`。
- 客户端完成直传后，Upload 服务调用 `UpdateOriginalMedia` 写入 `raw_file_size`、`raw_resolution`、`raw_bitrate` 等原始媒体信息。
- 失败场景：配额不足（429 `quota-exceeded`）、文件不合法等。
- GCS 策略需绑定 `video_id`，确保上传前必须先注册。

### 7.2 Media → Catalog

- Start：当转码任务受理后，调用 `UpdateProcessingStatus(stage=MEDIA, newStatus=processing)`（或等效 RPC）更新 `media_status`，总体 `status` 自动变为 `processing`。
- 成功处理：`UpdateMediaInfo` 写入 `duration_micros`、`encoded_resolution`、`encoded_bitrate`、`thumbnail_url`、`hls_master_playlist`，并将 `media_status` 置为 `ready`；若 `analysis_status` 亦为 `ready`，总体 `status` 会被重算为 `ready`。
- 失败：`UpdateProcessingStatus(stage=MEDIA, newStatus=failed, error_message=...)`，总体 `status` 同步置为 `failed`；必要时媒资管道可复用原始对象重新转码并再次调用 `UpdateProcessingStatus(stage=MEDIA, newStatus=processing, job_id)` 以恢复流程。

### 7.3 AI → Catalog

- Start：分析任务受理后调用 `UpdateProcessingStatus(stage=ANALYSIS, newStatus=processing)`。
- 分析完成：`UpdateAIAttributes` 写入 `difficulty`、`summary`、`tags`、`raw_subtitle_url` 等语义信息并将 `analysis_status` 置为 `ready`；若 `media_status` 也为 `ready`，总体 `status` 变为 `ready`。
- 失败：`UpdateProcessingStatus(stage=ANALYSIS, newStatus=failed, error_message=...)`，总体 `status` 同步为 `failed`。

### 7.4 Safety / QA → Catalog

- 审核通过：`FinalizeVisibility` 更新为 `published` / `unlisted` 并设置 `publish_at`。
- 审核拒绝：`FinalizeVisibility` 设置 `status=rejected`，记录 `takedown_reason`。

### 7.5 Gateway → Catalog

- Gateway 作为纯反向代理，将外部 REST `/api/v1/video*` 请求转换为 gRPC 调用 `CatalogQueryService`，响应体由 Catalog 基于主库数据组装，并遵循 Problem Details / ETag / 游标规范。
- 投影异常时，可在 Catalog 内部回退到主库或其他兜底路径，Gateway 无需感知；所有回退与降级需在 Catalog 侧记录审计并暴露告警。

### 7.6 Feed / Search / RecSys → Catalog

- 直接订阅 `video.events` 构建各自的只读投影（例如 `feed_read.video_cards`, `search_read.video_index`），按事件版本号做 UPSERT 幂等。
- 投影滞后或重建时，应通过回放事件恢复数据；不再依赖 Catalog gRPC Query 作为常规补水手段。

---

## 8. 非功能需求

| 类别     | 要求                                                                                                                                                                                                               |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 幂等     | 写接口均要求 `Idempotency-Key`，`catalog.idempotency_keys` 存储响应；重复请求返回首个结果。                                                                                                                        |
| 超时     | 外部调用（Engagement、数据库）设置 `<500ms` 超时；Lifecycle 请求整体超时 `3s`。                                                                                                                                    |
| 日志     | 使用 `log/slog` JSON，字段：`ts`, `level`, `msg`, `trace_id`, `video_id`, `status`（`actor` 字段 Post-MVP 预留）。                                                                                                 |
| 追踪     | OpenTelemetry span，如 `Catalog.RegisterUpload`，记录 `status`, `video_id`, `error`。                                                                                                                              |
| 指标     | 暴露 `catalog_processing_status_total`, `catalog_visibility_change_total`, `catalog_outbox_lag_seconds`, `catalog_idempotency_hits_total`, `catalog_engagement_event_lag_ms` 等核心指标。 |
| 安全     | gRPC 双向 TLS + OIDC 服务鉴权；只读接口支持用户 JWT（Gateway 透传）。                                                                                                                                              |
| 配额     | `RegisterUpload` 前调用 `QuotaChecker`；超限返回 429。                                                                                                                                                             |
| 审计     | 所有状态变更写入 `video_audit_trail`，提供 Admin 查询接口。                                                                                                                                                        |
| 失败恢复 | Outbox Relay 断点续传；`failed` 状态可通过 `OverrideStatus` 恢复；数据库持久化全部字段。                                                                                                                           |

---

## 9. 开发路线图

1. **契约**：编写 `api/proto/catalog/v1/*.proto`（Query / Lifecycle / Admin），执行 `buf lint` 与 `buf breaking`。
2. **领域实现**：创建 `internal/domain/video`，实现状态机及单测（覆盖率 ≥ 90%）。
3. **用例层**：实现 `RegisterUploadHandler`、`UpdateMediaInfoHandler`、`FinalizeVisibilityHandler` 等，使用 ports mock 做单测。
4. **持久层**：使用 `sqlc` 生成 DAO；实现 `TxManager`（pgx + pgxpool）。
5. **gRPC Server**：实现 Query / Lifecycle server，中间件包含认证、幂等、Problem Details、ETag。
6. **Outbox Relay**：实现后台进程（启动时加载），配置 `OUTBOX_POLL_INTERVAL`。
7. **测试**：领域与应用层单元测试；Testcontainers 集成测试覆盖完整流程（Register → Media → AI → Publish）。
8. **观测性**：暴露 `/metrics`；配置 OTel exporter（stdout / OTLP）。
9. **验证**：提供 `grpcurl` 示例、启动步骤（`PG_DSN`, `make run catalog`）。
10. **直传补写**：上传完成后及时调用 `UpdateOriginalMedia`，否则媒体/AI 阶段无法开始。

---

## 10. 风险与缓解

| 风险     | 描述                            | 缓解措施                                                                 |
| -------- | ------------------------------- | ------------------------------------------------------------------------ |
| 状态竞争 | 多个服务并发写同一视频          | 强制 `expected_status`/`expected_version` 校验 + `SELECT ... FOR UPDATE` |
| 事件丢失 | Outbox Relay 异常导致事件未发布 | 发布前重试，监控 `catalog_outbox_lag_seconds`                            |
| 超时退避 | Engagement 等下游慢导致详情阻塞 | 设置查询超时；失败时返回 `partial=true` 并记录日志                       |
| 配额绕过 | 客户端跳过 `RegisterUpload`     | GCS 策略绑定 `video_id`；Gateway 校验上传前必须注册                      |
| 审核延迟 | `ready` 视频长时间未发布        | `FinalizeVisibility` 支持 `auto_publish_after` 异步任务                  |
| 数据漂移 | AI/Media 重写导致历史数据缺失   | 审计表记录字段差异；事件 payload 附 `updated_fields`                     |

---

## 11. 后续扩展

- **读写分离**：引入专用读模型（Elasticsearch / pgvector）应对高并发查询。
- **多语言资源**：扩展字段 `subtitle_tracks[]`、`audio_tracks[]` 并与 Subtitle 服务协作。
- **版本管理**：新增 `VideoVersion` 表支持历史回溯与 AB 实验。
- **审核工作流**：与 Safety 服务建立基于事件的多阶段审核。
- **多租户**：增加 `tenant_id` 字段，索引需加前缀，确保数据隔离。

---

## 13. 版本记录

- **v0.1（2025-10-19）**：首版草案，覆盖 Query / Lifecycle 契约、状态机、事件、非功能需求。

---

> 实施前请同步更新 Gateway、Upload、Media、AI 等相关服务的契约与错误语义，确保 Problem Details 类型与事件命名保持一致。  
> TODO：实现后补充实际数据库迁移脚本与 `sqlc` 生成文件路径说明。
