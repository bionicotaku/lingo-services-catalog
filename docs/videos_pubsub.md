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

## 三、订阅端配置（Uploads Runner → `catalog.video-uploads`）

1. **创建 Topic / Subscription**
   ```bash
   gcloud pubsub topics create catalog.video-uploads \
       --project=smiling-landing-472320-q0

   gcloud pubsub subscriptions create catalog.video-uploads.runner \
       --project=smiling-landing-472320-q0 \
       --topic=catalog.video-uploads \
       --ack-deadline=60 \
       --message-retention-duration=7d

   gcloud pubsub topics add-iam-policy-binding catalog.video-uploads \
       --member=serviceAccount:sa-catalog-reader@smiling-landing-472320-q0.iam.gserviceaccount.com \
       --role=roles/pubsub.subscriber
   ```

2. **`config.yaml` 段落**
   ```yaml
   messaging:
     topics:
       uploads:
         project_id: smiling-landing-472320-q0
         topic_id: catalog.video-uploads
         subscription_id: catalog.video-uploads.runner
         logging_enabled: true
         metrics_enabled: true
         emulator_endpoint: ""
         publish_timeout: 5s
         receive:
           num_goroutines: 4
           max_outstanding_messages: 500
           max_outstanding_bytes: 67108864
           max_extension: 60s
           max_extension_period: 600s
   ```

3. **Runner 处理流程**
   - 消费 GCS `OBJECT_FINALIZE` 通知 → 根据 `(bucket, object_name)` 查找 `catalog.uploads` 会话。
   - 校验 `md5Hash`，成功则 `MarkCompleted` 并创建/更新 `catalog.videos`，随后写入 Outbox。
   - 利用 Inbox 表 (`INSERT ... ON CONFLICT DO NOTHING`) 保证重复消息幂等。
   - MD5 mismatch 时将会话标记为 `failed`，等待后续正确事件恢复。
   - 本地开发可配置 `PUBSUB_EMULATOR_HOST` 使用 emulator，`emulator_endpoint` 指向同地址，以保持同一代码路径。

---

该文档覆盖了 Catalog Videos 领域事件的 Pub/Sub 资源创建与代码配置。部署时请确保：

- `config.yaml` 与实际项目 ID/Topic/Subscription 一致；
- 服务账号拥有发布/订阅权限并提供带私钥的 JSON；
- `.env` 中仅存放必要密钥（如 `DATABASE_URL`、Base64 SA JSON），其余运行参数通过配置文件管理。*** End Patch***
