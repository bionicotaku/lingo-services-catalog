# Catalog Videos 事件：Pub/Sub 资源说明

> 适用范围：`services-catalog` — `catalog.videos` 领域事件的发布（Outbox）与订阅（Uploads Runner）。  
> 示例项目：`smiling-landing-472320-q0`（请执行命令前替换为实际项目）。

---

## 一、配置与资源创建

1. **启用 API 并设置默认项目**
   ```bash
   gcloud services enable pubsub.googleapis.com \
       --project=smiling-landing-472320-q0
   gcloud config set project smiling-landing-472320-q0
   ```

2. **服务账号（最小权限）**
   ```bash
   # 发布端：Outbox Publisher
   gcloud iam service-accounts create sa-catalog-publisher \
       --display-name="Catalog Outbox Publisher"

   # 消费端：Uploads Runner
   gcloud iam service-accounts create sa-catalog-reader \
       --display-name="Catalog Uploads Runner"
   ```

3. **注册事件 Schema（Proto）**
   ```bash
   gcloud pubsub schemas create video-events \
       --project=smiling-landing-472320-q0 \
       --type=protocol-buffer \
       --definition-file=api/video/v1/events.proto
   ```

4. **创建 Topic 与 DLQ**
   ```bash
   gcloud pubsub topics create catalog.video.events \
       --project=smiling-landing-472320-q0 \
       --schema=video-events \
       --message-encoding=binary

   gcloud pubsub topics create catalog.video.events.dlq \
       --project=smiling-landing-472320-q0
   ```

5. **创建订阅（供 Outbox 下游或手动巡检）**
   ```bash
   gcloud pubsub subscriptions create catalog.video.events.catalog-reader \
       --project=smiling-landing-472320-q0 \
       --topic=catalog.video.events \
       --ack-deadline=60 \
       --message-retention-duration=7d \
       --enable-message-ordering \
       --enable-exactly-once-delivery \
       --dead-letter-topic=catalog.video.events.dlq \
       --max-delivery-attempts=5 \
       --min-retry-delay=10s \
       --max-retry-delay=600s
   ```

6. **授予权限**
   ```bash
   gcloud pubsub topics add-iam-policy-binding catalog.video.events \
       --member=serviceAccount:sa-catalog-publisher@smiling-landing-472320-q0.iam.gserviceaccount.com \
       --role=roles/pubsub.publisher

   gcloud pubsub subscriptions add-iam-policy-binding catalog.video.events.catalog-reader \
       --member=serviceAccount:sa-catalog-reader@smiling-landing-472320-q0.iam.gserviceaccount.com \
       --role=roles/pubsub.subscriber
   ```

---

## 二、发布端配置（videos Outbox → `catalog.video.events`）

1. **`configs/config.yaml` 参考**
   ```yaml
   messaging:
     schema: catalog              # Outbox / Inbox 表所在的 DB schema
     topics:
       default:
         project_id: smiling-landing-472320-q0
         topic_id: catalog.video.events
         subscription_id: catalog.video.events.catalog-reader
         dead_letter_topic_id: catalog.video.events.dlq
         ordering_key_enabled: true
         logging_enabled: true
         metrics_enabled: true
         emulator_endpoint: ""    # 本地 Pub/Sub emulator 可改为 localhost:8085
         publish_timeout: 5s
         receive:
           num_goroutines: 4
           max_outstanding_messages: 500
           max_outstanding_bytes: 67108864
           max_extension: 60s
           max_extension_period: 600s
         exactly_once_delivery: true
   ```

2. **凭据要求**
   - 需提供带私钥的 Service Account JSON（用于 V4 Signed URL）；推荐使用环境变量 `GOOGLE_APPLICATION_CREDENTIALS` 指向该文件。
   - 若通过环境变量提供 Base64 JSON，可在 entrypoint 解码后再设置上述变量。

3. **发布流程要点**
   - Outbox Writer 写入 `catalog.outbox_events` → Outbox Runner 序列化 `videov1.Event` → `topic.Publish(ctx, msg)`。
   - 消息属性包含 `schema_version`、`trace_id` 等；`OrderingKey` 取 `video_id`。
   - 发布成功后更新 `published_at`，失败按指数退避重试；达到上限后进入 DLQ。

---

## 三、GCS → Pub/Sub → Uploads Runner（`catalog.video-uploads`）端到端搭建

> 目标：当 GCS 存储桶写入原始视频后，自动触发 `OBJECT_FINALIZE` 事件，推送到 `catalog.video-uploads` Topic，再由 Catalog Uploads Runner 串联 `catalog.uploads` / `catalog.videos` / Outbox。

### 3.1 前置条件

- 已创建目标存储桶（示例：`media-uploads-dev`），并确认启用了 **Uniform bucket-level access**。
- Catalog 服务运行账号具备：
  - `roles/storage.objectViewer`（读取 GCS 元数据，用于校验）；
  - `roles/pubsub.publisher`（若后续扩展需要回写 Topic）；
  - `roles/pubsub.subscriber`（消费上传通知）。
- 本文示例默认项目为 `smiling-landing-472320-q0`，执行命令前请替换。

### 3.2 创建 Topic 与订阅

```bash
gcloud pubsub topics create catalog.video-uploads \
    --project=smiling-landing-472320-q0

gcloud pubsub subscriptions create catalog.video-uploads.runner \
    --project=smiling-landing-472320-q0 \
    --topic=catalog.video-uploads \
    --ack-deadline=60 \
    --message-retention-duration=7d \
    --enable-message-ordering \
    --min-retry-delay=10s \
    --max-retry-delay=600s

gcloud pubsub topics add-iam-policy-binding catalog.video-uploads \
    --project=smiling-landing-472320-q0 \
    --member=serviceAccount:sa-catalog-reader@smiling-landing-472320-q0.iam.gserviceaccount.com \
    --role=roles/pubsub.subscriber
```

> 若 Uploads Runner 使用独立 Service Account，可在此替换为实际账户；推荐单独授予 `roles/pubsub.viewer` 以便排查。

### 3.3 为存储桶创建通知（Cloud Storage → Pub/Sub）

1. 确认项目启用 Storage Notifications API：
   ```bash
   gcloud services enable storage.googleapis.com \
       --project=smiling-landing-472320-q0
   ```
2. 调用 `gsutil notification create` 绑定存储桶与 Topic：
   ```bash
   gsutil notification create \
       -t projects/smiling-landing-472320-q0/topics/catalog.video-uploads \
       -f json \
       -e OBJECT_FINALIZE \
       -p raw_videos/ \
       gs://media-uploads-dev
   ```
   - `-f json` 指定 Cloud Storage JSON 消息格式（Uploads Runner 解码 JSON）。
   - `-e OBJECT_FINALIZE` 仅捕获最终写入事件，忽略删除、元数据更新。
   - `-p raw_videos/` 使用对象前缀过滤，只投递用户上传的原始视频目录。
3. 验证通知是否生效：
   ```bash
   gsutil notification list gs://media-uploads-dev
   ```
   需看到 `topic:projects/.../topics/catalog.video-uploads` 的记录。

### 3.4 更新 Catalog 配置

在 `services-catalog/configs/config.yaml` 中补全 uploads 段落（若已有仅需确认值）：

```yaml
messaging:
  topics:
    uploads:
      project_id: smiling-landing-472320-q0
      topic_id: catalog.video-uploads
      subscription_id: catalog.video-uploads.runner
      logging_enabled: true
      metrics_enabled: true
      emulator_endpoint: ""      # 使用 Pub/Sub emulator 时填入 localhost:8085
      publish_timeout: 5s        # 仅用于需要往该 topic 发布消息的场景
      receive:
        num_goroutines: 4
        max_outstanding_messages: 500
        max_outstanding_bytes: 67108864
        max_extension: 60s
        max_extension_period: 600s
```

> 与 `messaging.outboxes`、`messaging.inboxes` 保持统一，便于 Wire 装配。若本地联调需要 emulator，请同时设置 `PUBSUB_EMULATOR_HOST` 环境变量。

### 3.5 Uploads Runner 处理流程速览

1. StreamingPull 获取消息 → Inbox Runner 记录 `inbox(event_id)` 去重。
2. 解析 JSON payload，抽取 `bucket/name/md5Hash/generation/size`。
3. 查找 `catalog.uploads`，校验 `content_md5`、判断是否重入。
4. `MarkCompleted` 更新上传状态、写入对象摘要；首次完成时驱动 `LifecycleWriter.CreateVideo` + `UpdateVideo`。
5. Lifecycle Writer 在同事务中写入 Outbox 事件，交由 Outbox Runner 发布 `catalog.video.events`。

详见 `internal/tasks/uploads/handler.go` 与 `internal/services/lifecycle_writer.go`。

### 3.6 验证与排查

- **手动触发**：向存储桶上传测试对象（保持 `raw_videos/{user_id}/{video_id}` 命名），查询 Uploads Runner 日志确认处理完整。
- **订阅抽样**：
  ```bash
  gcloud pubsub subscriptions pull catalog.video-uploads.runner \
      --project=smiling-landing-472320-q0 \
      --limit=5 --auto-ack
  ```
- **通知状态**：若消息未触发，使用 `gsutil notification list` 验证配置；或通过 Cloud Logging 查看 GCS 通知是否报错。
- **DLQ 策略**：若后续需要将毒消息隔离，可为 `catalog.video-uploads` 单独创建 DLQ 与监控订阅（流程与视频事件一致）。

> Uploads Runner 默认不返回 Ack 直到事务成功提交，若数据库不可用将自动重试；超过最大投递次数前请确保数据库恢复或手动处理。

---

该文档覆盖了 Catalog Videos 领域事件的 Pub/Sub 资源创建与代码配置。部署时请确保：

- `config.yaml` 与实际项目 ID/Topic/Subscription 一致；
- 服务账号拥有发布/订阅权限并提供带私钥的 JSON；
- `.env` 中仅存放必要密钥（如 `DATABASE_URL`、Base64 SA JSON），其余运行参数通过配置文件管理。*** End Patch***
