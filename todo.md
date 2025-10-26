# Catalog 服务 MVP TODO Checklist (2025-10-26)

> 目标：根据《catalog-design.md》落地 Catalog 服务 MVP，并准备验收。所有任务按照“架构 → 数据 → 接口 → 业务 → 事件 → 读模型 → 非功能 → 验收”顺序推进。若任务已完成，请在前端勾选并附验证记录；部分任务依赖其他微服务协作时需在备注中标明负责人。

## 0. 预备检查
- [ ] 阅读并确认 `AGENTS.md`、`catalog-design.md`、`只读投影方案.md` 内容一致
- [ ] 与 Upload/Media/AI/Gateway 团队同步接口变更计划（Confluence+Slack）
- [ ] 更新 `services-catalog/README.md` 标题及简介为 Catalog 服务

## 1. 数据层建设
1. **Schema 迁移**
   - [ ] 新增 `migrations/005_create_catalog_idempotency_table.sql`
   - [ ] 新增 `migrations/006_create_catalog_video_audit_trail.sql`
   - [ ] 审核并补充 `catalog_read` schema（`video_projection` 扩展字段、`user_video_projection`、`projection_offsets`）
   - [ ] 运行全部迁移，使用 `psql` 验证表/索引/触发器存在
2. **sqlc 与模型**
   - [ ] 更新 `sqlc.yaml` 新增查询文件（幂等、审计、读库 UPSERT）
   - [ ] 生成 `internal/repositories/sqlc` 代码并通过 `go test ./internal/repositories/...`
   - [ ] 更新 `internal/models/po`、`vo` 与 `mappers`

## 2. gRPC 契约与配置
- [ ] 拆分 proto：`CatalogQueryService`、`CatalogLifecycleService`、事件定义；补充请求/响应/错误消息
- [ ] 调整 `buf.gen.yaml`/`buf.yaml` 并跑 `buf lint`、`buf breaking`
- [ ] 更新 `configs/conf.proto` 增加 `allowed_actor_types`、`metadata_keys` 注释
- [ ] 调整 `configs/config.yaml` 默认值（服务名、metadata 列表、JWT audience）

## 3. 控制器与基础设施
- [ ] 扩展 `BaseHandler`：条件请求 (`If-None-Match`)、`Idempotency-Key` 缓存挂钩
- [ ] 新增 Lifecycle/Query Controller，注册至 gRPC Server（`cmd/grpc/wire.go`）
- [ ] 在 `internal/infrastructure/configloader` 中提供 `IdempotencyStore`、`AuditRepo` 等 Provider
- [ ] 更新 `internal/infrastructure/grpc_server` 注释，确认 metadata 中间件仅透传白名单

## 4. 业务用例实现
1. **Lifecycle**
   - [ ] `RegisterUploadService`
   - [ ] `ProcessingStatusService`（含媒体/AI阶段通用逻辑、`job_id`/`emitted_at` 校验）
   - [ ] `MediaInfoService`
   - [ ] `AIAttributesService`
   - [ ] `VisibilityService`（发布/拒绝/override）
   - [ ] 写入 `video_audit_trail` + Outbox + 幂等存储
2. **Query**
   - [ ] 实现 `CatalogQueryService` 使用投影读取（含 Engagement 降级逻辑）
   - [ ] 支持 `ListUserPublicVideos`、`ListMyUploads` 分页与 ETag
   - [ ] 集成 `HandlerMetadata` 解析，按用户权限过滤

## 5. 事件构建与 Outbox Runner
- [ ] 扩展 `internal/models/outbox_events` 构造器，覆盖 `video.media_ready` 等事件 payload
- [ ] 在 Service 层构建事件并调用 `OutboxRepository.Enqueue`
- [ ] 调整 Outbox Runner（`internal/tasks/outbox`）批量参数、logging、metrics；新增 CLI `cmd/tasks/outbox`
- [ ] E2E 测试：创建视频→媒体/AI 回调→发布，确认 Pub/Sub 收到完整事件序列

## 6. 读模型 & Projection Runner
- [ ] 扩展 `internal/tasks/projection` 解码与 handler，支持所有事件类型
- [ ] 新增 `repositories.VideoProjectionRepository` 方法：UPSERT 扩展字段、删除、version 校验
- [ ] 实现用户态投影写入（`user_video_projection`）及 Engagement 事件订阅（占位，MVP 可 stub）
- [ ] 暴露投影指标 `projection_apply_*`、`projection_event_lag_ms`
- [ ] CLI `cmd/tasks/projection` 支持 `--replay-from` 参数

## 7. 幂等 & 审计组件
- [ ] 编写 `internal/repositories/idempotency_repo.go`、`audit_repo.go`
- [ ] 在 Service 调用流程中接入 `IdempotencyStore.Begin` / `Commit`
- [ ] 单测覆盖：
  - 重复 `RegisterUpload`
  - 媒体重复回调（不同 `emitted_at`）
  - 审计记录字段正确

## 8. 非功能性完善
- [ ] 日志：统一字段，新增审计/事件日志
- [ ] 指标：`catalog_lifecycle_duration_ms`, `catalog_outbox_lag_seconds`, `catalog_idempotency_hits_total`
- [ ] 追踪：在 controller/service 添加关键 span 属性
- [ ] 超时配置：全部外部调用使用 `context.WithTimeout`，并在 config 中可配置
- [ ] 安全：校验 `allowed_actor_types`，补充本地跳过说明

## 9. 文档与工具
- [ ] 更新 `services-catalog/README.md`（启动步骤、grpcurl 示例、观测指标）
- [ ] 在文档中记录 Outbox/Projection 运维手册
- [ ] 为集成测试编写脚本 `test/mvp_smoke.sh`
- [ ] 在 CI 中新增 `make generate`, `make lint`, `go test ./...` 流程检查

## 10. 验收前自检
- [ ] 执行 `make fmt && make lint && make test`
- [ ] 运行 `sqlc generate`, `buf generate`, `go generate ./...`
- [ ] 启动 `cmd/grpc`, `cmd/tasks/outbox`, `cmd/tasks/projection` 并通过 grpcurl 验证主要流程
- [ ] 验证幂等、状态冲突、投影滞后等异常路径
- [ ] 更新 `todo.md` 勾选完成项，提交验收报告

## 11. 交付清单
- [ ] 代码（仓库分支+PR 链接）
- [ ] 配置（config.yaml 示例）
- [ ] 数据迁移（SQL 脚本+执行记录）
- [ ] 运行手册（README/运维说明）
- [ ] 验收报告（含自测结果、指标截图）

---

> 若执行过程中发现设计缺口或跨团队依赖，请在事项备注并同步更新 `catalog-design.md`，确保文档与实现保持一致。
