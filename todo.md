# Catalog 服务 MVP TODO Checklist (2025-10-26)

> 目标：根据《MVP.md》落地 Catalog 服务 MVP，并准备验收。所有任务按照“架构 → 数据 → 接口 → 业务 → 事件 → 读模型 → 非功能 → 验收”顺序推进。若任务已完成，请在前端勾选并附验证记录；部分任务依赖其他微服务协作时需在备注中标明负责人。

## 0. 预备检查
- [ ] 阅读并确认 `AGENTS.md`、`MVP.md` 内容一致（`只读投影方案.md` 已归档）
- [ ] 与 Upload/Media/AI/Gateway 团队同步接口变更计划（Confluence+Slack）
- [ ] 更新 `services-catalog/README.md` 标题及简介为 Catalog 服务

## 1. 数据层建设
1. **Schema 迁移** *(进行中)*
  - [x] 新增 `migrations/004_create_catalog_video_user_states.sql`
   - [ ] 运行全部迁移，使用 `psql` 验证表/索引/触发器存在
2. **sqlc 与模型**
   - [x] 更新 `sqlc.yaml` 新增查询文件（视频主表、`video_user_states`）
   - [x] 生成 `internal/repositories/sqlc` 代码并通过 `go test ./internal/repositories/...`
   - [x] 更新 `internal/models/po`、`vo` 与 `mappers`

## 2. gRPC 契约与配置
- [x] 拆分 proto：`CatalogQueryService`、`CatalogLifecycleService`、事件定义；补充请求/响应/错误消息（2025-10-27：完成拆分并为 Lifecycle RPC 增加独立响应消息；handler/service 已切换至新 proto）
- [x] 调整 `buf.gen.yaml`/`buf.yaml` 并跑 `buf lint`、`buf breaking`
- [ ] 更新 `configs/conf.proto` 增加 `allowed_actor_types`、`metadata_keys` 注释
- [ ] 调整 `configs/config.yaml` 默认值（服务名、metadata 列表、JWT audience）

## 3. 业务用例实现
1. **Lifecycle** *(进行中)*
   - [x] `RegisterUploadService`
   - [x] `ProcessingStatusService`（含媒体/AI阶段通用逻辑、`job_id`/`emitted_at` 校验）
   - [x] `MediaInfoService`
   - [x] `AIAttributesService`
   - [x] `VisibilityService`（发布/拒绝/override）
   - [x] 输出领域事件并写入 Outbox
     - [x] 2025-10-26：扩展 proto/outbox 以覆盖 `catalog.video.media_ready`、`catalog.video.ai_enriched`、`catalog.video.processing_failed`、`catalog.video.visibility_changed` 并在 Service 层落地（已完成，make lint && make test）
2. **Query**
 - [x] 实现 `CatalogQueryService` 基于主表读取（含 Engagement 降级逻辑）
  - [x] 支持 `ListUserPublicVideos`、`ListMyUploads` 分页与 ETag
  - [x] 集成 `HandlerMetadata` 解析，按用户权限过滤（2025-10-26：服务层从 Context 读取 metadata 校验 user_id，匿名/非法请求统一处理）
  - [x] 2025-10-26：配置 `metadata_keys` 补充 `x-md-actor-type`/`x-md-actor-id` 并在文档中解释来源与用途（已更新 config.yaml、conf.proto、加载器测试）
  - [x] 2025-10-27：补齐仓储集成测试覆盖分页与过滤（`ListPublicVideos`、`ListUserUploads`、`GetMetadata`），并通过 `make lint` + `go test ./...`

## 4. 事件构建与 Outbox Runner
- [x] 扩展 `internal/models/outbox_events` 构造器，覆盖 `video.media_ready` 等事件 payload（2025-10-26 完成）
- [x] 在 Service 层构建事件并调用 `OutboxRepository.Enqueue`（2025-10-26 完成）
  - [x] 调整 Outbox Runner（`internal/tasks/outbox`）批量参数、logging、metrics；新增 CLI `cmd/tasks/outbox`（2025-10-26：尊重 logging/metrics 开关、输出配置日志，新增独立 Runner CLI）
- [x] E2E 测试：创建视频→媒体/AI 回调→发布，确认 Pub/Sub 收到完整事件序列
  - [x] 2025-10-27：完成，使用服务层用例驱动 Outbox Runner+pstest 验证 11 个领域事件按序发布

## 5. Engagement 投影 Runner
- [x] 新增 `internal/tasks/engagement`（或同目录）消费 Engagement 事件
  - [x] 2025-10-27：定义 Runner 结构（Pub/Sub Subscriber + ack/retry），支持 graceful shutdown
- [ ] 编写 `repositories.VideoUserStatesRepository`，提供 UPSERT/删除能力
  - [x] 已具备 Upsert/Get/Delete；Runner 需复用并补齐事务/metrics 包装
- [x] 定义事件解码结构，将 liked/bookmarked/watched 写入 `catalog.video_user_states`
  - [x] 2025-10-27：创建 `engagement.Event` 解码器（兼容 JSON/Proto，含版本校验）
- [x] 暴露指标 `catalog_engagement_apply_*`, `catalog_engagement_event_lag_ms`
  - [x] 2025-10-27：实现成功/失败计数器、滞后直方图；整合 OTEL Meter
- [ ] 提供回放/偏移管理方案（可选：测试实现内存 offset） *(Post-MVP 延后)*
  - [ ] 2025-10-28：offsetProvider 设计/实现移至 Post-MVP（接口 + Postgres provider + Runner 集成 + 基础测试）
- [x] 新增 CLI `cmd/tasks/engagement` 并在 README 记录启动方式
- [x] 集成测试：`internal/tasks/engagement/test/runner_integration_test.go`（模拟 Pub/Sub + PG，覆盖重复/异常场景）
  - [x] 2025-10-27：完成，使用自定义 Subscriber/TxManager 仿真重复、无效载荷与事务失败路径，覆盖 proto/json 消费

## 6. 非功能性完善
- [ ] 日志：统一字段，新增事件日志
- [ ] 指标：`catalog_lifecycle_duration_ms`, `catalog_outbox_lag_seconds`
- [ ] 追踪：在 controller/service 添加关键 span 属性
- [ ] 超时配置：全部外部调用使用 `context.WithTimeout`，并在 config 中可配置
- [ ] 安全：校验 `allowed_actor_types`，补充本地跳过说明

## 7. 文档与工具
- [ ] 更新 `services-catalog/README.md`（启动步骤、grpcurl 示例、观测指标）
- [ ] 在文档中记录 Outbox/Projection 运维手册
- [ ] 为集成测试编写脚本 `test/mvp_smoke.sh`
- [ ] 在 CI 中新增 `make generate`, `make lint`, `go test ./...` 流程检查

## 8. 验收前自检
- [ ] 执行 `make fmt && make lint && make test`
- [ ] 运行 `sqlc generate`, `buf generate`, `go generate ./...`
- [ ] 启动 `cmd/grpc`, `cmd/tasks/outbox`, `cmd/tasks/engagement` 并通过 grpcurl 验证主要流程
- [ ] 验证状态冲突、投影滞后等异常路径
- [ ] 更新 `todo.md` 勾选完成项，提交验收报告

## 9. 交付清单
- [ ] 代码（仓库分支+PR 链接）
- [ ] 配置（config.yaml 示例）
- [ ] 数据迁移（SQL 脚本+执行记录）
- [ ] 运行手册（README/运维说明）
- [ ] 验收报告（含自测结果、指标截图）

---

> 若执行过程中发现设计缺口或跨团队依赖，请在事项备注并同步更新 `MVP.md`，确保文档与实现保持一致。

---

## Post-MVP Backlog（幂等、审计、基础设施）
- [ ] 设计并创建 `catalog.idempotency_keys` 表及 TTL 清理策略
- [ ] 实现 `IdempotencyStore` 仓储与 BaseHandler 集成
- [ ] 为写接口补充幂等回放逻辑与单测（重复调用、媒体回调去重）
- [ ] 引入指标 `catalog_idempotency_hits_total`、`catalog_idempotency_conflicts_total`
- [ ] 设计并创建 `catalog.video_audit_trail` 与相关仓储、指标
- [ ] 评估 BaseHandler、configloader Provider 等控制器/基础设施抽象统一方案
- [ ] Engagement offsetProvider：持久化消费位点、回放/多实例策略（承接 TODO §5）
