下面给出一份**面向你当前仓库结构**（api/, cmd/, internal/, migrations/, sqlc/ 等） 的**分阶段重构 TODO 清单**，按**最小可行增量**推进，最终达到“**移动端直传 GCS，Catalog 微服务自主管理上传**（MD5 强唯一 + 回调幂等 + 事件驱动）”的**验收目标**。

文内所有文件/目录命名均与仓库根目录显示一致（例如 api/video/v1, migrations/_, internal/_, cmd/\* 等） 。你现有的 README/架构说明与迁移脚本编号也已被纳入考虑（001~004 迁移、cmd/tasks/outbox 等）。

---

## **目标基线（达成后即视为“上线合格”）**

- **仅移动端**直传 GCS（**无需 CORS**）。
- Catalog 以 **(user_id, content_md5)** **全状态唯一**约束，确保**同一用户同一文件只能产生 1 条视频**，对象命名 videos/{user_id}/{content_md5}。
- 以 **V4 Signed URL (XML API +** **x-goog-resumable:start\*\***)** 仅用于**发起会话**；后续分片走 session URI；签名中强制 **ifGenerationMatch=0\*\* 防覆盖。
- GCS → Pub/Sub **OBJECT_FINALIZE** → **Push + OIDC** 调用 /\_/gcs/callback；服务端**幂等**推进 uploads 与 videos，并写 **Outbox** 事件。
- 分片/断点遵循 GCS **Resumable Upload** 规范（308 Resume Incomplete + Range），移动端遵循 256 KiB 的整数倍（推荐 ≥8 MiB）。

---

# **阶段 0 ｜准备与基线对齐**

**0-1. 创建长支与议题看板**

- 新建分支 feature/upload-system-gcs。
- 在 todo.md 建立 Epic 项，分解本文阶段任务为 Issues（带 Owner/优先级/验收标准）。

**0-2. 本地可运行基线**

- 依 README 启动 gRPC 进程与（如有）Outbox 任务进程，完成 001~004 迁移演练。
- **DoD**：go run ./cmd/grpc -conf configs/config.yaml 能启动；cmd/tasks/outbox 可运行（若存在）。

---

# **阶段 1 ｜数据层与迁移**

**1-1. 新增迁移脚本 migrations/005_create_catalog_uploads.sql**

内容要点：

- 表 catalog.uploads：

  - upload_id uuid pk default gen_random_uuid()
  - user_id uuid not null
  - video_id uuid（可空；先上传后建视频或反之）
  - bucket text not null
  - object_name text not null（例如 videos/{user_id}/{content_md5}）
  - original_filename text not null
  - content_type text not null
  - expected_size bigint not null default 0、size_bytes bigint not null default 0
  - content_md5 char(32) not null（**hex 32**）
  - status text check in ('pending','uploading','completed','failed') not null
  - gcs_generation text, gcs_etag text, md5_hash text, crc32c text
  - error_code text, error_message text
  - created_at/updated_at timestamptz default now()

- **唯一索引**：

  - unique (user_id, content_md5)（**全状态唯一**，强制同一用户同一内容仅一条记录）
  - unique (bucket, object_name)（对象路径唯一）

- 触发器：updated_at 自动触达。

- 若仓库已有 catalog.inbox*events/catalog.outbox_events（见 002*\*），保持不变；若**不存在**，本阶段**另起** **006\_\*** 增加 Inbox 表（source, dedup_key 主键）。

**1-2. SQLC 配置与查询**

- 在 sqlc.yaml 中添加新查询路径，例如：sqlc/queries/uploads.sql。

- 新增查询（建议最少集）：

  - InsertUpload(user_id, content_md5, bucket, object_name, original_filename, content_type, expected_size, status='pending')

    - 用 **ON CONFLICT (user_id, content_md5) DO NOTHING**；随后 SELECT 返回现存行，实现**会话合并**。

  - GetUploadByUserMd5(user_id, content_md5)

  - UpdateUploadCompleted(upload_id, size_bytes, md5_hash, crc32c, gcs_generation, gcs_etag)（**where status != ‘completed’**）

  - BindUploadToVideo(upload_id, video_id)（可选）

- **DoD**：make sqlc 生成通过；执行 005\_\* 迁移成功（对照 README 的 001~004）。

---

# **阶段 2 ｜ Proto 与服务契约**

**2-1. 新增 api/video/v1/upload.proto**

```
service UploadService {
  rpc InitResumableUpload(InitResumableUploadRequest) returns (InitResumableUploadResponse);
  rpc GetUpload(GetUploadRequest) returns (GetUploadResponse);
}
message InitResumableUploadRequest {
  string filename = 1;
  int64  size_bytes = 2;
  string content_type = 3;
  string content_md5_hex = 4;  // 移动端先算好
  int32  duration_seconds = 5; // 可选，要求 <= 300
  string video_id = 6;         // 可选
}
message InitResumableUploadResponse {
  string upload_id = 1;
  string bucket = 2;
  string object_name = 3;        // videos/{user}/{md5}
  string resumable_init_url = 4; // V4 signed URL for POST + x-goog-resumable:start
  int64  expires_at_unixms = 5;  // ~15min
  bool   already_completed = 6;  // 若该 md5 已完成
}
message GetUploadRequest  { string upload_id = 1; }
message GetUploadResponse {
  string status = 1; string bucket = 2; string object_name = 3;
  string gcs_etag = 4; string gcs_generation = 5;
  int64  size_bytes = 6; string md5_hash_hex = 7;
}
```

- 更新 buf.yaml / buf.gen.yaml 并执行 make proto。
- **DoD**：生成的 Go 代码编译通过。

---

# **阶段 3 ｜ GCS 集成（签名）与配置**

**3-1. 配置结构**

- 在 configs/config.yaml/conf.proto 新增：

```
gcs:
  project_id: your-project
  bucket: your-bucket
  signer_service_account: upload-signer@your-project.iam.gserviceaccount.com
  signed_url_ttl_seconds: 900
  callback_audience: "https://<catalog-domain>/_/gcs/callback"
server:
  grpc_addr: "0.0.0.0:9000"
  http_addr: "0.0.0.0:8000"  # 新增：接收 Pub/Sub Push
pubsub:
  project_id: your-project
  notification_topic: "video-uploads"
  callback_path: "/_/gcs/callback"
```

- （网关 HTTP→gRPC + JWT → metadata 注入 user_id 已就绪，本文仅使用，不展开）

**3-2. 依赖与封装**

- go.mod 加入：cloud.google.com/go/storage（用于 V4 签名），google.golang.org/api/iamcredentials/v1（可选，用 IAM SignBlob）。

- 新增 internal/infrastructure/gcs/signer.go：

  - 暴露 SignedResumableInitURL(bucket, object, contentType, ttl, withIfGenerationMatch0 bool)。
  - **签名必须包含头**：x-goog-resumable:start、x-upload-content-type:<mime>；并将 **ifGenerationMatch: 0** 作为**签名条件**（防覆盖）。

- **DoD**：本地调用 SignedResumableInitURL 能返回可用 URL（可用 curl 发起会话，得到 Location）。

---

# **阶段 4 ｜仓储层与领域服务**

**4-1. Repository**

- 新增 internal/repositories/upload_repo.go，封装阶段 1 的 SQLC 查询：

  - InsertOrGetByUserMd5(...) (row, inserted bool)（处理 ON CONFLICT + SELECT）
  - MarkCompleted(...)（条件更新）
  - GetByUploadID(...)，BindUploadToVideo(...)

**4-2. Service（领域逻辑）**

- 新增 internal/services/upload_service.go：

  - InitResumableUpload(ctx, req)

    1. 从 metadata 取 user_id；校验 duration<=300、content_type 白名单、size_bytes 上限；
    2. 生成 object_name = "videos/{user_id}/{content_md5}"；
    3. InsertOrGetByUserMd5 写入/复用 uploads（status='pending'）；
    4. 生成 **V4 Signed URL（POST）**，**带** x-goog-resumable:start 与 **ifGenerationMatch=0**；
    5. 若该记录已 completed → already_completed=true；
    6. 返回响应。

  - GetUpload(ctx, upload_id)：直读仓储。

- **DoD**：并发 10 个相同 (user_id, md5) 调用仅返回**同一条**记录与对象名；不同 md5 互不影响。

---

# **阶段 5 ｜接口适配与回调入口**

**5-1. gRPC Handler**

- 新增 internal/controllers/upload_grpc.go，注册 UploadService 到服务器，复用现有依赖注入（Wire）与 metadata 解析（README 已说明 header/metadata 规范）。

**5-2. HTTP 回调（Pub/Sub Push + OIDC）**

- 新增轻量 HTTP 服务（若 cmd/grpc 已兼容多 server，则在同进程启动 HTTP）与 /\_/gcs/callback：

  - 校验 Authorization: Bearer <JWT> 的 **issuer/audience/exp**（OIDC）并验签。

  - 解析 Pub/Sub Push JSON（message.attributes.eventType == OBJECT_FINALIZE 时处理；message.data base64 的对象 JSON）。

  - **Inbox 去重**（若已存在 catalog.inbox*events，用 source='gcs', dedup_key='{bucket}/{name}#{generation}' 作为主键；若无，则创建 006*\* 迁移加上）。

  - 单事务**幂等**推进：

    - 校验 md5Hash（base64→hex）与 uploads.content_md5 一致；否则 failed(MD5_MISMATCH)；
    - uploads.status -> completed 并回填 size/hash/etag/generation（仅当当前非 completed）；
    - videos.raw_file_reference="gs://{bucket}/{object_name}"；status: pending_upload -> processing；
    - 写 **Outbox**：video.upload.completed。

  - 返回 **2xx** 即 ACK；否则让 Pub/Sub 重试（至少一次）。

- **DoD**：重复推送同一 generation 仅首次推进；后续无副作用（幂等）。

---

# **阶段 6 ｜移动端集成规范与冒烟脚本**

**6-1. 文档**

- 在 docs/ 或 upload-system.md（重写/替换）写清移动端调用与分片规范：

  - 先算 **MD5(hex 32)**；
  - 调 InitResumableUpload 获取 resumable_init_url；
  - POST（**必须**带 x-goog-resumable:start 与 x-upload-content-type）发起会话，取 **Location** 为 session URI；
  - 以 **256 KiB 整倍数**分片 PUT，处理 **308 Resume Incomplete** 与 Range，断点用 Content-Range: bytes \*/TOTAL；推荐块 ≥ 8 MiB。
  - **无需 CORS**（移动端直传）。

- **6-2. 本地/CI 冒烟**

  - 使用 curl 脚本模拟：**发起会话** → **分片上传**（2~3 块）→ **回调处理**（可用 replay 工具手动 POST 回调负载）。

- **DoD**：脚本一次跑通；GetUpload 返回 completed 且视频状态进入 processing。

---

# **阶段 7 ｜可观测性与告警**

**7-1. 指标**

- upload.init.count/latency、upload.session.expired.count（会话超期）
- upload.chunk.resume.count（308）、upload.offset.mismatch.count
- 回调处理耗时与重试次数、md5.mismatch.count、if_generation_match.violation.count

**7-2. 日志关联**

- 打印关键字段：upload_id、user_id、object_name、pubsub_message_id、gcs_generation。
- **DoD**：Grafana/日志平台可按 upload_id 串联完整链路。

---

# **阶段 8 ｜ DevOps：GCS & Pub/Sub 配置（一次性）**

> 可写成 scripts/gcloud/setup_uploads.sh 自动化脚本。

1. **Bucket 通知 → Pub/Sub**（仅 OBJECT_FINALIZE，可设置 --object-name-prefix=videos/）

2. **Push 订阅（OIDC）**：

   - --push-endpoint=https://<catalog-domain>/\_/gcs/callback
   - --push-auth-service-account=<sa>@<project>.iam.gserviceaccount.com
   - --push-auth-token-audience=https://<catalog-domain>/\_/gcs/callback
   - 订阅的服务账号授予最小 IAM；Catalog 端严格校验 JWT。

3. **注意**：**无需 CORS**（移动端-only）。

- **DoD**：上传完成后 3~5 秒内可见回调命中；订阅健康（无堆积）。

---

# **阶段 9 ｜回归与验收**

**9-1. 并发/覆盖**

- 并发 10× InitResumableUpload（同 user、同 md5）→ 仅 1 条 uploads 记录，余者复用；对象名一致。
- 启动两个独立会话同时上传同一对象 → **仅一个**成功；另一端在最终提交阶段因 **ifGenerationMatch=0** 失败（412）。

**9-2. 断点/续传**

- 上传至 60% 断开 → 通过 PUT 0 字节 + Content-Range: bytes \*/TOTAL 恢复，直至完成；看到 308 与 Range 正常返回。

**9-3. 回调幂等**

- 重放同一 generation 的回调 3 次 → 仅首次推进，后续 204。

**9-4. MD5 对账**

- 构造错误 md5 请求 → 回调时标记 failed(MD5_MISMATCH) 并告警；不推进视频状态。

**9-5. 文档**

- README 与 upload-system.md 更新完毕（移动端-only、MD5 强唯一、签名/回调路径、观测项和运维步骤）。

**验收标准（Definition of Done, 最终）**

- 以上 9-1 ~ 9-5 全部通过；并提交演示录屏/脚本。

---

## **具体文件落点（建议）**

```
api/video/v1/upload.proto              # 阶段 2
internal/infrastructure/gcs/signer.go  # 阶段 3
internal/repositories/upload_repo.go   # 阶段 4
internal/services/upload_service.go    # 阶段 4
internal/controllers/upload_grpc.go    # 阶段 5
internal/controllers/upload_http.go    # 阶段 5  (/_/gcs/callback)
sqlc/queries/uploads.sql               # 阶段 1
migrations/005_create_catalog_uploads.sql      # 阶段 1
# 若缺 Inbox：
migrations/006_create_catalog_inbox_events.sql # 阶段 1（条件）
configs/config.yaml                     # 阶段 3
docs/upload-system.md（或替换根 upload-system.md） # 阶段 6
scripts/gcloud/setup_uploads.sh         # 阶段 8（可选）
```

---

## **代码骨架（选摘，便于开工）**

**V4 签名（仅会话初始化；强制防覆盖）**

> 只在**发起会话**的 POST 使用 Signed URL；后续分片 PUT 使用 **session URI**，不再需要签名或凭据。

```
// signer.SignedResumableInitURL
storage.SignedURL(bucket, object, &storage.SignedURLOptions{
  Scheme:         storage.SigningSchemeV4,
  Method:         "POST",
  Expires:        time.Now().Add(15*time.Minute),
  GoogleAccessID: saEmail,
  PrivateKey:     pk, // 或使用 SignBytes 回调
  Headers: []string{
    "x-goog-resumable:start",
    "x-upload-content-type:"+contentType,
    // 将前置条件作为签名的一部分以防覆盖
    "x-goog-if-generation-match:0",
  },
})
```

**InitResumableUpload（并发合并 + 已完成复用）**

```
object := fmt.Sprintf("videos/%s/%s", userID, md5hex)
row, inserted := repo.InsertOrGetByUserMd5(ctx, userID, md5hex, NewPendingRow(...object...))
signedURL, exp := signer.SignedResumableInitURL(cfg.GCS.Bucket, object, req.ContentType, ttl, true)
resp := { UploadId: row.UploadID, Bucket: cfg.GCS.Bucket, ObjectName: object, ResumableInitUrl: signedURL, ExpiresAtUnixms: exp.UnixMilli(), AlreadyCompleted: row.Status=="completed" }
return resp
```

**回调（Inbox 去重 + 幂等推进）**

```
-- 事务内
insert into catalog.inbox_events(source, dedup_key, first_msg_id, payload)
values ('gcs', :bucket||'/'||:name||'#'||:generation, :msg_id, :payload)
on conflict do nothing;

-- 如果受影响行=0 -> 已处理，204 返回；否则：
-- uploads: status != 'completed' -> completed + 回填 size/hash/etag/generation
-- videos:  raw_file_reference = 'gs://bucket/object' & status: pending_upload -> processing
-- outbox:  video.upload.completed
-- 标记 inbox.processed_at = now()
```

---

## **风险与回滚**

- **移动端预哈希耗时**：5 分钟视频通常可接受；如需优化可做“边读边算边上传”，但不是本期必需。
- **会话区域固定**（由发起会话的网络路径决定）：移动端地理分布广时需关注时延，但非阻塞。
- **回滚方案**：功能开关 UPLOADS_FEATURE_ENABLED=false 回退至旧路径（若仍保留）；DB 只加表/索引，无破坏性迁移，随时可停用新功能。

---

## **参考（关键事实）**

- **Resumable Upload**（POST 发起会话 → Session URI → PUT 分片 → 308/Range/断点）
- **Signed URL 与 Resumable**（PUT 阶段不需要再签名；Session URI 充当凭据）
- **请求前置条件** ifGenerationMatch=0 防覆盖（存在则 412）
- **Cloud Storage → Pub/Sub → Push + OIDC**（OBJECT_FINALIZE 事件与认证）
- **仓库结构/脚手架**（api/, cmd/, configs/, internal/, migrations/, sqlc/、README 迁移脚本与 Outbox 说明）

---

以上清单按阶段划分、每步都有明确 DoD（验收标准）。你可以直接把各小项转成 Issues/PR 清单逐步落地。如果需要，我可以基于你仓库现有的 wire/kratos 初始化代码，把 **upload_service.go、upload_grpc.go、upload_http.go、upload_repo.go** 四个文件的**最小可运行实现**补全成可编译代码（含 sqlc 查询样例）。
