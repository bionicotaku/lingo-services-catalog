# 实现《只读投影方案》落地 TODO

## 现状基线
- ~~`services/catalog` 当前仅提供视频查询 gRPC（`internal/controllers/video_handler.go` → `internal/services/video.go` → `internal/repositories/video_repo.go`），无事件发布与只读投影。~~
- ~~数据层只有主权 schema `catalog` 与 sqlc 查询；`internal/tasks` 仅保留占位文档，尚未实现 Outbox 发布或订阅消费。~~
- **已完成**: 数据库已包含 `catalog.outbox_events`、`catalog.inbox_events` 表，sqlc 已生成相关查询方法
- 配置（`configs/config.yaml`）未定义 Pub/Sub、Outbox、读模型等参数；Wire 仅组装 gRPC 与数据库。

## 阶段 0｜准备与基线验证
- [x] 审核现有 `kratos-template/docs/只读投影方案.md`、`投影一致性问题解决方案.md` 与代码，明确契约与目录约束。
- [ ] 盘点依赖：确认本地 Pub/Sub 模拟器或 GCP 凭证、Supabase Schema 迁移流程、`sqlc`, `wire`, `buf` 版本一致性。
- [ ] 补充测试数据库 & Pub/Sub 启动脚本说明（若缺失则纳入 README/Troubleshooting 更新计划）。
- [x] 建立 Outbox/Inbox 模板共享方案：在 `lingo-utils/outbox` 新增 DDL/sqlc 模板与渲染工具，可跨服务复用（2025-10-25）

## 阶段 1｜数据库与代码生成 ✅
- [x] 新增迁移：
  - [x] `catalog.outbox_events` 表（字段按照方案统一结构，含索引/约束/注释）✅ 2025-10-24
  - [x] 保留单表结构（包含 published_at、delivery_attempts 等字段）✅
  - [x] `catalog.inbox_events` 表（event_id 主键，含 processed_at 索引）✅ 2025-10-24
- [x] `catalog.video_projection` 读模型表 ✅ 2025-10-25
- [x] 更新 `sqlc/schema/catalog.sql` 与查询定义，生成 Outbox、Inbox 的类型安全查询（含 UPSERT）✅ 2025-10-24
- [x] 执行 `sqlc generate` 并修复生成差异；确保 `go build ./...` 编译通过 ✅ 2025-10-24

**生成的查询方法**:
- ✅ `InsertOutboxEvent` - 插入 Outbox 事件
- ✅ `ClaimPendingOutboxEvents` - 认领待发布事件 (FOR UPDATE SKIP LOCKED)
- ✅ `MarkOutboxEventPublished` - 标记事件已发布
- ✅ `RescheduleOutboxEvent` - 重新调度失败事件
- ✅ `InsertInboxEvent` - 插入 Inbox 事件 (ON CONFLICT DO NOTHING)
- ✅ `GetInboxEvent` - 查询 Inbox 事件
- ✅ `MarkInboxEventProcessed` - 标记事件已处理
- ✅ `RecordInboxEventError` - 记录处理错误
- ✅ `ListReadyVideosForTest` - 查询测试视图

## 阶段 2｜事件模型与契约 ✅
- [x] 在 `api/video/v1` 新增事件 proto（或单独 `api/events/catalog/v1`），定义 `VideoCreated/Updated/Deleted` 或 Envelope。✅ 2025-10-24
  - 创建了 `api/video/v1/events.proto`，定义了：
    - `EventType` 枚举（VIDEO_CREATED, VIDEO_UPDATED, VIDEO_DELETED）
    - `VideoCreated` 消息（包含 video_id, uploader_id, title, description, duration_micros, version, occurred_at, status, media_status, analysis_status）
    - `VideoUpdated` 消息（部分更新语义，只包含变更字段）
    - `VideoDeleted` 消息（包含 video_id, version, deleted_at, occurred_at, reason）
    - `Event` 通用信封（Envelope），使用 oneof payload 实现多态
- [x] 更新 `buf.yaml` / 模块依赖并运行 `make api`，通过 `buf lint`、`buf breaking`。✅ 2025-10-24
  - 成功生成 `api/video/v1/events.pb.go`（757 行代码）
  - 所有代码编译通过 `go build ./...`
- [x] 约定 Pub/Sub topic/ordering key/attributes（event_id, aggregate_id, version, schema_version, occurred_at）并在文档/配置中记录。✅ 2025-10-24
  - 创建了详细的 `docs/pubsub-conventions.md` 规范文档，包含：
    - Topic/Subscription 命名规范（catalog.video.events）
    - Message Attributes 必需/可选字段定义
    - Ordering Key 策略（使用 aggregate_id）
    - Message 格式（Protobuf Envelope）
    - 版本管理策略（schema_version）
    - 幂等性保证（Outbox/Inbox 模式）
    - 错误处理和重试策略（指数退避、DLQ）
    - 配置示例和监控告警规范

## 阶段 3｜写路径改造（Outbox）
- [x] 为 Service 添加事务边界：在写操作用例中引入 `TxManager`/Unit of Work，确保业务写 + Outbox INSERT 同事务提交。（2025-10-24，本次迭代已完成基础 Wiring，后续 Outbox 写入落地仍需跟进）
- [x] 抽象 `OutboxWriter` 接口与实现（`internal/repositories/outbox_repository.go`），复用 sqlc 查询并接受上下文。
- [x] 设计事件构造器（`internal/models/outbox_events`）：从 PO/领域对象生成 protobuf/JSON payload，封装 version 递增策略。
- [x] Service 层在关键状态变更（创建、更新、删除）后写入 Outbox，返回幂等响应（保留 event metadata 用于客户端一致性 token）。（Create + Update + Delete 全量接入，响应内附带 event_id/version/occurred_at）✅ 2025-10-25
- [x] 增加单元测试覆盖 Outbox 写入分支（含错误包装、version 规则）。

- [x] 认领逻辑（`FOR UPDATE SKIP LOCKED` + `lock_token` 租约控制）。
- [x] 发布器协程池与状态回写（完成 Publish().Get()、退避重试、释放租约）。
- [x] 指标与日志（发布成功/失败计数、积压长度、拉取批次）。✅ 2025-10-25
  - 发布器新增 OTel Counter/Histogram/Gauge，日志补充 `backlog_before/backlog_after`、重试计划与发布延迟。
- [x] 新增 `config_loader` → `gcpubsub.ProviderSet`、`OutboxPublisherConfig`，支持 emulator / 默认值。
- [x] 在 `cmd/grpc/wire.go` 将 OutboxPublisher 注入并随 Kratos 生命周期启动后台任务。
- [x] 编写集成测试或 e2e stub（可使用 Pub/Sub emulator）验证重复发布、退避策略。✅ 2025-10-25
  - `internal/repositories/test/outbox_repo_integration_test.go` 使用 Postgres Testcontainers 验证租约、重试与发布状态。
      - `internal/tasks/outbox/test/publisher_runner_integration_test.go` 结合 pstest + OTel 手动 meter 校验成功路径、指标与消息入站。

## 阶段 5｜StreamingPull 消费者与投影 ✅
- [x] 规划订阅命名 `<topic>.catalog-reader`，配置 DLQ、Ack deadline、Exactly-once（可选）。
- [x] 在 `internal/tasks/projection` 实现 StreamingPull handler：
  - [x] 反序列化事件，校验 schema_version、version 单调性。
  - [x] 数据库事务内执行 `Inbox INSERT ON CONFLICT DO NOTHING` 与投影 `UPSERT ... WHERE version < excluded.version`。
  - [x] 成功后 Ack；失败时 Nack 或返回错误（触发重投）；记录 DeliveryAttempt。
  - [x] metrics：处理耗时/失败次数、滞后版本（事件 version - 投影 version）。
- [x] 通过 Wire 将消费者注册为后台 goroutine；支持 graceful shutdown。
- [x] 添加消费者配置项：订阅 ID、MaxOutstandingMessages/Bytes、NumGoroutines、ExactlyOnce flag 等。
- [x] 编写集成测试验证幂等（重复消息、乱序版本、Exactly-once 配置、删除场景，自动跳过无 Docker 环境）。

## 阶段 6｜配置、部署与文档
- [x] 扩展 `configs/config.yaml`：
  - [x] `pubsub.project_id/topic_id/subscription_id/exactly_once` 等参数。
  - [x] Outbox 扫描批次、退避基础时长、最大尝试次数（默认值已落地，可按环境覆盖）。
  - [x] 投影消费者并发、Ack deadline、重试策略。
- [x] 更新 `internal/infrastructure/config_loader` 解析新配置，提供默认值与校验。
- [ ] Makefile 新增快捷命令（运行发布器/消费者、启动 emulator 文档链接）。
- [ ] 更新 README 或专属文档，补充运行手册、验证步骤、监控面板指标说明。

## 阶段 7｜观测、回放与运维
- [x] 在发布器/消费者中打 `log/slog` 结构化日志（event_id, aggregate_id, version, message_id）。
- [x] 导出 OTel 指标与 tracing span（Outbox publish、Projection apply）。
- [ ] 提供回放脚本：调用 Pub/Sub Seek 或扫描 Outbox 重新发布（文档化流程）。
- [ ] 设计告警阈值：Outbox 累积、订阅滞后、DLQ 消息计数、连续退避上限。

## 阶段 8｜测试矩阵与验收
- [ ] 单元测试覆盖：补齐 `lingo-utils/outbox` 发布器/仓储租约路径与 `inbox.Consumer` 成功/失败/重复消息分支，覆盖 Outbox 写入、发布器重试策略、消费者 UPSERT 幂等。
- [x] 集成测试：本地 Postgres + Pub/Sub emulator，验证端到端写读一致性（见 `internal/tasks/projection/test/projection_integration_test.go`）。
- [ ] 混沌演练：模拟发布器崩溃、Ack 失败、长事务、毒消息，确保恢复策略生效。
- [ ] 手工 QA checklist：API 写入→Outbox→Pub/Sub→Projection 数据与事件版本对齐。

## 阶段 9｜上线与回滚计划
- [ ] 部署顺序：
  1. 执行数据库迁移（添加 outbox / projection / inbox）。
  2. 发布新版本服务（含 Outbox 写入 + Publisher），先关闭消费者。
  3. 观察 Outbox 正常积压再开启 StreamingPull 消费者。
- [ ] 回滚策略：
  - 发布器/消费者均可独立关闭；主流程继续写入 Outbox（可后补发布）。
  - 如 schema 需回滚，先停消费者与发布器，导出未消费事件后再执行回退迁移。
- [ ] 数据修复手段：利用 Outbox 重新驱动，或 Seek 订阅回放到指定时间。

## 阶段 10｜文档与交付
- [ ] 更新 `docs/只读投影方案.md`（新增具体落地路径/命令示例）与 `投影一致性问题解决方案.md`（勾选完成项、补充经验）。
- [ ] 在服务 README 中添加“只读投影”章节：数据流图、配置示例、演练指南。
- [ ] 输出运维 Runbook（可链接至 wiki）：常见故障、DLQ 处理、回放步骤。
