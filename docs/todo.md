# 实现《只读投影方案》落地 TODO

## 现状基线
- `services/catalog` 当前仅提供视频查询 gRPC（`internal/controllers/video_handler.go` → `internal/services/video.go` → `internal/repositories/video_repo.go`），无事件发布与只读投影。
- 数据层只有主权 schema `catalog` 与 sqlc 查询；`internal/tasks` 仅保留占位文档，尚未实现 Outbox 发布或订阅消费。
- 配置（`configs/config.yaml`）未定义 Pub/Sub、Outbox、读模型等参数；Wire 仅组装 gRPC 与数据库。

## 阶段 0｜准备与基线验证
- [ ] 审核现有 `kratos-template/docs/只读投影方案.md`、`投影一致性问题解决方案.md` 与代码，明确契约与目录约束。
- [ ] 盘点依赖：确认本地 Pub/Sub 模拟器或 GCP 凭证、Supabase Schema 迁移流程、`sqlc`, `wire`, `buf` 版本一致性。
- [ ] 补充测试数据库 & Pub/Sub 启动脚本说明（若缺失则纳入 README/Troubleshooting 更新计划）。

## 阶段 1｜数据库与代码生成
- [ ] 新增迁移：
  - [ ] `catalog.app_outbox` 表（字段按照方案统一结构，含索引/约束/注释）。
  - [ ] `catalog_outbox_claims`（若选择分离状态表）或保留单表结构；评估 append-only 需求。
  - [ ] 只读 schema `catalog_read`，包含 `video_projection`、`inbox`（event_id 主键）以及必要索引（version、aggregate_id）。
- [ ] 更新 `kratos-template/sqlc/schema/*.sql` 与 `sqlc.yaml`，生成 Outbox、Projection、Inbox 的类型安全查询（含 UPSERT）。
- [ ] 执行 `make sqlc` 并修复生成差异；确保 `go test ./...` 继续通过。

## 阶段 2｜事件模型与契约
- [ ] 在 `api/video/v1` 新增事件 proto（或单独 `api/events/catalog/v1`），定义 `VideoCreated/Updated/Deleted` 或 Envelope。
- [ ] 更新 `buf.yaml` / 模块依赖并运行 `make api`，通过 `buf lint`、`buf breaking`。
- [ ] 约定 Pub/Sub topic/ordering key/attributes（event_id, aggregate_id, version, schema_version, occurred_at）并在文档/配置中记录。

## 阶段 3｜写路径改造（Outbox）
- [ ] 为 Service 添加事务边界：在写操作用例中引入 `TxManager`/Unit of Work，确保业务写 + Outbox INSERT 同事务提交。
- [ ] 抽象 `OutboxWriter` 接口与实现（`internal/repositories/outbox_repository.go`），复用 sqlc 查询并接受上下文。
- [ ] 设计事件构造器（`internal/models/events`）：从 PO/领域对象生成 protobuf/JSON payload，封装 version 递增策略。
- [ ] Service 层在关键状态变更（创建、更新、删除）后写入 Outbox，返回幂等响应（保留 event metadata 用于客户端一致性 token）。
- [ ] 增加单元测试覆盖 Outbox 写入分支（含错误包装、version 规则）。

## 阶段 4｜发布器任务
- [ ] 在 `internal/tasks/outbox_publisher` 实现：
  - [ ] 认领逻辑（`FOR UPDATE SKIP LOCKED` + `lock_token` 字段）。
  - [ ] 发布器协程池，等待 `Publish().Get()` 完成后标记 `published_at`，失败退避调整 `next_retry_at`。
  - [ ] 指标与日志（发布成功/失败计数、积压长度、拉取批次）。
- [ ] 新增 `internal/infrastructure/pubsub` 初始化 Pub/Sub Publisher 客户端，提供 Wire Provider（支持本地 emulator）。
- [ ] 在 `cmd/grpc/wire.go` 将 OutboxPublisher 注入并在启动时启动后台任务，响应 `context.Context` 取消。
- [ ] 编写集成测试或 e2e stub（可使用 Pub/Sub emulator）验证重复发布、退避策略。

## 阶段 5｜StreamingPull 消费者与投影
- [ ] 规划订阅命名 `<topic>.catalog-reader`，配置 DLQ、Ack deadline、Exactly-once（可选）。
- [ ] 在 `internal/tasks/projection_consumer` 实现 StreamingPull handler：
  - [ ] 反序列化事件，校验 schema_version、version 单调性。
  - [ ] 数据库事务内执行 `Inbox INSERT ON CONFLICT DO NOTHING` 与投影 `UPSERT ... WHERE version < excluded.version`。
  - [ ] 成功后 Ack；失败时 Nack 或不 Ack（让 Pub/Sub 重投）；记录 DeliveryAttempt。
  - [ ] metrics：处理耗时、nack 次数、滞后版本（事件 version - 投影 version）。
- [ ] 通过 Wire 将消费者注册为后台 goroutine；支持 graceful shutdown。
- [ ] 添加配置项：订阅 ID、MaxOutstandingMessages/Bytes、NumGoroutines、ExactlyOnce flag。
- [ ] 编写单元/集成测试验证幂等（重复消息、乱序版本、毒消息进入 DLQ）。

## 阶段 6｜配置、部署与文档
- [ ] 扩展 `configs/config.yaml`：
  - [ ] `pubsub.project_id/topic_id/subscription_id/exactly_once` 等参数。
  - [ ] Outbox 扫描批次、退避基础时长、最大尝试次数。
  - [ ] 投影消费者并发、Ack deadline、重试策略。
- [ ] 更新 `internal/infrastructure/config_loader` 解析新配置，提供默认值与校验。
- [ ] Makefile 新增快捷命令（运行发布器/消费者、启动 emulator 文档链接）。
- [ ] 更新 README 或专属文档，补充运行手册、验证步骤、监控面板指标说明。

## 阶段 7｜观测、回放与运维
- [ ] 在发布器/消费者中打 `log/slog` 结构化日志（event_id, aggregate_id, version, message_id）。
- [ ] 导出 OTel 指标与 tracing span（Outbox publish、Projection apply）。
- [ ] 提供回放脚本：调用 Pub/Sub Seek 或扫描 Outbox 重新发布（文档化流程）。
- [ ] 设计告警阈值：Outbox 累积、订阅滞后、DLQ 消息计数、连续退避上限。

## 阶段 8｜测试矩阵与验收
- [ ] 单元测试覆盖：Outbox 写入、发布器重试策略、消费者 UPSERT 幂等。
- [ ] 集成测试：本地 Postgres + Pub/Sub emulator，验证端到端写读一致性。
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

