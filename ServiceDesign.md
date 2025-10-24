# Catalog Service Detailed Design (v0.1 · 2025-10-19)

> 本文作为 Catalog 模块的工程实现说明，用于指导 MVP → 可演进阶段的开发与评审。覆盖领域职责、数据主权、gRPC 契约、事件、非功能需求与工程落地要求。阅读本档前请先熟悉《1 项目概述》《2 对外 API 概述》《4DDD-lite 架构》《6 语言范式与骨架》。

---

## 1. 使命与边界

- **核心使命**：维护视频的权威元数据与可见性真相，协调上传 → 转码 → AI → 上架的流程，为下游提供一致且可信的读取视图。
- **关键原则**
  - **单一写入者**：Catalog 负责 `videos` 表的所有写入；其他服务只能通过 Catalog 端口提交更新。
  - **分层字段**：基础层、媒体层、AI 语义层、可见性层分别由各责任方生产，由 Catalog 聚合。
  - **事件驱动**：所有变更同步写入 Outbox，发布供 Search / Feed / RecSys 消费。
  - **访问控制**：外部经 Gateway；内部写接口仅接受受信服务身份，并要求幂等。

---

## 2. 领域模型

### 2.1 聚合根 `Video`

| 层级       | 字段                                                                                                                                                                                                                                           | 说明                                                                                            | 来源                     |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- | ------------------------ |
| 基础层     | `video_id`(ULID)、`upload_user_id`、`created_at`、`title`、`description`、`raw_file_reference`、`status`(`pending_upload`→`processing`→`ready`→`published`/`failed`/`rejected`/`archived`)、`media_status`、`analysis_status`、`error_message` | 上传记录、元信息、原始对象存储路径、阶段状态与错误信息；`raw_file_reference` 在注册上传时即写入 | Catalog + Upload         |
| 媒体层     | `duration_micros`、`encoded_resolution`、`encoded_bitrate`、`thumbnail_url`、`hls_master_playlist`                                                                                                                                             | 转码后 HLS 列表（同名文件夹下 `master.m3u8`）、封面与时长（微秒）、目标码率                     | Media Pipeline           |
| AI 层      | `difficulty`、`summary`、`tags`、`raw_subtitle_url`                                                                                                                                                                                            | AI 评估难度、生成摘要、标签以及原始字幕存储位置                                                 | AI Content Understanding |
| 可见性层\* | `visibility_status`(`public`/`unlisted`/`private`)、`region_restrictions[]`、`age_rating`、`publish_time`、`takedown_reason`                                                                                                                   | 上架/区域/版权裁决（Post-MVP）                                                                  | Safety & QA / 运营       |

> \* 标记为 Post-MVP 的字段在初版数据库中不落库，仅保留领域概念以便后续演进。

- **原始媒体策略**：上传用户可提交 `mp4`、`mov` 等任意受支持容器；`RegisterUpload` 写入基础元数据与 `raw_file_reference`（客户端上传前即分配对象路径），`raw_file_size` 等字段在直传完成并回执后再补写。媒体流水线完成后会在同名目录下生成 HLS (`master.m3u8` + 分片)，并将目录入口写入 `hls_master_playlist`。

- **不变量**：`status=ready` 时必须满足 `media_status=ready` 以及 `analysis_status=ready`；`status=published` 同时要求已通过可见性判定；`difficulty`、`summary` 等 AI 字段仅可在 `analysis_status=ready` 之后更新；阶段状态只能单向推进（失败除外）。
- **领域行为**：
  - `StartMediaProcessing` / `CompleteMediaProcessing`：将 `media_status` 从 `pending` 推进到 `processing` / `ready`，并同步媒体层字段。
  - `StartAnalysis` / `CompleteAnalysis`：将 `analysis_status` 从 `pending` 推进到 `processing` / `ready`，并写入 AI 层字段（难度、摘要、标签、原始字幕 URL 等）。
  - `RecomputeOverallStatus`：根据两个阶段状态刷新 `status`（任一阶段 `failed` → `status=failed`；两者 `ready` → `status=ready`；否则 `status=processing`）。
  - `Publish`：仅允许 `status=ready` 时转 `published`，填充 `publish_time` 与 `visibility_status`。
  - `Reject`：允许在 `status` ∈ `{ready, processing}` 时转 `rejected`，记录 `takedown_reason`。
  - `FailProcessing`：标记相应阶段为 `failed` 并将总体状态置为 `failed`；可通过 `RestartProcessing` 将阶段回到 `processing` 并重试。
  - 所有行为都会发出领域事件（详见 §6）。

### 2.2 值对象

- `ProcessingStatus`：枚举 + 状态机校验。
- `Visibility`：包含公开等级、区域限制、年龄分级，负责规则验证。
- `Topic`：`type` + `label` + `confidence`。
- `RegionRestriction`：`country_code` + `mode`(`allow`/`deny`)，校验 ISO 3166-1。

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
> - `publish_time`：发布时间（Safety 写入）。
> - `etag_version`：并发控制版本号（用于 ETag）。
> - `region_restrictions`：区域限制列表。
> - `age_rating`：年龄分级。
> - `takedown_reason`：拒绝/下架原因。

辅助表：

- `catalog.video_audit_trail(video_id, from_status, to_status, actor_type, actor_id, reason, metadata, occurred_at)`
- `catalog.video_outbox(event_id UUID, video_id TEXT, event_type TEXT, payload JSONB, occurred_at TIMESTAMPTZ, published_at TIMESTAMPTZ NULL)`
- `catalog.idempotency_keys(key TEXT PRIMARY KEY, video_id TEXT, response JSONB, created_at TIMESTAMPTZ)`
- （可选）`catalog.tags(tag_id TEXT PRIMARY KEY, name TEXT, category TEXT)` — 仅在需要集中管理标签元数据时启用

### 3.2 读模型（可选）

- 物化视图 `catalog.video_cards_mv`：预聚合列表所需字段，定期刷新或事件驱动增量更新。
- `catalog.visibility_snapshot`：缓存 region/age 过滤结果，加速批量校验。

---

## 4. 服务结构（DDD-lite）

```text
services/catalog
├── cmd/http                # 健康检查 / metrics
├── cmd/grpc                # gRPC 入口
├── internal
│   ├── domain/video        # 实体 / 值对象 / 领域事件
│   ├── app
│   │   ├── port            # Repo / Outbox / TxManager / Clock / IDGenerator
│   │   └── usecase         # RegisterUpload / UpdateOriginalMedia / UpdateMediaInfo / UpdateAIAttributes / FinalizeVisibility / GetVideoDetail ...
│   └── infra
│       ├── repo            # sqlc 生成 + 自定义查询
│       ├── event           # Outbox writer & relay
│       ├── grpc            # Query / Lifecycle server
│       ├── auth            # 服务身份校验
│       ├── middleware      # Idempotency / Logging / ProblemDetails / Metadata (ETag)
│       └── config          # 环境配置
└── api/proto/catalog/v1    # gRPC 契约（buf 管理）
```

- 分层依赖：`domain → app → infra`。
- 配置项示例：`PG_DSN`, `KAFKA_TOPIC_PREFIX`, `OUTBOX_POLL_INTERVAL`, `VISIBILITY_DEFAULT`, `MAX_TOPICS`, `IDEMPOTENCY_TTL`。

---

## 5. gRPC 契约

### 5.1 `CatalogQueryService`

```proto
service CatalogQueryService {
  // 返回用户无关的视频元数据（媒体、AI 字段），供内部组装
  rpc GetVideoMetadata(GetVideoMetadataRequest) returns (GetVideoMetadataResponse);

  // Gateway 调用；返回包含用户 engagement 信息的单视频详情（现在不做，post mvp）
  rpc GetVideoDetail(GetVideoDetailRequest) returns (GetVideoDetailResponse);

  // Gateway 调用；列出指定用户的公开视频
  rpc ListUserPublicVideos(ListUserPublicVideosRequest) returns (ListUserPublicVideosResponse);

  // Gateway 调用；列出当前用户上传的全部视频（含非公开状态）
  rpc ListMyUploads(ListMyUploadsRequest) returns (ListMyUploadsResponse);

  // Feed/Search 调用；批量补无用户态的卡片字段
  rpc BatchGetCards(BatchGetCardsRequest) returns (BatchGetCardsResponse);

  // Feed/Search 调用；批量校验可见性
  rpc CheckVisibility(CheckVisibilityRequest) returns (CheckVisibilityResponse);
}
```

- `GetVideoMetadata`：返回与用户无关的客观元数据（媒体、AI 字段等），可供 Gateway 或内部服务组合使用。
- `GetVideoDetail`：支持 `If-None-Match`（metadata `if-none-match`），并在内部调用 Engagement 服务补齐用户态字段（点赞、收藏、观看进度）。
- `ListUserPublicVideos`：过滤 `status=published AND visibility_status=public`，提供游标与 `Link` 风格信息。
- `ListMyUploads`：校验 `user_id`，返回所有状态及处理进度。
- `BatchGetCards`：用于 Feed/Search 补水，仅返回客观字段（title, thumbnail, duration, status, visibility, publish_time）。
- `CheckVisibility`：批量判定可见性，返回 `allowed` / `denied` 及 `reason`。

### 5.2 `CatalogLifecycleService`

```proto
service CatalogLifecycleService {
  // Upload 服务调用；注册上传并生成 video_id
  rpc RegisterUpload(RegisterUploadRequest) returns (RegisterUploadResponse);

  // Upload 服务调用；直传完成后补写原始媒体信息
  rpc UpdateOriginalMedia(UpdateOriginalMediaRequest) returns (VideoRevision);

  // Upload / Media / AI 调用；更新阶段状态
  rpc UpdateProcessingStatus(UpdateProcessingStatusRequest) returns (VideoRevision);

  // Media 服务调用；写入转码结果
  rpc UpdateMediaInfo(UpdateMediaInfoRequest) returns (VideoRevision);

  // AI 服务调用；写入分析结果与标签
  rpc UpdateAIAttributes(UpdateAIAttributesRequest) returns (VideoRevision);

}
```

- 安全：要求 mTLS + OIDC（audience=`catalog-lifecycle`）并校验 `actor_type`（`upload_service`、`media_service`、`ai_service`、`safety_service`、`operator`）。
- 幂等：所有写请求必须包含 `idempotency_key`，重复请求返回首个结果。
- 并发控制：需传入 `expected_status` 或 `expected_version`，冲突返回 `codes.FailedPrecondition`（Problem type `status-conflict`）。
- 原始媒体更新：上传完成后由 Upload 服务调用 `UpdateOriginalMedia` 写入 `raw_file_size`、`raw_resolution`、`raw_bitrate` 等字段。
- 阶段更新：`UpdateProcessingStatus` 需要携带 `stage`（`MEDIA`/`ANALYSIS`）与目标阶段状态，服务会据此更新 `media_status` 或 `analysis_status` 并重算总体 `status`。

### 5.3 `CatalogAdminService`（post mvp 后续扩展）

```proto
service CatalogAdminService {
  // 运营后台调用；搜索视频并支持多条件过滤
  rpc SearchVideos(AdminSearchVideosRequest) returns (AdminSearchVideosResponse);

  // 运营后台调用；获取单条视频的审计轨迹
  rpc GetAuditTrail(GetAuditTrailRequest) returns (GetAuditTrailResponse);

  // 运营后台调用；获取处理/审核统计指标
  rpc GetProcessingMetrics(GetProcessingMetricsRequest) returns (GetProcessingMetricsResponse);

  // Safety/运营调用；审核发布或拒绝
  rpc FinalizeVisibility(FinalizeVisibilityRequest) returns (VideoRevision);

  // 运营调用；强制调整状态（紧急下架等）
  rpc OverrideStatus(OverrideStatusRequest) returns (VideoRevision);
}
```

> 该 Service 仅供内部控制台使用，需独立 IAM 角色与网络隔离。

---

## 6. 领域事件与 Outbox

| 事件名                             | 触发条件                                 | 关键字段                                                                               | 主要订阅方                 |
| ---------------------------------- | ---------------------------------------- | -------------------------------------------------------------------------------------- | -------------------------- |
| `catalog.video.created`            | `RegisterUpload` 成功                    | `video_id`, `upload_user_id`, `status`, `raw_file_reference`, `occurred_at`            | Search, Reporting          |
| `catalog.video.stage_updated`      | 任一阶段（媒体/分析）状态变化            | `video_id`, `stage`, `previous_stage_status`, `new_stage_status`, `status`, `trace_id` | Monitoring, Media 控制台   |
| `catalog.video.media_ready`        | `UpdateMediaInfo` 成功                   | `video_id`, `duration_micros`, `thumbnail_url`, `hls_master_playlist`                  | Feed, Search, AI           |
| `catalog.video.ai_enriched`        | `UpdateAIAttributes` 成功                | `video_id`, `difficulty`, `summary`, `tags`, `raw_subtitle_url`                        | Search, RecSys             |
| `catalog.video.visibility_changed` | `FinalizeVisibility` 或 `OverrideStatus` | `video_id`, `visibility_status`, `publish_time`, `region_restrictions`                 | Feed, Gateway 缓存, Search |
| `catalog.video.processing_failed`  | 状态转为 `failed`                        | `video_id`, `error_message`, `failed_stage`                                            | Alerting, Upload, Support  |
| `catalog.video.restored`           | 从失败/拒绝恢复                          | `video_id`, `previous_status`, `new_status`, `actor`                                   | Audit, Reporting           |

- Outbox Relay 每 200ms 轮询 `FOR UPDATE SKIP LOCKED`，批量发布到 Kafka 或进程事件总线；成功后写入 `published_at`。
- 事件 payload 必含 `event_id`, `trace_id`, `occurred_at`, `version`, `actor`。

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
- 失败：`UpdateProcessingStatus(stage=MEDIA, newStatus=failed, error_message=...)`，总体 `status` 同步置为 `failed`；必要时媒资管道可复用原始对象重新转码并调用 `RestartProcessing` 使阶段回到 `processing`。

### 7.3 AI → Catalog

- Start：分析任务受理后调用 `UpdateProcessingStatus(stage=ANALYSIS, newStatus=processing)`。
- 分析完成：`UpdateAIAttributes` 写入 `difficulty`、`summary`、`tags`、`raw_subtitle_url` 等语义信息并将 `analysis_status` 置为 `ready`；若 `media_status` 也为 `ready`，总体 `status` 变为 `ready`。
- 失败：`UpdateProcessingStatus(stage=ANALYSIS, newStatus=failed, error_message=...)`，总体 `status` 同步为 `failed`。

### 7.4 Safety / QA → Catalog

- 审核通过：`FinalizeVisibility` 更新为 `published` / `unlisted` 并设置 `publish_time`。
- 审核拒绝：`FinalizeVisibility` 设置 `status=rejected`，记录 `takedown_reason`。

### 7.5 Gateway → Catalog

- 通过 Query Service 获取详情 / 列表；响应需映射 Problem Details、ETag、游标等约束。
- gRPC 错误映射示例：`codes.NotFound` → HTTP 404，`codes.ResourceExhausted` → HTTP 429。

### 7.6 Feed / Search / RecSys → Catalog

- 通过 `BatchGetCards` 及 `CheckVisibility` 补全候选列表。
- 订阅事件更新索引和缓存；要保证消费幂等（使用 `event_id`）。

---

## 8. 非功能需求

| 类别     | 要求                                                                                                                                           |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| 幂等     | 写接口均要求 `Idempotency-Key`，`catalog.idempotency_keys` 存储响应；重复请求返回首个结果。                                                    |
| 超时     | 外部调用（Engagement、数据库）设置 `<500ms` 超时；Lifecycle 请求整体超时 `3s`。                                                                |
| 日志     | 使用 `log/slog` JSON，字段：`ts`, `level`, `msg`, `trace_id`, `video_id`, `status`, `actor`。                                                  |
| 追踪     | OpenTelemetry span，如 `Catalog.RegisterUpload`，记录 `status`, `video_id`, `error`。                                                          |
| 指标     | 暴露 `catalog_processing_status_total`, `catalog_visibility_change_total`, `catalog_outbox_lag_seconds`, `catalog_idempotency_hits_total` 等。 |
| 安全     | gRPC 双向 TLS + OIDC 服务鉴权；只读接口支持用户 JWT（Gateway 透传）。                                                                          |
| 配额     | `RegisterUpload` 前调用 `QuotaChecker`；超限返回 429。                                                                                         |
| 审计     | 所有状态变更写入 `video_audit_trail`，提供 Admin 查询接口。                                                                                    |
| 失败恢复 | Outbox Relay 断点续传；`failed` 状态可通过 `OverrideStatus` 恢复；数据库持久化全部字段。                                                       |

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

## 12. 运行示例

```bash
# 启动 Catalog gRPC（包含 Outbox Relay）
APP_ENV=dev PG_DSN="postgres://..." go run ./services/catalog/cmd/grpc

# 健康检查
curl -sf http://localhost:8081/healthz

# gRPC 请求示例：注册上传（仅提供基础元信息）
grpcurl \
  -d '{\
"uploadUserId":"user_123",\
"title":"Clip - Day 1 Warmup",\
"description":"Warmup conversation clip for day 1."\
}' \
  -H "idempotency-key: upload-123" \
  localhost:9090 catalog.v1.CatalogLifecycleService/RegisterUpload

# gRPC 请求示例：直传完成后补写原始媒体信息
grpcurl \
  -d '{\
"videoId":"vid_01HXXYYY",\
"rawFileSize":1048576,\
"rawResolution":"1920x1080",\
"rawBitrate":7200,\
"rawFileReference":"gs://bucket/uploads/clip_day1.mov"\
}' \
  -H "idempotency-key: upload-confirm-123" \
  localhost:9090 catalog.v1.CatalogLifecycleService/UpdateOriginalMedia
```

---

## 13. 版本记录

- **v0.1（2025-10-19）**：首版草案，覆盖 Query / Lifecycle 契约、状态机、事件、非功能需求。

---

> 实施前请同步更新 Gateway、Upload、Media、AI 等相关服务的契约与错误语义，确保 Problem Details 类型与事件命名保持一致。  
> TODO：实现后补充实际数据库迁移脚本与 `sqlc` 生成文件路径说明。
