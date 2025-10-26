# Catalog Service MVP Detailed Design (v1.0 · 2025-10-26)

> 面向 Catalog 服务首版 MVP 的设计说明，覆盖系统目标、边界、数据/接口契约、事件机制、非功能性约束与验收标准。阅读本文件前请先确认已理解《项目结构》《4MVC 架构》《只读投影方案》。

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
Catalog 服务维护**视频权威元数据**，协调上传→转码→AI→上架的生命周期，并向下游提供一致的读取视图与领域事件。

### 1.2 MVP 范围
| 能力 | 描述 | 责任接口 |
| ---- | ---- | -------- |
| 上传注册 | 创建 `video` 基础记录、分配 `video_id`、保存原始对象引用 | `CatalogLifecycleService/RegisterUpload` |
| 媒体进度 | 接收媒体流水线回调，更新媒体阶段状态与产物字段 | `UpdateProcessingStatus`(MEDIA)、`UpdateMediaInfo` |
| AI 进度 | 接收 AI 任务回调，更新 AI 阶段状态与语义字段 | `UpdateProcessingStatus`(ANALYSIS)、`UpdateAIAttributes` |
| 可见性审核 | 通过/拒绝/下架，写入版本并发事件 | `FinalizeVisibility`、`OverrideStatus` |
| 幂等 & 乐观锁 | 支持 `Idempotency-Key`、`expected_version`；重复调用返回首个结果 | Lifecycle 全部写接口 |
| 读接口 | 提供视频详情、我的上传、公开列表，并支持条件请求与游标分页 | `CatalogQueryService` |
| 事件发布 | 写事务内写入 `catalog.outbox_events`，后台发布到 `video.events` | Outbox Runner |
| 投影消费 | 独立 `catalog-read` 进程消费事件构建 `catalog_read.video_projection` | Projection Task |
| 审计 | 所有状态迁移写入 `catalog.video_audit_trail` | Lifecycle 用例 |

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
    PR[Projection Consumer]
  end
  subgraph Messaging
    PUB[Pub/Sub Topic: video.events]
  end
  subgraph Read Plane
    CR[Catalog Read gRPC]
    RP[(catalog_read schema)]
  end
  A --> S --> R --> DB
  S -->|Domain Events| OQ
  OR -->|Publish| PUB
  PR -->|Consume| PUB
  PR --> RP
  CR --> RP
  Gateway --> A
  Upload & Media & AI --> A
```

- **Catalog gRPC Server**：处理写/读请求，执行业务用例，并在事务内写 Outbox/InBox/审计表。
- **Outbox Runner**：扫描 `catalog.outbox_events`，发布 Protobuf 事件到 `video.events`，保证同一视频按 `ordering_key=video_id` 顺序。
- **Projection Consumer (`catalog-read`)**：消费事件并 UPSERT 到只读表，供查询接口使用；暴露投影延迟指标。
- **仓储边界**：主服务仅访问 `catalog` schema；读服务仅访问 `catalog_read` schema；禁止跨服务越权。

---

## 领域模型

### 3.1 聚合 `Video`
- **标识**：`video_id` (UUID)。
- **层次字段**：
  - 基础：`upload_user_id`, `title`, `description`, `raw_file_reference`, `created_at`, `version`。
  - 媒体：`media_status`, `duration_micros`, `encoded_resolution`, `encoded_bitrate`, `thumbnail_url`, `hls_master_playlist`, `media_job_id`, `media_emitted_at`。
  - AI：`analysis_status`, `difficulty`, `summary`, `tags`, `raw_subtitle_url`, `analysis_job_id`, `analysis_emitted_at`。
  - 可见性：`status`, `publish_time`, `visibility_status`, `takedown_reason`（MVP 先保留字段，逻辑暂仅支持 `status` 推进）。
  - 审计：`error_message`、`updated_at`。

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
| 表 | 作用 | 关键字段 | 约束 |
| --- | --- | --- | --- |
| `catalog.videos` | 视频主表 | 各层字段、`version`、阶段 `job_id`/`emitted_at` | 触发器维护 `updated_at`；枚举 `catalog.video_status` / `catalog.stage_status` |
| `catalog.outbox_events` | 事件 Outbox | `event_id`, `aggregate_type`, `aggregate_id`, `payload`, `headers`, `occurred_at`, `version`, `lock_token` | `available_at`/`published_at` 索引；见《只读投影方案》 |
| `catalog.inbox_events` | 外部事件幂等（预留） | `event_id`, `source_service`, `payload`, `processed_at` | MVP 暂仅用于投影回放；默认保持空 |
| `catalog.idempotency_keys` | 幂等存储 | `key`(PK), `video_id`, `response_payload`, `created_at`, `expires_at` | TTL 由后台任务清理 |
| `catalog.video_audit_trail` | 状态轨迹 | `audit_id`, `video_id`, `from_status`, `to_status`, `actor_type`, `actor_id`, `reason`, `metadata`, `occurred_at` | 插入仅由 Lifecycle 服务在事务内完成 |

> 迁移脚本：`migrations/001_*`~`004_*` 已包含核心表。需新增 `005_create_catalog_idempotency_table.sql` 与 `006_create_catalog_video_audit_trail.sql` 完成幂等/审计落库。

### 4.2 读库 (`catalog_read` schema)
| 表 | 字段 | 描述 |
| --- | --- | --- |
| `catalog_read.video_projection` | `video_id`, `title`, `status`, `media_status`, `analysis_status`, `duration_micros`, `thumbnail_url`, `hls_master_playlist`, `difficulty`, `summary`, `version`, `occurred_at`, `updated_at` | 主视图，供 Query 服务直接返回 |
| `catalog_read.user_video_projection` | `(user_id, video_id)`, `has_liked`, `has_bookmarked`, `has_watched`, `version`, `occurred_at`, `updated_at` | 结合 Engagement 事件提供用户态字段 |
| `catalog_read.projection_offsets` | `consumer`, `last_event_id`, `last_version`, `updated_at` | 记录重放/回滚进度，便于观测 |

读库仅由投影进程写入，Query 服务只读并可加只读事务。

### 4.3 指标/清理任务
- 幂等表定期清理 `expires_at < now()`。
- 审计表提供 `video_id` + 时间范围索引。
- Outbox 表通过 `delivery_attempts` 与 `last_error` 监控异常。

---

## 应用分层与依赖关系

### 5.1 目录结构
```
services/catalog/
  cmd/grpc            # 主 gRPC 入口
  cmd/tasks/outbox    # Outbox Runner 独立可执行文件
  cmd/tasks/projection# Projection Runner（可由 catalog-read 单独仓库维护）
  configs/            # config.yaml + .env
  internal/
    controllers/      # gRPC Handler (Lifecycle/Query/Admin)
    services/         # 用例实现（upload, media, ai, visibility, query）
    repositories/     # pgx/sqlc DAO + 幂等/审计仓储
    models/{domain,po,vo}
    tasks/{outbox,projection}
    infrastructure/   # configloader, grpc server/client, tx manager, jwt, metadata
  migrations/
  sqlc/
```

### 5.2 服务依赖
| 层级 | 输入 | 输出 |
| ---- | ---- | ---- |
| Controllers | gRPC 请求、Problem Details | 调用 Services、封装响应、处理 Metadata/ETag/Idempotency-Key |
| Services | DTO、Repository 接口、TxManager | 业务结果、领域事件、审计记录、Outbox 消息 |
| Repositories | pgxpool、sqlc 生成代码 | CRUD、投影 UPSERT、幂等写入、审计插入 |
| Tasks | Repositories、Pub/Sub 客户端 | 发布事件、消费事件、更新读库 |

### 5.3 生命周期用例拆分
- `RegisterUploadService`：创建基础记录，写入 audit、outbox(`video.created`)。
- `ProcessingStatusService`：处理媒体/AI 阶段状态推进，校验 `expected_stage_status`、`job_id`、`emitted_at`。
- `MediaInfoService`：写入转码产物，重算 overall status。
- `AIAttributesService`：写入语义字段，重算 overall status。
- `VisibilityService`：审核发布/拒绝，更新 `status` 并输出 `video.visibility_changed`。
- `VideoQueryService`：读取 `catalog_read` 投影，支持 `If-None-Match`，返回用户态字段（并列调用 Engagement 客户端时遵守 500ms 超时）。

---

## gRPC/API 契约

### 6.1 CatalogQueryService（只读）
- `GetVideoDetail(GetVideoDetailRequest) → GetVideoDetailResponse`
  - 请求：`video_id` (UUID)、可选 `If-None-Match` ETag。
  - 响应：`detail` + `etag` + `partial` 标记；当 Engagement 降级时 `partial=true`。
- `ListUserPublicVideos(ListUserPublicVideosRequest) → ListUserPublicVideosResponse`
  - 请求：`user_id`、分页 `page_size`/`page_token`。内部默认 `status=published`。
  - 响应：视频列表、`next_page_token`、`total`（可选）。
- `ListMyUploads(ListMyUploadsRequest) → ListMyUploadsResponse`
  - 请求：`user_id`、`stage_filter[]`、分页参数。
  - 响应：包含处理状态、`version`、阶段进度。

### 6.2 CatalogLifecycleService（写端）
- 通用请求头：`x-md-global-user-id`、`x-md-idempotency-key`、`x-md-if-match`/`x-md-if-none-match`，均由内嵌 BaseHandler 注入。
- 所有写请求包含：
  - `expected_version`（或 `expected_status`）。
  - `idempotency_key`（默认使用客户端生成的 ULID；媒体/AI 回调可使用 `job_id`）。
  - `actor` 信息（`actor_type`, `actor_id`）。
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
  - `codes.AlreadyExists` → `idempotency-replayed`。
  - `codes.NotFound` → `video-not-found`。
  - `codes.DeadlineExceeded` → `request-timeout`。
- 响应字段：`type`, `title`, `detail`, `status`, `trace_id`, `instance`。

---

## 事件与 Outbox 策略

### 7.1 事件清单
| 事件名 | 触发时机 | Payload 字段（摘要） | 订阅方 |
| ------ | -------- | -------------------- | ------ |
| `catalog.video.created` | `RegisterUpload` 成功 | `video_id`, `upload_user_id`, `title`, `status`, `media_status`, `analysis_status`, `version`, `occurred_at` | Catalog-read, Search, Feed |
| `catalog.video.media_ready` | 媒体阶段成功 | 媒体字段全量快照、`version`, `occurred_at`, `job_id` | Catalog-read, Media Analytics |
| `catalog.video.ai_enriched` | AI 阶段成功 | AI 字段全量快照、`version`, `occurred_at`, `job_id` | Catalog-read, Search |
| `catalog.video.processing_failed` | 任一阶段失败 | `failed_stage`, `error_message`, `version`, `occurred_at`, `job_id` | Alerting, Support |
| `catalog.video.visibility_changed` | 发布/拒绝/恢复 | `status`, `previous_status`, `publish_time`, `takedown_reason`, `actor` | Feed, Gateway Cache |
| `catalog.video.deleted` | 删除视频（暂非 MVP） | `video_id`, `version`, `occurred_at` | Catalog-read |

- Payload 以 Protobuf 定义在 `api/video/v1/events.proto`，字段只新增不复用 tag。
- 事件 Headers：`trace_id`, `idempotency_key`, `actor_type`, `actor_id`, `schema_version`。

### 7.2 Outbox 发布
- Runner 采用 `FOR UPDATE SKIP LOCKED` + 指数退避；默认批量大小 200，最大尝试 10 次。
- `ordering_key = video_id.String()`，确保同一聚合顺序。
- 消费失败写回 `last_error` 并延迟 `available_at`。
- 成功发布后填充 `published_at`，并在日志/指标记录耗时。

---

## 读模型与投影进程

### 8.1 投影流程
1. StreamingPull 取消息 → 写 `catalog_read.inbox_events`（幂等）。
2. 按 `event_type` 分派：
   - `created`：直接 UPSERT 新记录。
   - `media_ready` / `ai_enriched`：基于当前记录更新局部字段，保留 `version` 单调递增。
   - `processing_failed`：更新 `status` = `failed` 并记录 `error_message`。
   - `visibility_changed`：更新 `status` 与 `publish_time`。
3. 更新 `projection_metrics`：
   - `projection_apply_success_total{event_type}`
   - `projection_event_lag_ms`
   - `projection_apply_failure_total{event_type}`
4. Ack 消息。

### 8.2 回放/补偿
- 通过 `projection_offsets` 记录最后处理的 `event_id` 与 `version`。
- 支持命令 `cmd/tasks/projection --replay-from <timestamp>` 清空投影后重放。
- 若投影滞后 > 5 分钟，Query 服务降级到主库读取并记录告警。

---

## 非功能需求

### 9.1 可靠性
- 所有写接口默认超时 3s；对下游（Engagement、Profile）调用设置 500ms 超时。
- 事务：使用 `txmanager.WithinTx`，默认隔离级别 `read_committed`，遇到 `serialization_failure` 可重试 ≤3 次。
- 幂等：`
  - BaseHandler 解析 `x-md-idempotency-key` → Service 调用 `IdempotencyStore.Begin(key)`；若命中，则直接返回缓存响应。
  - 成功事务提交后 `IdempotencyStore.Commit(key, response, ttl)`。
- 审计：所有状态变更在事务内写入 `video_audit_trail`（触发器/函数 `catalog.fn_log_status_change`）。

### 9.2 安全
- 认证：
  - 入站：`gcjwt.ServerMiddleware` 校验 OIDC audience (`catalog-lifecycle`, `catalog-query`)；本地可 `skip_validate`。
  - 出站：调用其他服务采用 `gcjwt.ClientMiddleware` 注入服务间 token。
- 授权：
  - Lifecycle 接口校验 `actor_type`（枚举：`upload_service`, `media_service`, `ai_service`, `safety_service`, `operator`）。
  - Query 接口根据 Gateway 注入的用户信息判断是否允许访问非公开视频。
- 元数据：统一使用 `x-md-*` / `x-md-global-*` 前缀，Server 端仅允许白名单字段透传。

### 9.3 观测
- 日志：`log/slog` JSON，字段 `ts`, `level`, `msg`, `trace_id`, `span_id`, `video_id`, `status`, `actor_type`。
- 指标：
  - `catalog_lifecycle_duration_ms{method}`
  - `catalog_outbox_lag_seconds`
  - `catalog_projection_lag_seconds`
  - `catalog_idempotency_hits_total`
  - `catalog_audit_records_total`
- 追踪：每个 gRPC 方法创建 span，附加属性 `video.id`, `actor.type`, `status`, `version`。

### 9.4 性能
- 预计 QPS：写 ≤ 50，读 ≤ 500。当前 PG 配置（`max_open_conns=4`）可支撑 MVP；需要时可提升。
- `sqlc` 查询全部使用 Prepared Statement（除 Supabase Pooler 场景，可通过配置关闭）。

---

## 验收清单

| 项目 | 验收标准 | 验证方式 |
| ---- | -------- | -------- |
| Schema 就绪 | `migrations` 执行后存在所有主/辅表、索引、触发器 | `psql` 验证 + `sqlc generate` 通过 |
| gRPC 契约 | Proto 定义涵盖 Query + Lifecycle；`buf lint`、`buf breaking` 通过 | CI / `make lint` |
| 幂等实现 | Lifecycle 接口重复调用返回相同结果；`idempotency_keys` 命中率指标可观测 | 单元测试 + 集成测试 |
| 审计记录 | 状态变更后 `video_audit_trail` 记录正确 | 集成测试 |
| Outbox 发布 | 手动触发事件后 Pub/Sub 收到消息，`catalog_outbox_lag_seconds < 5s` | 本地模拟 Runner |
| 投影同步 | `catalog-read` 能消费事件并在 1s 内更新 read schema | `go test` + e2e 脚本 |
| Query 接口 | `GetVideoDetail` 支持 ETag，`List*` 支持分页 | gRPCurl 用例 |
| 超时 & 重试 | 对 Engagement 模拟超时，服务返回 `partial=true` 且日志/指标记录 | 测试脚本 |
| 覆盖率 | 服务层单测覆盖率 ≥ 80%，关键分支（状态机、幂等）需有用例 | `go test -cover` |
| 文档 | README/设计文件与实现一致，给出启动/验证步骤 | 文档评审 |

---

## 实施里程碑

1. **阶段一：契约与数据基础**（2 日）
   - 完成 proto 拆分、`buf lint`。
   - 编写/执行迁移（幂等、审计、读库扩展）。
   - 更新 `sqlc.yaml`、生成 DAO。

2. **阶段二：业务用例与控制器**（3 日）
   - 实现 Lifecycle 服务及单测（状态机、幂等、审计）。
   - 更新 BaseHandler，支持 ETag/Idempotency。
   - 完成 Query 服务读投影逻辑、Engagement 降级策略。

3. **阶段三：事件与投影**（2 日）
   - 扩充 Outbox 构造器、事件 payload。
   - 实现投影 Runner（含指标、回放）。
   - 编写端到端脚本验证事件→投影→查询链路。

4. **阶段四：非功能与验收**（2 日）
   - 接入 OTel、日志字段、指标导出。
   - 覆盖幂等/审计/异常路径测试。
   - 更新 README & 运维手册，联调 Gateway/Upload/Media/AI。

---

## 风险、回滚与后续演进

### 12.1 风险 & 缓解
| 风险 | 影响 | 缓解 |
| ---- | ---- | ---- |
| 媒体/AI 重放旧回调 | 旧数据覆盖 | `emitted_at` 比较 + 版本校验，拒绝旧回调 |
| 投影滞后 | 读接口数据陈旧 | 监控 `catalog_projection_lag_seconds`，>5 分钟触发告警并临时回退主库读取 |
| Outbox 累积 | 事件延迟 | 设置 `available_at` + 再平衡 Runner；记录 `delivery_attempts` |
| 幂等表膨胀 | 存储压力 | TTL 清理任务（默认 7 天） + 指标监控 |
| 服务间鉴权误配 | 写接口被滥用 | 在 config.yaml 明确 `allowed_actor_types`，上线前联调 IAM |

### 12.2 回滚策略
- Schema 回滚：使用事务化迁移（`BEGIN..COMMIT`），提供 `DOWN` 脚本。
- 服务回滚：保留上一版二进制；Outbox/投影使用版本号确保兼容；若事件结构有破坏性变更，需 bump `schema_version` 并双写。

### 12.3 后续演进
- 引入 `catalog-read` 独立仓库与 HTTP Cache。
- 扩展可见性字段（区域、年龄、租户）。
- 与 Search 服务联动建立倒排索引。
- 提供 Admin 控制台接口（高级过滤、批量操作）。

---

> 本设计文档与实现需保持同步。任何重大调整（事件结构、表字段、接口行为）都必须同步更新本文件并走设计评审。
