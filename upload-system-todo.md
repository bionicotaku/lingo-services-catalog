下面列出“Catalog 上传系统重构”全过程的可执行待办列表。每个阶段都包含可打勾的子任务（`[ ]`），完成后请即时更新文件为 `[x]`。除非显式标注“可选”，其余条目均为上线前必须完成的工作。

---

## ✅ 上线完成判定（Definition of Done）

- [ ] 移动端仅需一次 InitResumableUpload → GCS 直传即可完成上传；Catalog 无需代理字节流。
- [ ] `(user_id, content_md5)` 在任意状态下保持唯一；重复上传被拒绝或复用单一会话。
- [ ] OBJECT_FINALIZE 回调能够在单事务内：幂等落库 uploads/videos、生成 outbox 事件。
- [ ] 所有代码通过 `make lint`、`go test ./...`；文档（upload-system.md、ARCHITECTURE.md 等）与实现一致。
- [ ] 端到端验证脚本覆盖“并发 Init、续传、重复事件、MD5 不一致、回调失败重试”等关键场景。

---

## 阶段 0 ｜准备与现状确认

- [ ] 新建工作分支（示例：`feature/catalog-upload-system`），并在 `services-catalog/todo.md` 建立对应 Epic/子项。
- [ ] 按 README 启动现有 Catalog gRPC 服务与 Outbox Runner，确认 001~004 迁移已执行。
- [ ] 记录基准状态：`git status`、现有测试结果，便于后续回归对比。

---

## 阶段 1 ｜数据层扩展（migrations + sqlc）

- [x] 创建 `migrations/005_create_catalog_uploads.sql`，字段与约束要求：
  - 基础信息：`video_id uuid primary key`、`user_id uuid not null`、`bucket text not null`、`object_name text not null`（约定 `raw_videos/{user_id}/{video_id}`）。
  - 元数据：`content_type text`、`title text not null`、`description text not null`、`expected_size bigint not null default 0`、`size_bytes bigint not null default 0`。
  - 约束字段：`content_md5 char(32) not null`、`status text check (status in ('uploading','completed','failed')) not null`。
  - 签名 & 校验：`signed_url text`、`signed_url_expires_at timestamptz`、`gcs_generation text`、`gcs_etag text`、`md5_hash text`、`crc32c text`、`error_code text`、`error_message text`。
  - 审计：`created_at timestamptz default now()`、`updated_at timestamptz default now()` + 触发器。
  - 唯一索引：`(user_id, content_md5)` 与 `(bucket, object_name)`。
- [x] 若仓库尚无 Inbox 表（按 002 迁移确认），补充 `006_create_catalog_inbox_events.sql` 并在文档说明。（现有 002 迁移已包含 inbox/outbox 表，确认无需新增）
- [x] 在 `sqlc/schema/` 新增或更新 `003_catalog_uploads.sql`，与迁移保持一致。
- [x] 增加 `sqlc/queries/uploads.sql`，至少包含：`UpsertUpload`、`GetUploadByVideoID`、`GetUploadByUserMd5`、`MarkUploadCompleted`、`MarkUploadFailed`、`ListExpiredUploads`（可选）。
- [x] 运行 `sqlc generate` 并确认生成代码无误。
- [ ] 本阶段 DoD：迁移可成功执行（`psql -f` 或 `go run ./cmd/migrate`），sqlc 生成无报错。

---

## 阶段 2 ｜Proto 契约

- [x] 更新/新增 `api/video/v1/upload.proto`：
  - `InitResumableUploadRequest` 仅保留：`size_bytes`、`content_type`、`content_md5_hex`、`duration_seconds`、`title`、`description`（所有字段为必填，添加 buf.validate 约束）。
  - `InitResumableUploadResponse` 仅返回：`video_id`、`resumable_init_url`、`expires_at_unixms`。
- [x] 清除旧的 `filename`、`video_id`（请求端）或 `bucket/object_name/already_uploaded` 等字段。
- [x] 运行 `buf generate`（或 `make proto`）并确保 gRPC stub 更新。
- [x] 更新 `cmd/grpc/wire.go` / `wire_gen.go` 中的服务注册引用（如有 proto 包名或接口变动）。
- [x] 本阶段 DoD：`go test ./api/...` 编译通过，生成代码没有遗留冲突。

---

## 阶段 3 ｜配置与基础设施

- [x] 在 `configs/conf.proto`、`configs/config.yaml` 中加入/确认以下字段：
  - `gcs.project_id`、`gcs.bucket`、`gcs.signer_service_account`、`gcs.signed_url_ttl_seconds`。
  - `pubsub.notification_topic`、`pubsub.subscription_id`、`pubsub.receive.*`。
- [x] 新建 `internal/infrastructure/gcs/signer.go`（若已存在则扩展）：提供 `SignedResumableInitURL(ctx, bucket, objectName, contentType, ttl)`，签名 headers 包含 `x-goog-resumable:start`、`x-upload-content-type`、`x-goog-if-generation-match:0`。
- [x] 在 Wire 图中注入 signer，并在服务启动/任务启动入口加载配置。
- [x] 本阶段 DoD：手动调用 signer 返回的 URL 可成功发起 Resumable 会话并得到 Session URI（新增单元测试覆盖签名头与 TTL）。

---

## 阶段 4 ｜仓储与服务层实现

- [x] 新增 `internal/repositories/upload_repository.go`：封装 sqlc 生成的查询，提供接口：
  - `Upsert(ctx, params) (UploadRecord, created bool, error)`。
  - `GetByVideoID(ctx, videoID)`、`GetByUserMd5(ctx, userID, md5)`。
  - `MarkCompleted(ctx, params)`（写入 size/hash/generation 等）。
  - `MarkFailed(ctx, params)`（写入错误码/信息）。
  - 可选：`ListExpired(ctx, now)` 用于后续 Reaper。
- [x] 新增 `internal/services/upload_service.go`（或并入 LifecycleService 子模块）：
  - 校验 metadata 中的 `user_id`、白名单 `content_type`、`duration_seconds <= 300`、`size_bytes` 上限。
  - 始终生成新的 `video_id`；`object_name = fmt.Sprintf("raw_videos/%s/%s", userID, videoID)`。
  - 调用 `Upsert`：若 `created==false && status=='uploading'` → 覆盖最新元数据并返回原签名；若 `status=='completed'` → 返回业务错误（重复资源）。
  - 调 signer 得最新 `resumable_init_url` 与过期时间；必要时刷新并回写 uploads 表。
  - Response 只包含 `video_id`、`resumable_init_url`、`expires_at`。
- [x] 本阶段 DoD：服务层单元测试覆盖“首次创建”、“重复调用复用”、“完成后拒绝”、“签名刷新失败”等分支。

---

## 阶段 5 ｜接口适配（Controller / Wire）

- [x] 新增 `internal/controllers/upload_handler.go`（或命名为 `upload_grpc.go`）：
  - 从 context metadata 解析 user id，调用 Service，按统一错误规范返回 Problem Details。
  - 将业务错误（重复资源等）映射为 `FailedPrecondition` 或 `AlreadyExists`。
- [x] 更新 `internal/controllers/init.go`、`cmd/grpc/main.go`、`cmd/grpc/wire.go`/`wire_gen.go`，确保 UploadService 完成注册。
- [x] 为 handler 编写最小单元测试（metadata 缺失、校验失败、成功路径）。

---

## 阶段 6 ｜回调 Runner（StreamingPull + Inbox）

- [ ] 创建 `internal/tasks/uploads/{decoder.go,handler.go,runner.go}`：
  - Decoder：仅处理 `eventType == OBJECT_FINALIZE`，解析 message attributes/data。
  - Handler：在事务内执行 Inbox 去重 → 校验 md5 → 更新 uploads → 创建/更新 videos → 写 outbox → 标记 inbox processed。
  - 保证幂等：重复 generation / 已完成上传不再重复写主表。
- [ ] 新增 `cmd/tasks/uploads/main.go`，注入 Pub/Sub subscriber、logger、metrics、handler。
- [ ] 在 Wire 中绑定 Runner，复用现有 Outbox/Inbox 基础设施。
- [ ] 本阶段 DoD：本地启动 Runner（可指向 pstest 或真实 Pub/Sub），能消费模拟消息并更新数据库。

---

## 阶段 7 ｜测试矩阵

- [ ] 单元测试：
  - Service 层：覆盖输入校验、复用逻辑、重复资源拒绝、签名刷新异常。
  - Repository 层：使用 testcontainers/Postgres 验证 Upsert/Completed/Failed 行为及约束。
- [ ] 集成测试：
  - `InitResumableUpload` → Mock signer 返回固定 URL，验证响应及仓储记录。
  - `OBJECT_FINALIZE` → 使用 pstest + testcontainers 模拟消息，验证 uploads → videos → outbox 全链路（包括重复消息、MD5 mismatch）。
  - 并发场景：多 goroutine 同时调用 Init，确保只生成一个 video_id。
- [ ] CI 覆盖：`make lint`、`go test ./...`、`buf lint`、`sqlc generate`（若集成在 make 目标）。

---

## 阶段 8 ｜文档与配置同步

- [ ] 更新 `services-catalog/upload-system.md`（已完成的内容需再次核对与实现一致）。
- [ ] 更新 `services-catalog/ARCHITECTURE.md`、`README.md`、`MVP.md` 中关于上传流程、接口、事件的描述。
- [ ] 在 `docs/` 或 `infra` 目录补充 Pub/Sub/GCS 开通脚本、权限说明（若尚未存在）。
- [ ] 将新的配置项加入 Sample `.env` 或配置示例。
- [ ] 更新 `services-catalog/todo.md` 任务状态，使其与实际工作同步。

---

## 阶段 9 ｜端到端演练与回归

- [ ] 编写或更新端到端脚本：模拟客户端计算 MD5 → 调 Init → 使用 Session URI 上传（可用本地文件/假文件）→ 假触发回调。
- [ ] 验证并发 10 次 Init：仅 1 条 uploads 记录，自增字段正确。
- [ ] 验证会话过期：模拟签名过期后重新 Init，旧 Session URI 上传成功被 412 拒绝。
- [ ] 验证回调重复：同一消息投递 3 次仅第一次生效。
- [ ] 验证 MD5 不一致：构造错误 MD5，Runner 将记录标记为 failed 并写 error_code。
- [ ] 重新执行 `make lint && go test ./...`，确认无回归；记录输出供交付复核。

---

## 阶段 10 ｜收尾

- [ ] 更新 `CHANGELOG` 或发布记录（若项目要求）。
- [ ] 与前端 / 移动端 / 网关团队同步新 API 契约及错误语义。
- [ ] 评估是否需要 Reaper 任务（清理长时间 uploading 的记录），若需要则在后续迭代排期。
- [ ] 合并前进行最终代码审查，确认没有未使用文件或调试日志。
- [ ] 准备回滚方案：保留旧接口实现的紧急开关（若需要），并记录数据库回滚指引。

> 完成以上所有勾选项，方可认为“Catalog 上传系统”重构落地。
