下面是**可直接替换仓库文档的最终版**《**Upload System（GCS/移动端）**》设计稿。已按你当前约束落地：

- **仅移动端直传 GCS**（无浏览器，无需 CORS）。
- **同一用户 + 同一文件只允许 1 条视频**（用 (user_id, content_md5) **强唯一**达成；不做幂等键）。
- **网关**已完成 HTTP→gRPC 转发与 JWT 鉴权并在 Metadata 注入 user_id 等（本文仅**说明使用**，不再展开方案）。
- 视频时长 **≤ 5 分钟**（用于接口校验与异常分支）。

文中给出：**端到端流程、数据模型与迁移、对象命名、gRPC 契约、服务端与移动端步骤、GCS 配置（Resumable + Signed URL + 前置条件）、回调去重与幂等、权限与安全、观测与上线清单**。对关键 GCS 细节附官方参考文献。

---

# **Upload System（GCS / Mobile-only）**

**状态**：Accepted（本稿为最终版，取代历史 upload-system.md）

**所属服务**：services-catalog（Catalog 微服务）

**目标**：Catalog 作为**控制面**统一编排上传：发起会话（签名）、强约束唯一性（user_id+content_md5）、接收回调核验、推进领域状态、发布领域事件。数据面（字节流）**仅在移动端 ↔ GCS**之间流动。

**非目标**：

- 服务端**不代理字节流**（避免高带宽/高成本）。
- 本文不定义转码/AI 处理细节，仅触发其前置事件。
- 网关 JWT 方案与用户身份注入**已就绪**，本文仅说明其使用。

---

## **1. 术语与外部事实（关键行为来自 GCS 官方）**

- **Resumable Upload**：先发起会话拿到 **Session URI**，之后用 PUT 携带 Content-Range 分块上传；未完成时返回 **308 Resume Incomplete** 与已持久化区间（Range 头）。会话**约 1 周**有效，成功后返回 **200/201**。
- **XML API 发起会话**：用 POST Object 且带 **x-goog-resumable: start**，响应里的 **Location** 为 Session URI。可通过 **V4 Signed URL** 为该 POST 进行签名。
- **分块大小**：推荐**块大小为 256 KiB 的整数倍**（最后一块可不满足），建议**≥ 8 MiB**以兼顾性能与内存。
- **前置条件**：通过**请求前置条件**避免覆盖既有对象，例如 **ifGenerationMatch=0** 仅在对象不存在时允许写入。
- **对象元数据**：对象包含 **md5Hash\*\***（Base64）\*\* 字段（非复合对象），可用于与客户端 MD5 对账。
- **通知回调**：启用 “**Cloud Storage → Pub/Sub**” 的 **OBJECT_FINALIZE** 事件；Catalog 以 StreamingPull + Inbox Runner 消费，实现事务性幂等处理。

---

## **2. 端到端流程（移动端直传）**

```
Mobile Client         Catalog (gRPC)                    GCS                        Pub/Sub
    |   InitResumableUpload(user_id, filename, size, content_type, content_md5, duration<=300)
    |----------------------------->|
    |   ← video_id, object_name="videos/{user}/{md5}", signed_init_url (V4, 15m)              |
    |                                                                                         |
    |   POST signed_init_url + x-goog-resumable:start  → 201 + Location: <session-URI>        |
    |---------------------------------------------------------------------------------------> |
    |   PUT <session-URI> chunks with Content-Range (256KiB aligned; recommend 8MiB)          |
    |---------------------------------------------------------------------------------------> |
    |   ... 308 Resume Incomplete + Range (已落盘上界)                                         |
    |<--------------------------------------------------------------------------------------- |
    |   (final chunk) → 200/201                                                               |
    |                                                                                         |
    |                                                          OBJECT_FINALIZE (JSON)  -----> | (topic)
    |                                                                                         |
    |         StreamingPull (Inbox Runner → create video + update uploads + publish event)    |
    |<----------------------------------------------------------------------------------------|
    |    tx: inbox.record + uploads.completed + videos.insert_or_update + outbox.enqueue       |
```

> 本系统对“同一用户 + 同一内容（content_md5）”**强唯一**：并发/重试全部收敛到**同一条**上传记录与**同一个对象名**；通过 **ifGenerationMatch=0** 保障对象不被覆盖。

---

```
移动端
   │ 1. 调用 InitResumableUpload（提交文件信息，服务端返回 video_id + 签名 URL）
   ▼
Catalog 服务
   │ 2. 写入 catalog.uploads（状态 pending，预留 video_id）
   │ 3. 生成 V4 Signed URL 并返回
   ▼
Google Cloud Storage (GCS)
   │ 4. 使用签名 URL 发 POST（x-goog-resumable:start）获取 Session URI
   │ 5. 通过 Session URI 分块 PUT 上传，处理 308/Range
   │ 6. 上传成功 → 200/201
   │
   ├─(事件) 对象完成 → Pub/Sub 发布 OBJECT_FINALIZE
   │
Catalog StreamingPull Runner
   │ 7. Inbox 去重 → 校验 md5Hash → 更新 catalog.uploads
   │ 8. 若 catalog.videos 尚不存在该 video_id，则创建基础记录；
   │    写 raw_file_reference、状态置 processing，并写 Outbox 事件
   ▼
后续流水线（转码 / AI / Feed 等）
```

## **3. 数据模型与迁移**

> 你仓库已有 catalog.videos（含 raw_file_reference 与状态机）与 Outbox/Inbox 机制；此处在其上新增/强化“上传聚合”。

### **3.1 表：**

### **catalog.uploads**

```
-- 005_create_catalog_uploads.sql
create table if not exists catalog.uploads (
  video_id          uuid primary key,    -- 预留的视频 ID（上传成功后用于创建主表记录）
  user_id           uuid not null,
  bucket            text not null,
  object_name       text not null,       -- 建议: "videos/{user_id}/{content_md5}"
  original_filename text,
  content_type      text,
  expected_size     bigint not null default 0,  -- 0=未知
  size_bytes        bigint not null default 0,
  content_md5       char(32) not null,         -- 客户端上报的 MD5（hex）
  status            text not null check (status in ('pending','uploading','completed','failed')),
  gcs_generation    text,
  gcs_etag          text,
  md5_hash          text,                 -- GCS 回调里的 md5Hash（统一为 hex 再存）
  crc32c            text,
  error_code        text,
  error_message     text,
  reserved_at       timestamptz not null default now(),
  created_at        timestamptz not null default now(),
  updated_at        timestamptz not null default now()
);

-- 唯一性：同一用户同一内容 永远 只有 1 条上传记录
create unique index if not exists uploads_unique_user_md5
  on catalog.uploads (user_id, content_md5);

-- 同一对象路径唯一（防止命名冲突）
create unique index if not exists uploads_object_unique_idx
  on catalog.uploads (bucket, object_name);

-- updated_at 触发器（略，同仓库风格）
```

> uploads 表中的 `video_id` 仅在回调成功时才会在 `catalog.videos` 创建对应记录；保留独立唯一索引可确保同一用户同一内容只会预留一次 video_id。无需在 videos 表额外添加 `(user_id, content_md5)` 约束。

### **3.2**

### **videos**

### **侧约定（无需迁移）**

- raw_file_reference 回调后写 **gs://{bucket}/{object_name}**；
- 状态由 pending_upload → processing，随后由媒体管线推进到 ready/published。

---

## **4. 对象命名与前置条件**

- **对象路径**：object_name = "videos/{user_id}/{content_md5}"。
- **前置条件**：签名时强制 **ifGenerationMatch=0**，表示**仅当对象不存在**时允许创建，避免并发/重试覆盖。
- **文件名展示**：不进入对象名（避免路径注入），保存在 original_filename 供 UI 显示即可。

---

## **5. gRPC 契约**

> 网关已将 HTTP→gRPC 代理与 JWT 校验完成，并将 user_id 等放入 metadata。服务端从 context 取用户身份执行业务即可（不在本文展开）。

```
syntax = "proto3";
package video.v1;

service UploadService {
  // 创建或复用上传会话（以 user_id + content_md5 为强唯一），并预留 video_id。
  rpc InitResumableUpload(InitResumableUploadRequest) returns (InitResumableUploadResponse);
}

message InitResumableUploadRequest {
  string filename         = 1;
  int64  size_bytes       = 2;    // 可为 0（未知）
  string content_type     = 3;    // e.g. video/mp4
  string content_md5_hex  = 4;    // 必填：移动端先算好 MD5 (hex 32)
  int32  duration_seconds = 5;    // 可选：若提供则校验 <= 300
  string video_id         = 6;    // 可选：复用既有视频 ID（重传场景）
}

message InitResumableUploadResponse {
  string video_id           = 1; // 预留的视频 ID，上传成功后据此创建主表记录
  string bucket             = 2;
  string object_name        = 3; // videos/{user}/{md5}
  string resumable_init_url = 4; // V4 Signed URL (POST + x-goog-resumable:start)
  int64  expires_at_unixms  = 5; // 默认 15 分钟
  bool   already_uploaded   = 6; // 若此前已完成上传则为 true
}
```

---

## **6. 服务端实现**

### **6.1 初始化（无幂等键，靠 MD5 强唯一 + 前置条件）**

**步骤**

1. 从 metadata 读取 user_id。

2. 校验：duration_seconds ≤ 300、content_type 白名单、size_bytes 上限。

3. **预留 video_id 并写入 uploads**：
   - 若请求提供 video_id，则校验其归属与状态（允许重传时复用该记录）。
   - 若未提供，则生成新的 UUID 作为 video_id（此时主表尚未创建记录）。
   - 以 (user_id, content_md5) 为强唯一，在 `catalog.uploads` upsert：若冲突（并发/重试），复用既有 video_id；否则插入状态 `pending` 的新记录。

4. 生成 **V4 Signed URL**（XML API 的 POST），签名包含：

   - x-goog-resumable:start；
   - x-goog-if-generation-match: 0（避免覆盖）；
   - x-upload-content-type: <content_type>；
   - 过期时间（建议 15 分钟）。

5. 返回：video_id/bucket/object_name/resumable_init_url/exp；若记录已处于 completed，`already_uploaded=true` 并可直接使用原有 video 资源。

> 发起会话：移动端对 **Signed URL 做 POST**，成功后从响应头 **Location** 取 **Session URI**。

### **6.2 移动端分块上传规范（摘要，详见 §7）**

- 使用 Session URI 进行 **PUT**；分块按 **256 KiB 的整数倍**（推荐 **≥ 8 MiB**）；最后一块可不满足。
- 未完成时返回 **308 Resume Incomplete**，并携带 **Range**；断线后可发送 **0 字节 PUT +** **Content-Range: bytes \*/TOTAL** 查询偏移。

### **6.3 回调处理（StreamingPull + Inbox Runner）**

**入口**：后台任务通过 `gcpubsub.Subscriber.Receive` 消费 `OBJECT_FINALIZE` 事件，所有逻辑在单事务内完成。

**处理流程（单事务 + 幂等）**

1. 解析消息：仅当 `attributes.eventType == OBJECT_FINALIZE` 时处理；Base64 解码 `message.data` 为对象 JSON。

2. **Inbox 去重**：使用 `source='gcs'`、`dedup_key="{bucket}/{name}#{generation}"` 写入 `catalog.inbox_events`；若记录已存在且 `processed_at` 非空，立即返回成功（Ack）。

3. 读取对象元数据：bucket/name/size/contentType/generation/etag/md5Hash/crc32c。

4. **校验 MD5**：将 md5Hash（Base64）转为 hex 并与 uploads.content_md5 对比：

   - 一致 → 继续；
   - 不一致 → uploads.status='failed'，error_code='MD5_MISMATCH'，记录告警，事务提交后 Ack（不推进视频状态）。

5. 幂等更新：

   - uploads：若当前状态非 completed，则更新为 completed，回填 size/hash/etag/generation。
   - videos：若 `catalog.videos` 中无该 video_id，则创建基础记录（user_id、默认标题/描述、`status='processing'`）；若已存在，则更新 `raw_file_reference` 并按需推进状态。
   - 写 **Outbox**：`video.upload.completed` 等事件，用于触发转码、AI 等后续流程。

6. 标记 Inbox 事件已处理并提交事务；若任一步骤出错则记录 `last_error` 并返回错误，由 Pub/Sub 进行重投（至少一次交付）。

---

## **7. 移动端集成规范（iOS/Android）**

> 移动端**不需要任何 GCP 凭据**：凭借 Catalog 签发的**短时 V4 Signed URL**（POST 发起会话）+ **Session URI** 完成上传。

**步骤**

1. **本地计算 MD5（hex 32）**：建议流式/分片计算（后台线程），5 分钟以内视频通常可接受等待；或“边读边算边上传”。

2. 调 gRPC：InitResumableUpload(filename, size, content_type, content_md5_hex, duration_seconds<=300) → 返回 resumable_init_url。

3. POST resumable_init_url，**必须**带：

   - x-goog-resumable: start

   - x-upload-content-type: <content_type>

   - （已签名）x-goog-if-generation-match: 0（在签名中体现）

     成功后从**响应头** **Location** 读取 **Session URI**。

4. **分块上传**：

   - 设定 CHUNK = 8 MiB（或其他 256 KiB 整倍数）；
   - 逐块 PUT <session-URI>，Content-Range: bytes {start}-{end}/{total}；
   - 若返回 **308**，读取 Range 继续；
   - 断点恢复：PUT 0 字节 + Content-Range: bytes \*/{total} 查询偏移。

5. 最后一块完成时返回 **200/201**；客户端可等待服务端事件通知，或在稍后通过 `CatalogQueryService.GetVideoDetail` 查询状态。

**建议与限制**

- content_type 必须与初始化一致；
- size_bytes 建议提供（可校验异常场景）；
- 仅允许 **video/mp4 / video/quicktime**（示例）等白名单。

---

## **8. 权限与安全**

- **客户端（移动端）零 GCP 凭据**：凭借 **V4 Signed URL** 的时间/路径受限能力，直接对目标对象发起会话（POST），随后使用 Session URI 上传；**任何持有者**在有效期内可用，因此要**缩短有效期**并通过 HTTPS 传输，避免日志泄漏。
- **前置条件**：强制 ifGenerationMatch=0，避免并发/重试覆盖已存在对象。
- **桶访问**：开启 UBLA（统一桶级访问）；Catalog 使用最小权限服务账号（若仅签名，使用 IAM signBlob 能力即可；不需要为客户端授予任何 GCS 角色）。
- **回调**：StreamingPull 任务使用受控服务账号（`roles/pubsub.subscriber`），凭据仅在后台进程内加载。
- **输入校验**：时长 ≤ 300 秒、大小上限、防止异常类型与滥用。

---

## **9. 并发控制、去重与幂等**

- **强唯一（设计基石）**：(user_id, content_md5) 全状态唯一索引 → 「同一用户同一内容仅 1 条上传记录」。
- **会话合并**：并发初始化时，只有一个事务插入成功；冲突方 SELECT 既有记录并返回**同一对象名**与签名 URL。
- **对象不覆盖**：签名中加入 x-goog-if-generation-match: 0，即便出现两个会话，第二个会在最终提交阶段收到**前置条件失败**，不会破坏对象。
- **回调去重**：以 **{bucket}/{name}#{generation}** 为自然去重键写 Inbox，重复回调无副作用。
- **业务幂等**：更新 uploads/videos 使用**条件更新**（如 “仅当 status!=completed 时置为 completed”），重复执行不改变最终状态；Outbox 与业务更新**同事务**提交，实现“几乎一次投递”。

---

## **10. 观测与告警**

**指标（建议以 OpenTelemetry / Prometheus 暴露）**

- upload.init.count/latency
- upload.chunk.resume.count（收到 308 次数）与 upload.offset.mismatch.count
- upload.session.expired.count（一周过期未完成）
- 回调处理时延（OBJECT_FINALIZE → outbox 事件延迟）、回调重试次数
- md5.mismatch.count、if_generation_match.violation.count

**日志关联键**：video_id、user_id、object_name、pubsub_message_id、gcs_generation。

---

## **11. 上线与运维**

### **11.1 迁移**

- 执行 005_create_catalog_uploads.sql（创建表与索引）。
- 升级 Catalog 以暴露 UploadService，并部署 StreamingPull Runner（如 `cmd/tasks/uploads`）。
- 不需要配置 CORS（**移动端-only**）。

### **11.2 GCS 与 Pub/Sub**

1. **创建 Topic**：`video-uploads`。

2. **桶通知 → Pub/Sub**：启用 `OBJECT_FINALIZE`，建议限定 `prefix=videos/`。

3. **创建 StreamingPull 订阅**：

   ```sh
   gcloud pubsub subscriptions create video-uploads-catalog \
     --topic=video-uploads \
     --ack-deadline=30 \
     --message-retention-duration=1209600s \
     --enable-message-ordering \
     --enable-exactly-once-delivery
   ```

   为订阅绑定最小权限服务账号（`roles/pubsub.subscriber`），可按需配置 dead-letter topic 兜底异常消息。

### **11.3 配置清单（**

### **configs/config.yaml**

### **摘要）**

```
gcs:
  project_id: your-project
  bucket: your-bucket
  signer_service_account: upload-signer@your-project.iam.gserviceaccount.com
  signed_url_ttl_seconds: 900

pubsub:
  project_id: your-project
  notification_topic: video-uploads
  subscription_id: video-uploads-catalog
  receive:
    numGoroutines: 4
    maxOutstandingMessages: 500
    maxOutstandingBytes: 67108864
```

---

## **12. 失败与清理**

- **会话过期**：GCS 会话约一周有效，后台 Reaper 定期将超期的 pending/uploading 标记为 failed 并告警。
- **MD5 不一致**：标记 failed (MD5_MISMATCH)，不推进视频状态；必要时提示用户重传。
- **重复上传**：初始化阶段即命中 (user_id, content_md5) 唯一约束，直接复用已存在记录；若对象已存在，ifGenerationMatch=0 会阻止覆盖。

---

## **13. 安全基线**

- Signed URL 有效期**尽量短**（默认 15 分钟），Session URI 不持久化到日志。
- 桶启用 **Uniform bucket-level access**，Catalog 的服务账号仅授予必要权限。
- 回调消费仅运行在受控 StreamingPull 任务中；为其服务账号授予最小权限，错误自动重投但幂等保证最终一致。

---

## **14. 验收用例（建议脚本化）**

1. **并发初始化**：同一用户 + 相同 MD5 并发 10 次 → 仅 1 条 uploads，其余复用；对象名一致。
2. **覆盖防护**：开启两个会话同时上传 → 仅第一个成功，第二个在最终提交阶段因 ifGenerationMatch=0 失败。
3. **断点续传**：上传到 60%，终止进程 → 重新以 Session URI + Content-Range: bytes \*/total 查询并续传至完成；观察 308/Range。
4. **回调幂等**：模拟重复推送同一 generation 的 OBJECT_FINALIZE → 仅首次推进，重复无副作用。
5. **MD5 对账**：构造错误 MD5 请求 → 回调时触发 MD5_MISMATCH。
6. **会话过期**：超过 7 天未完成 → Reaper 标记失败并告警。

---

## **15. 附：实现要点片段**

### **15.1 签名（V4 / XML API / Resumable-init）**

> 在使用 GCS 客户端库生成 V4 Signed URL 时，显式包含需要签名的**请求头**：

- x-goog-resumable:start
- x-goog-if-generation-match: 0
- x-upload-content-type: <content_type>

> 该 URL 仅用于**发起会话的 POST**；移动端随后使用响应头 Location 中的 **Session URI** 进行 PUT 分块。

### **15.2 分块建议**

- chunk = 8 MiB 起步（或网络状况更优时增大）；
- 每块必须是 **256 KiB 的整数倍**（最后一块除外）；处理 **308 + Range**。

### **15.3 回调幂等伪码**

```
-- 事务内
insert into catalog.inbox_events(source, dedup_key, first_msg_id, payload)
values ('gcs', :bucket||'/'||:name||'#'||:generation, :msg_id, :payload)
on conflict do nothing;

-- 若受影响行=0，则已处理过，直接 204 返回
-- 否则：做 uploads/videos 的条件更新 + 写 outbox，最后标记 processed_at
```

---

## **16. 未来演进（不影响现有接口）**

- **请求级幂等键**：未来如需“网关重放零副作用”，可增设 idempotency_keys 表；流程：先查幂等键 → 未命中再走 MD5 强唯一分支（**与本稿完全兼容**）。
- **更强校验**：在移动端计算 **SHA-256** 并存为 content_sha256，将 md5 仅用于与 GCS md5Hash 对账。
- **资产复用**：若日后允许“一份文件多条视频”，将 (user_id, content_md5) 唯一约束迁移到 raw_assets 表，并让 videos 引用 asset_id。

---

## **17. 参考**

- **Resumable 上传步骤 / 308 / Range / 一周有效期**（官方）：发起/执行/状态码说明。
- **V4 Signed URL 概念**（时效性与最小暴露原则）。
- **分块大小要求与推荐**（256 KiB 整倍数、建议 ≥8 MiB）。
- **请求前置条件**（ifGenerationMatch=0 防覆盖）。
- **对象元数据中的** **md5Hash\*\***（Base64）\*\*。
- **Cloud Storage → Pub/Sub 通知与 StreamingPull 消费**。

---

> **总结**：本方案以 **MD5 强唯一 + V4 Signed URL + Resumable + 前置条件**搭建“移动端直传、服务端编排”的可靠闭环；在你“不允许同一内容产生两条视频”的业务约束下，**并发可收敛、覆盖有防护、回调可核验、失败可恢复**，并为后续“请求级幂等键/资产复用/更强校验”预留升级路径。
