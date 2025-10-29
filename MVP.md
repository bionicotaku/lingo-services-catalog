# Catalog Service MVP Detailed Design (v1.0 · 2025-10-26)

> 面向 Catalog 服务首版 MVP 的设计说明，覆盖系统目标、边界、数据/接口契约、事件机制、非功能性约束与验收标准。写作、实现与后续更新须严格参照模板服务（kratos-template）的目录结构、编码规范与文档格式。阅读本文件前请先确认已理解《项目结构》《4MVC 架构》《只读投影方案》。

---

## 目录

1. [目标与范围](#目标与范围)
2. [系统概览](#系统概览)
3. [领域模型](#领域模型)
4. [数据模型与存储设计](#数据模型与存储设计)
5. [应用分层与依赖关系](#应用分层与依赖关系)
6. [gRPC/API 契约](#grpcapi-契约)
7. [事件与 Outbox 策略](#事件与-outbox-策略)
8. [读模型与投影进程](#读模型与投影进程)
9. [非功能需求](#非功能需求)
10. [验收清单](#验收清单)
11. [实施里程碑](#实施里程碑)
12. [风险、回滚与后续演进](#风险回滚与后续演进)

---

## 目标与范围

### 1.1 愿景

Catalog 服务维护**视频权威元数据**，协调上传 → 转码 →AI→ 上架的生命周期，并向下游提供一致的读取视图与领域事件。

### 1.2 MVP 范围

| 能力               | 描述                                                                 | 责任接口 / 组件                                          |
| ------------------ | -------------------------------------------------------------------- | -------------------------------------------------------- |
| 上传注册           | 创建 `video` 基础记录、分配 `video_id`、保存原始对象引用             | `CatalogLifecycleService/RegisterUpload`                 |
| 媒体进度           | 接收媒体流水线回调，更新媒体阶段状态与产物字段                       | `UpdateProcessingStatus`(MEDIA)、`UpdateMediaInfo`       |
| AI 进度            | 接收 AI 任务回调，更新 AI 阶段状态与语义字段                         | `UpdateProcessingStatus`(ANALYSIS)、`UpdateAIAttributes` |
| 可见性审核         | 通过/拒绝/下架，写入版本并发事件                                     | `FinalizeVisibility`、`OverrideStatus`                   |
| 乐观锁             | 支持 `expected_version`，并在冲突时返回 `FailedPrecondition`         | Lifecycle 全部写接口                                     |
| 读接口             | 提供视频详情、我的上传、公开列表，直接查询 `catalog.videos` 并按需关联用户态状态 | `CatalogQueryService`                                    |
| 事件发布           | 写事务内写入 `catalog.outbox_events`，后台发布到 `video.events`      | Outbox Runner                                            |
| Engagement 用户态投影 | 订阅 Engagement 事件构建 `catalog.video_user_engagements_projection`（liked/bookmarked 两布尔） | Engagement Projection Runner                             |

超出 MVP 的能力（多租户、可见性精细化、搜索索引、多语言等）在设计中预留接口，但不在验收范围。

### 1.3 不做的事

- 不承担视频文件存储/转码（由 Upload/Media 服务负责）。
- 不直接暴露外部 REST，所有外部流量经 Gateway。
- 不实现全文搜索与推荐排序（交由 Search/Feed 服务）。

---

## 系统概览

```mermaid
graph TD
  subgraph Catalog Service (Write Plane)
    A[Catalog gRPC Server]
    S[Services Layer]
    R[Repositories]
    DB[(Supabase Postgres<br/>catalog schema)]
    OQ[(catalog.outbox_events)]
  end
  subgraph Background Workers
    OR[Outbox Runner]
    EP[Engagement Projection Consumer]
  end
  subgraph Messaging
    PUB[Pub/Sub Topic: video.events]
    ENG[Pub/Sub Topic: profile.engagement.events]
  end
  subgraph Engagement State Store
    UV[(catalog.video_user_engagements_projection)]
  end
  A --> S --> R --> DB
  S -->|Domain Events| OQ
  OR -->|Publish| PUB
  EP --> UV
  ENG --> EP
  Gateway --> A
  Upload & Media & AI --> A
```

- **Catalog gRPC Server**：处理写/读请求，执行业务用例，并在事务内写入 Outbox/InBox；读接口直接查询主表。
- **Outbox Runner**：扫描 `catalog.outbox_events`，发布 Protobuf 事件到 `video.events`，保证同一视频按 `ordering_key=video_id` 顺序。
- **Engagement Projection Consumer**：订阅 Profile 服务发布的 `profile.engagement.*` 事件，将用户态三元状态写入 `catalog.video_user_engagements_projection`，供 Query 组合。
- **仓储边界**：Catalog 主服务访问 `catalog` schema；Engagement 投影仅访问同 schema 下的 `video_user_engagements_projection` 表；禁止跨服务越权。

---

## 领域模型

### 3.1 聚合 `Video`

- **标识**：`video_id` (UUID)。
- **层次字段**：
  - 基础：`upload_user_id`, `title`, `description`, `raw_file_reference`, `created_at`, `version`。
  - 媒体：`media_status`, `duration_micros`, `encoded_resolution`, `encoded_bitrate`, `thumbnail_url`, `hls_master_playlist`, `media_job_id`, `media_emitted_at`。
  - AI：`analysis_status`, `difficulty`, `summary`, `tags`, `raw_subtitle_url`, `analysis_job_id`, `analysis_emitted_at`。
  - 可见性：`status`, `publish_time`, `visibility_status`, `takedown_reason`（MVP 先保留字段，逻辑暂仅支持 `status` 推进）。

### 3.2 状态机

- `status` 流程：`pending_upload` → `processing` ↔ `ready` → `published` / `failed` / `rejected` / `archived`。
- 阶段状态：`media_status`、`analysis_status` ∈ `{pending,processing,ready,failed}`，只能单向推进（失败除外）。
- 不变量：
  - `status=ready` ⇒ `media_status=ready ∧ analysis_status=ready`。
  - `status=published` ⇒ `status` 必须先到 `ready`。
  - 同一阶段回调必须携带 `job_id` 与 `emitted_at`，若 `emitted_at` 落后于已存值则拒绝写入。
- 版本：每次变更自增 `version`；事件 payload 与投影均使用 `version` 做幂等。

---

## 数据模型与存储设计

### 4.1 主库 (`catalog` schema)

| 表                          | 作用                 | 关键字段                                                                                                          | 约束                                                                          |
| --------------------------- | -------------------- | ----------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| `catalog.videos`            | 视频主表             | 各层字段、`version`、阶段 `job_id`/`emitted_at`                                                                   | 触发器维护 `updated_at`；枚举 `catalog.video_status` / `catalog.stage_status` |
| `catalog.outbox_events`     | 事件 Outbox          | `event_id`, `aggregate_type`, `aggregate_id`, `payload`, `headers`, `occurred_at`, `version`, `lock_token`        | `available_at`/`published_at` 索引；见《只读投影方案》                        |
| `catalog.video_user_engagements_projection` | 用户态投影           | `(user_id, video_id)` 主键，`has_liked`, `has_bookmarked`, `occurred_at`                                         | 由 Engagement 投影消费者写入，缺失记录视为 false                             |
| `catalog.inbox_events`      | 外部事件幂等（预留） | `event_id`, `source_service`, `payload`, `processed_at`                                                           | 供未来跨服务回放使用，MVP 默认为空                                          |

> 迁移脚本：`migrations/001_*`~`004_*` 已包含核心表。MVP 若需新增表，仅针对 Engagement 状态存储补充 `005_*` 及后续脚本。幂等与审计存储保留为 Post-MVP 能力。

该用户态投影由 Engagement 投影消费者写入。Catalog Query 在需要返回用户态字段时左连接 `catalog.video_user_engagements_projection`；缺失记录视为 `false`。

### 4.2 指标/清理任务

- Outbox 表通过 `delivery_attempts` 与 `last_error` 监控异常。
- `catalog.video_user_engagements_projection` 可按需维护偏移量（例如追加 `catalog.video_user_engagements_projection_offsets`）以支持回放，MVP 可暂以内存偏移实现。

---

## 应用分层与依赖关系

### 5.1 目录结构

```
services/catalog/
  cmd/grpc            # 主 gRPC 入口
  cmd/tasks/outbox    # Outbox Runner 独立可执行文件
  cmd/tasks/engagement# Engagement 用户态投影 Runner
  configs/            # config.yaml + .env
  internal/
    controllers/      # gRPC Handler (Lifecycle/Query/Admin)
    services/         # 用例实现（upload, media, ai, visibility, query）
    repositories/     # pgx/sqlc DAO
    models/{domain,po,vo}
    tasks/{outbox,engagement}
    infrastructure/   # configloader, grpc server/client, tx manager, jwt, metadata
  migrations/
  sqlc/
```

### 5.2 服务依赖

| 层级         | 输入                            | 输出                                                        |
| ------------ | ------------------------------- | ----------------------------------------------------------- |
| Controllers  | gRPC 请求、Problem Details      | 调用 Services、封装响应、处理 Metadata/ETag                 |
| Services     | DTO、Repository 接口、TxManager | 业务结果、领域事件、Outbox 消息                               |
| Repositories | pgxpool、sqlc 生成代码          | CRUD、用户态投影写入                                           |
| Tasks        | Repositories、Pub/Sub 客户端    | 发布事件、消费 Engagement 事件并更新 `video_user_engagements_projection`     |

### 5.3 生命周期用例拆分

- `RegisterUploadService`：创建基础记录，写入 audit、outbox(`video.created`)。
- `ProcessingStatusService`：处理媒体/AI 阶段状态推进，校验 `expected_stage_status`、`job_id`、`emitted_at`。
- `MediaInfoService`：写入转码产物，重算 overall status。
- `AIAttributesService`：写入语义字段，重算 overall status。
- `VisibilityService`：审核发布/拒绝，更新 `status` 并输出 `video.visibility_changed`。
- `VideoQueryService`：直接查询 `catalog.videos`，并左连接 `catalog.video_user_engagements_projection` 组装用户态字段（并列调用 Engagement 客户端时遵守 500ms 超时）。

---

## gRPC/API 契约

### 6.1 CatalogQueryService（只读）

- **用户身份来源**：BaseHandler 会从 gRPC metadata 解析 `X-Apigateway-Api-Userinfo`（API Gateway 注入的 JWT payload）并注入到 Context。服务实现需从 Context 读取 `HandlerMetadata` 获取 `user_id`（取 `sub`/`user_id`），匿名调用时 `user_id` 为空，接口需按匿名逻辑降级。
- `GetVideoDetail(GetVideoDetailRequest) → GetVideoDetailResponse`
  - 请求：`video_id` (UUID)、可选 `If-None-Match` ETag。
  - 响应：`detail` + `etag` + `partial` 标记；当 Engagement 降级或无用户信息时 `partial=true`。
- `ListUserPublicVideos(ListUserPublicVideosRequest) → ListUserPublicVideosResponse`
  - 请求：分页参数 `page_size`/`page_token`；用户身份从 metadata 获取，匿名场景返回公开列表。
  - 响应：视频列表、`next_page_token`、`total`（可选）。
- `ListMyUploads(ListMyUploadsRequest) → ListMyUploadsResponse`
  - 请求：分页参数及可选 `stage_filter[]`；服务通过 metadata 得到 `user_id`，若缺失则返回鉴权错误或空集。
  - 响应：包含处理状态、`version`、阶段进度。

### 6.2 CatalogLifecycleService（写端）

- 通用请求头：`X-Apigateway-Api-Userinfo`、`x-md-if-match`/`x-md-if-none-match`，均由内嵌 BaseHandler 注入。
- 所有写请求包含：
  - `expected_version`（或 `expected_status`）。
  - （Post-MVP）`actor` 信息（`actor_type`, `actor_id`）；MVP 阶段暂不下传，不额外透传。
- 关键 RPC：
  1. `RegisterUpload`：创建视频，返回 `video_id`, `version`, `occurred_at`, `event_id`。
  2. `UpdateOriginalMedia`：补写 `raw_file_*` 属性。
  3. `UpdateProcessingStatus`：推进阶段状态；当 `new_status=failed` 时填充 `error_message` + outbox `video.processing_failed`。
  4. `UpdateMediaInfo` / `UpdateAIAttributes`：写入产物，刷新 `status`，发出 `video.media_ready` / `video.ai_enriched`。
  5. `FinalizeVisibility`：发布或拒绝视频，生成 `video.visibility_changed`。
  6. `OverrideStatus`（MVP：仅管理员调用，执行手工恢复或强制下架）。

### 6.3 Error & ProblemDetails

- 错误码：
  - `codes.InvalidArgument` → Problem type `validation-error`。
  - `codes.FailedPrecondition` → `status-conflict`（版本/状态不匹配）。
  - `codes.NotFound` → `video-not-found`。
  - `codes.DeadlineExceeded` → `request-timeout`。
- 响应字段：`type`, `title`, `detail`, `status`, `trace_id`, `instance`。

---

## 事件与 Outbox 策略

### 7.1 事件清单

| 事件名                             | 触发时机              | Payload 字段（摘要）                                                                                         | 订阅方                        |
| ---------------------------------- | --------------------- | ------------------------------------------------------------------------------------------------------------ | ----------------------------- |
| `catalog.video.created`            | `RegisterUpload` 成功 | `video_id`, `upload_user_id`, `title`, `status`, `media_status`, `analysis_status`, `version`, `occurred_at` | Search, Feed, Analytics       |
| `catalog.video.media_ready`        | 媒体阶段成功          | 媒体字段全量快照、`version`, `occurred_at`, `job_id`                                                         | Media Pipeline, Monitoring    |
| `catalog.video.ai_enriched`        | AI 阶段成功           | AI 字段全量快照、`version`, `occurred_at`, `job_id`                                                          | Search, Recommendation        |
| `catalog.video.processing_failed`  | 任一阶段失败          | `failed_stage`, `error_message`, `version`, `occurred_at`, `job_id`                                          | Alerting, Support             |
| `catalog.video.visibility_changed` | 发布/拒绝/恢复        | `status`, `previous_status`, `publish_time`, `takedown_reason`（actor 元数据 Post-MVP 预留）                 | Feed, Gateway Cache           |
| `catalog.video.deleted`            | 删除视频（暂非 MVP）  | `video_id`, `version`, `occurred_at`                                                                         | Downstream 清理任务           |

- Payload 以 Protobuf 定义在 `api/video/v1/events.proto`，字段只新增不复用 tag。
- 事件 Headers：`trace_id`, `schema_version`（`actor_type/actor_id` Post-MVP 预留）。

### 7.2 Outbox 发布

- Runner 采用 `FOR UPDATE SKIP LOCKED` + 指数退避；默认批量大小 200，最大尝试 10 次。
- `ordering_key = video_id.String()`，确保同一聚合顺序。
- 消费失败写回 `last_error` 并延迟 `available_at`。
- 成功发布后填充 `published_at`，并在日志/指标记录耗时。

---

## Engagement 用户态投影

### 8.1 事件来源与格式

- Profile 服务通过 `profile.engagement.events`（Pub/Sub）发布用户互动事件，包含 `event_name`, `state`, `engagement_type`, `user_id`, `video_id`, `occurred_at` 等字段。
- Runner 需要根据 `occurred_at` 只保留最新事件，避免乱序覆盖旧值。

### 8.2 消费流程

1. StreamingPull 从 `profile.engagement.events` 拉取消息。
2. 解析 payload，构造 `catalog.video_user_engagements_projection` 记录，缺失字段使用默认值 `false`。
3. 使用 `INSERT ... ON CONFLICT (user_id, video_id) DO UPDATE` 写入布尔字段与 `occurred_at`（若新事件时间更新）。
4. 更新指标后 Ack 消息；写入失败需记录 `last_error` 并按指数退避重试。

### 8.3 回放与偏移

- Runner 在内存中维护 offset；如需持久化，可在 Post-MVP 阶段扩展 `catalog.video_user_engagements_projection_offsets`。
- 当重新消费或回放历史事件时，先清空目标视频用户记录再执行顺序回放，保证与 `occurred_at` 一致。

### 8.4 观测指标

- `catalog_engagement_apply_success_total`
- `catalog_engagement_apply_failure_total`
- `catalog_engagement_event_lag_ms`

---

## 非功能需求

### 9.1 可靠性

- 所有写接口默认超时 3s；对下游（Engagement、Profile）调用设置 500ms 超时。
- 事务：使用 `txmanager.WithinTx`，默认隔离级别 `read_committed`，遇到 `serialization_failure` 可重试 ≤3 次。

### 9.2 安全

- 认证：
  - 入站：`gcjwt.ServerMiddleware` 校验 OIDC audience (`catalog-lifecycle`, `catalog-query`)；本地可 `skip_validate`。
  - 出站：调用其他服务采用 `gcjwt.ClientMiddleware` 注入服务间 token。
- 授权：
- Lifecycle 接口校验 `actor_type`（Post-MVP 规划，MVP 阶段可跳过；如需启用，枚举值参考：`upload_service`, `media_service`, `ai_service`, `safety_service`, `operator`）。
  - Query 接口根据 Gateway 注入的用户信息判断是否允许访问非公开视频。
- 元数据：统一使用 `x-md-*` / `x-md-global-*` 前缀，Server 端仅允许白名单字段透传。

### 9.3 观测

- 日志：`log/slog` JSON，字段 `ts`, `level`, `msg`, `trace_id`, `span_id`, `video_id`, `status`（`actor_type` Post-MVP 预留）。
- 指标：
  - `catalog_lifecycle_duration_ms{method}`
  - `catalog_outbox_lag_seconds`
  - `catalog_engagement_lag_ms`
- 追踪：每个 gRPC 方法创建 span，附加属性 `video.id`, `status`, `version`（`actor.type` Post-MVP 预留）。

### 9.4 性能

- 预计 QPS：写 ≤ 50，读 ≤ 500。当前 PG 配置（`max_open_conns=4`）可支撑 MVP；需要时可提升。
- `sqlc` 查询全部使用 Prepared Statement（除 Supabase Pooler 场景，可通过配置关闭）。

---

## 验收清单

| 项目        | 验收标准                                                                | 验证方式                           |
| ----------- | ----------------------------------------------------------------------- | ---------------------------------- |
| Schema 就绪 | `migrations` 执行后存在所有主/辅表、索引、触发器                        | `psql` 验证 + `sqlc generate` 通过 |
| gRPC 契约   | Proto 定义涵盖 Query + Lifecycle；`buf lint`、`buf breaking` 通过       | CI / `make lint`                   |
| Outbox 发布 | 手动触发事件后 Pub/Sub 收到消息，`catalog_outbox_lag_seconds < 5s`      | 本地模拟 Runner                    |
| Engagement 投影 | 消费 Engagement 事件后 `catalog.video_user_engagements_projection` 在 1s 内更新        | `go test` + e2e 脚本               |
| Query 接口  | `GetVideoDetail` 支持 ETag，`List*` 支持分页                            | gRPCurl 用例                       |
| 超时 & 重试 | 对 Engagement 模拟超时，服务返回 `partial=true` 且日志/指标记录         | 测试脚本                           |
| 覆盖率      | 服务层单测覆盖率 ≥ 80%，关键分支（状态机、可见性）需有用例              | `go test -cover`                   |
| 文档        | README/设计文件与实现一致，给出启动/验证步骤                            | 文档评审                           |

---

## 实施里程碑

1. **阶段一：契约与数据基础**（2 日）

   - 完成 proto 拆分、`buf lint`。
   - 编写/执行迁移（`catalog.video_user_engagements_projection`）。
   - 更新 `sqlc.yaml`、生成 DAO。

2. **阶段二：业务用例与控制器**（3 日）

   - 实现 Lifecycle 服务及单测（状态机）。
   - 更新 BaseHandler，支持 ETag。
   - 完成 Query 服务读投影逻辑、Engagement 降级策略。

3. **阶段三：事件与用户态投影**（2 日）

   - 扩充 Outbox 构造器、事件 payload。
   - 实现 Engagement 投影 Runner（含指标、回放策略）。
   - 编写端到端脚本验证事件 → 用户态投影 → 查询链路。

4. **阶段四：非功能与验收**（2 日）
   - 接入 OTel、日志字段、指标导出。
   - 覆盖异常路径测试。
   - 更新 README & 运维手册，联调 Gateway/Upload/Media/AI。

---

## 风险、回滚与后续演进

### 12.1 风险 & 缓解

| 风险               | 影响           | 缓解                                                                     |
| ------------------ | -------------- | ------------------------------------------------------------------------ |
| 媒体/AI 重放旧回调 | 旧数据覆盖     | `emitted_at` 比较 + 版本校验，拒绝旧回调                                 |
| 用户态投影滞后     | 用户态数据陈旧 | 监控 `catalog_engagement_lag_ms`，>5 分钟触发告警并回放 Engagement 事件 |
| Outbox 累积        | 事件延迟       | 设置 `available_at` + 再平衡 Runner；记录 `delivery_attempts`            |
| 读库膨胀           | 存储压力       | 定期归档早期投影数据或引入物化视图刷新策略                               |
| 服务间鉴权误配     | 写接口被滥用   | 在 config.yaml 明确 `allowed_actor_types`（Post-MVP），上线前联调 IAM    |

### 12.2 回滚策略

- Schema 回滚：使用事务化迁移（`BEGIN..COMMIT`），提供 `DOWN` 脚本。
- 服务回滚：保留上一版二进制；Outbox/投影使用版本号确保兼容；若事件结构有破坏性变更，需 bump `schema_version` 并双写。

### 12.3 后续演进

- 引入 `catalog-read` 独立仓库与 HTTP Cache。
- 扩展可见性字段（区域、年龄、租户）。
- 与 Search 服务联动建立倒排索引。
- 提供 Admin 控制台接口（高级过滤、批量操作）。
- 引入写接口幂等存储（`catalog.idempotency_keys`）及相应指标。
- 引入审计轨迹（`catalog.video_audit_trail`）与相关查询/指标。

---

> 本设计文档与实现需保持同步。任何重大调整（事件结构、表字段、接口行为）都必须同步更新本文件并走设计评审。
