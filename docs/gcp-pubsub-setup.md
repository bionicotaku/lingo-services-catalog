# GCP Pub/Sub 配置落地指南

> 版本：v1.0（2025-10-25）
> 适用范围：`services-catalog` Catalog 服务（Outbox 发布 + StreamingPull 投影）
> 维护人：Catalog Service Team

本文档手把手指导如何在 Google Cloud 上为 Catalog 服务创建 Pub/Sub 资源、绑定 Schema、配置权限，并说明本地 Emulator、配置文件扩展与运维要点。所有命令以 `gcloud` CLI 为例，请在执行前替换示例项目 ID（文中使用 `smiling-landing-472320-q0` 占位）。

---

## 1. 前置准备

- **启用 API**：确保目标项目已启用 `pubsub.googleapis.com`。
- **工具链**：安装 `gcloud` ≥ 470.0.0，完成 `gcloud auth login`，并设置默认项目：
  ```bash
  gcloud config set project smiling-landing-472320-q0
  ```
- **服务账号**（推荐拆分最小权限）：
  - 发布端（Outbox Publisher）：`sa-catalog-publisher@…`，授予 `roles/pubsub.publisher`。
  - 消费端（Projection Consumer）：`sa-catalog-reader@…`，授予 `roles/pubsub.subscriber`。
  - 监控（可选 DLQ 巡检）：授予 `roles/pubsub.viewer` + `roles/pubsub.subscriber`。
- **Proto Schema**：事件定义位于 `api/video/v1/events.proto`，需通过 `buf` 生成描述文件。

---

## 2. 构建并注册 Schema

1. 创建 Pub/Sub Schema（类型为 Protocol Buffer，`--definition-file` 需要传入文本 proto 而非 `.desc` 二进制）：
   ```bash
   gcloud pubsub schemas delete video-events
   gcloud pubsub schemas create video-events \
       --project=smiling-landing-472320-q0 \
       --type=protocol-buffer \
       --definition-file=api/video/v1/events.proto
   ```
2. Schema 演进策略：
   - 遵循 Proto 向后兼容规则（字段只增不删、不重用标签号）。
   - 调整 Schema 时重新执行 `buf build` → `schemas commit` 新版本，并同步更新消息 `schema_version` 属性（如从 `v1` 升级为 `v2`）。

---

## 3. 创建 Topic / DLQ

```bash
# 主 Topic，绑定 Schema 并使用二进制编码
gcloud pubsub topics create catalog.video.events \
    --project=smiling-landing-472320-q0 \
    --schema=video-events \
    --message-encoding=binary

# 死信 Topic
gcloud pubsub topics create catalog.video.events.dlq \
    --project=smiling-landing-472320-q0
```

命名遵循 `<domain>.<aggregate>.events`，与 `docs/pubsub-conventions.md` 保持一致。死信 Topic 仅接收无法正常投递的消息，需单独订阅监控。

---

## 4. 创建 Subscription

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

说明：

- 每个消费服务创建独立订阅，命名 `<topic>.<service>-<role>`（例如 `catalog.video.events.search-indexer`）。
- `--enable-message-ordering` 仅对订阅生效；发布端需在消息上设置 `OrderingKey`。
- `--enable-exactly-once-delivery` 要求 `cloud.google.com/go/pubsub` ≥ 1.25.1；若未满足可先关闭，仍由 Inbox + 版本 UPSERT 保证幂等。
- `max-delivery-attempts` 达到上限后消息会转入 DLQ，请配置监控告警。

---

## 5. 绑定 IAM 权限

### 5.1 使用 gcloud 创建服务账号

```bash
# 发布端 Service Account
gcloud iam service-accounts create sa-catalog-publisher \
    --display-name="Catalog Outbox Publisher" \
    --project=smiling-landing-472320-q0

# 消费端 Service Account
gcloud iam service-accounts create sa-catalog-reader \
    --display-name="Catalog Projection Consumer" \
    --project=smiling-landing-472320-q0
```

如果需要本地凭证，可额外创建密钥（建议仅在开发环境使用）：

```bash
gcloud iam service-accounts keys create ./keys/sa-catalog-publisher.json \
    --iam-account=sa-catalog-publisher@smiling-landing-472320-q0.iam.gserviceaccount.com \
    --project=smiling-landing-472320-q0
```

### 5.2 授权 Topic / Subscription

```bash
# 发布端
gcloud pubsub topics add-iam-policy-binding catalog.video.events \
    --project=smiling-landing-472320-q0 \
    --member=serviceAccount:sa-catalog-publisher@smiling-landing-472320-q0.iam.gserviceaccount.com \
    --role=roles/pubsub.publisher

# 消费端
gcloud pubsub subscriptions add-iam-policy-binding catalog.video.events.catalog-reader \
    --project=smiling-landing-472320-q0 \
    --member=serviceAccount:sa-catalog-reader@smiling-landing-472320-q0.iam.gserviceaccount.com \
    --role=roles/pubsub.subscriber
```

补充建议：

- 监控或运维账号可对 DLQ 订阅授予 `roles/pubsub.subscriber`，以便拉取并人工处理。
- 若部署在 Cloud Run，需在服务绑定的 Service Account 上附加上述角色。

---

## 6. 配置文件扩展（代码落地 TODO）

在 `configs/config.yaml` 更新 `messaging.pubsub` 节点：

```yaml
messaging:
  pubsub:
    project_id: smiling-landing-472320-q0 # 替换为实际项目 ID
    topic_id: catalog.video.events
    subscription_id: catalog.video.events.catalog-reader
    dead_letter_topic_id: catalog.video.events.dlq
    ordering_key_enabled: true
    logging_enabled: true
    metrics_enabled: true
    emulator_endpoint: "" # 本地 emulator 填 localhost:8085
    publish_timeout: 5s
    receive:
      num_goroutines: 4
      max_outstanding_messages: 500
      max_outstanding_bytes: 67108864 # 64 MiB
      max_extension: 60s
      max_extension_period: 600s
    exactly_once_delivery: true
```

后续需要在 `internal/infrastructure/configloader` 中解析该结构，并在 Wire 中注入：

- `pubsub.Client`
- Outbox Publisher（后台 goroutine）
- StreamingPull Consumer（后台 goroutine）

所有访问外部 I/O 的函数必须接收 `context.Context` 并设置超时。

---

## 7. 发布 / 消费编码约定

### 发布端（Outbox Publisher）

- 取出 Outbox 行后构造 `videov1.Event`，使用 `proto.Marshal` 得到消息体。
- `pubsub.Message` 设置：

  ```go
  protoEvent, err := events.ToProto(evt)
  if err != nil {
      return fmt.Errorf("encode proto: %w", err)
  }
  data, err := proto.Marshal(protoEvent)
  if err != nil {
      return fmt.Errorf("marshal event: %w", err)
  }

  msg := &pubsub.Message{
      Data:        data,
      OrderingKey: evt.AggregateID.String(),
      Attributes:  events.BuildAttributes(evt, events.SchemaVersionV1, traceID),
  }
  ```

- 调用 `topic.Publish(ctx, msg)` 后必须等待 `result.Get(ctx)`，确定成功再标记 Outbox 行 `published_at`。
- 发布失败时根据 `publisher.min/max_backoff` 做指数退避，达到 `max_attempts` 时告警。

### 消费端（StreamingPull）

- 启动前配置 `Subscription` 对象：
  ```go
  sub.ReceiveSettings.Synchronous = false
  sub.ReceiveSettings.NumGoroutines = cfg.Consumer.MaxGoroutines
  sub.EnableMessageOrdering = cfg.OrderingEnabled
  ```
- 在回调中：
  1. `proto.Unmarshal(msg.Data, &event)`
  2. 开启数据库事务，执行 `Inbox INSERT ... ON CONFLICT DO NOTHING`，并以 `event.version` 做投影表 UPSERT（`WHERE version < EXCLUDED.version`）
  3. 事务成功后 `msg.Ack()`；失败则 `msg.Nack()` 或直接返回错误让 Pub/Sub 重投。
- 捕获 `msg.DeliveryAttempt`，对连续失败的消息记录日志、必要时转入人工处理。
- 应用侧指标：消费者会额外暴露 `projection_apply_success_total`、`projection_apply_failure_total` 以及 `projection_event_lag_ms`，用于观察投影成功率与事件滞后时间。

---

## 8. 本地开发（Pub/Sub Emulator）

1. 启动模拟器：
   ```bash
   gcloud beta emulators pubsub start --project=smiling-landing-472320-q0
   ```
2. 在同一 shell 中设置环境变量（或写入 `.env.dev`）：
   ```bash
   export PUBSUB_EMULATOR_HOST=localhost:8085
   ```
3. 注意：模拟器**不支持** Google Cloud 控制台或 `gcloud pubsub` 命令，需要通过客户端库或 HTTP/gRPC 请求创建资源。示例（REST 调用）：

   ```bash
   curl -X PUT \
     "http://localhost:8085/v1/projects/smiling-landing-472320-q0/topics/catalog.video.events"

   curl -X PUT \
     "http://localhost:8085/v1/projects/smiling-landing-472320-q0/subscriptions/catalog.video.events.catalog-reader" \
     -H "Content-Type: application/json" \
     -d '{
           "topic": "projects/smiling-landing-472320-q0/topics/catalog.video.events",
           "ackDeadlineSeconds": 60
         }'
   ```

4. 运行服务时读取 `PUBSUB_EMULATOR_HOST` 覆盖客户端地址；本地仍需使用 Protobuf 序列化流程，确保与线上逻辑一致。

---

## 9. 验证步骤

1. **发布端连通性**：使用临时工具发布测试事件（待实现，可先 `go run` 写入 Outbox 并观察 Publisher 日志）。
2. **订阅端消费**：启动消费者后手动拉取：
   ```bash
    gcloud pubsub subscriptions pull catalog.video.events.catalog-reader \
       --project=smiling-landing-472320-q0 \
       --limit=5 --auto-ack
   ```
   观察是否为 Protobuf 二进制；如需查看内容，可写临时脚本反序列化。
3. **DLQ 巡检**：
   ```bash
   gcloud pubsub subscriptions pull catalog.video.events.dlq.monitor \
       --project=smiling-landing-472320-q0 \
       --limit=5 --auto-ack
   ```
   若出现异常消息，需定位错误并考虑手动回放。

---

## 9. DLQ 处理流程

1. **创建只读订阅**：为死信 Topic 创建独立订阅（示例 `catalog.video.events.dlq.monitor`），仅用于人工巡检：
   ```bash
   gcloud pubsub subscriptions create catalog.video.events.dlq.monitor \
       --project=smiling-landing-472320-q0 \
       --topic=catalog.video.events.dlq \
       --ack-deadline=300
   ```
2. **分析消息**：定期 `pull` 死信订阅，确认失败原因（如 Schema 不兼容、业务字段缺失、权限错误等）。必要时把 Payload 保存下来供本地复现。
3. **修复并回放**：
   - 修复代码/配置后，可将死信消息重新发布到主 Topic（慎重操作，确保消息幂等）。
   - 或在问题解决后使用 `Seek` 将主订阅回拨到指定时间点进行重放。
4. **治理指标**：为死信订阅配置告警（如 `num_undelivered_messages` 连续上升、`ack_latency` 异常）并建立 Runbook，确保问题不会长时间积压。

---

## 10. 运行与运维建议

- **监控指标**（结合 `docs/pubsub-conventions.md` 中 Prometheus 示例）：
  - Outbox 发布：`outbox_publish_success_total`、`outbox_publish_failure_total`、`outbox_backlog`（Gauge）
  - 投影消费者：`projection_apply_success_total`、`projection_apply_failure_total`、`projection_event_lag_ms`
  - Pub/Sub 内置指标：`subscription/num_undelivered_messages`、`subscription/oldest_unacked_message_age`、DLQ 消息计数
- **回放流程**：当读模型失步时：
  1. 暂停消费者（关闭 Cloud Run 服务或后台 goroutine）。
  2. 执行 `gcloud pubsub subscriptions seek catalog.video.events.catalog-reader --time=<timestamp>`。
  3. 重启消费者，等待投影追平。
- **Schema 升级回滚**：升级前保留旧 Schema；若出现兼容问题，可通过 `gcloud pubsub topics update --clear-schema` 回滚至无 Schema，再重新发布旧版事件并调整消费者。

---

## 11. 常见问题排查

| 现象                                     | 可能原因                                                                 | 解决方案                                                           |
| ---------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------ |
| `schema validation error`                | 发布 Payload 非 `videov1.Event` 或 Schema 未更新                         | 确认使用 `proto.Marshal`，必要时重新生成 `.desc` 并更新 Topic 绑定 |
| `ordering key requires message ordering` | Topic 未启用 `--message-ordering` 或客户端未设置 `EnableMessageOrdering` | 重新创建 Topic/订阅；代码中设置 `sub.EnableMessageOrdering = true` |
| `exceeded exactly once limits`           | Exactly-once 与客户端版本不兼容                                          | 升级 `cloud.google.com/go/pubsub` ≥ 1.27 或暂时关闭该选项          |
| Ack 成功但消息重复到达                   | 订阅启用了 Exactly-once 但消息在 Ack 前重投                              | 检查处理逻辑是否超时，确认 Inbox 幂等处理完整                      |
| Emulator 下无法校验 Schema               | Emulator 不支持 Schema                                                   | 本地开发保持 Protobuf 流程即可，Schema 校验仅在线上项目启用        |

---
