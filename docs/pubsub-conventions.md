# Pub/Sub 配置规范与约定

> **版本**: v1.0
> **更新日期**: 2025-10-24
> **适用范围**: services-catalog 微服务项目（Catalog Service）

本文档定义了基于 Google Cloud Pub/Sub 的事件驱动架构的配置规范和约定，确保生产者（Producer）和消费者（Consumer）之间的互操作性、可追溯性和一致性。

---

## 📚 目录

- [1. Topic 命名规范](#1-topic-命名规范)
- [2. Subscription 命名规范](#2-subscription-命名规范)
- [3. Message Attributes 规范](#3-message-attributes-规范)
- [4. Ordering Key 策略](#4-ordering-key-策略)
- [5. Message 格式（Payload）](#5-message-格式payload)
- [6. 版本管理策略](#6-版本管理策略)
- [7. 幂等性保证](#7-幂等性保证)
- [8. 错误处理和重试策略](#8-错误处理和重试策略)
- [9. 配置示例](#9-配置示例)
- [10. 监控和告警](#10-监控和告警)

---

## 1. Topic 命名规范

### 1.1 命名模式

```
<domain>.<aggregate>.<event_category>
```

**组成部分：**
- `domain`: 业务域名（如 catalog, search, feed, progress）
- `aggregate`: 聚合根类型（如 video, user, playlist）
- `event_category`: 事件类别（如 events, commands, queries）

### 1.2 示例

| Topic 名称 | 说明 |
|-----------|------|
| `catalog.video.events` | Catalog 服务的视频领域事件（VideoCreated, VideoUpdated, VideoDeleted） |
| `catalog.video.commands` | 视频相关命令（如果使用 CQRS） |
| `media.transcode.events` | 媒体转码事件 |
| `feed.recommendation.events` | 推荐引擎事件 |

### 1.3 命名规则

✅ **推荐做法：**
- 使用小写字母和点分隔符
- 保持简洁但语义明确
- 使用复数形式表示事件集合（events）

❌ **避免做法：**
- 使用下划线或驼峰命名
- 包含环境名称（环境通过 GCP Project 区分）
- 使用过长的名称（超过 3 段）

---

## 2. Subscription 命名规范

### 2.1 命名模式

```
<topic>.<consumer_service>-<consumer_role>
```

**组成部分：**
- `topic`: 关联的 Topic 名称
- `consumer_service`: 消费服务名称
- `consumer_role`: 消费者角色（reader, writer, processor）

### 2.2 示例

| Subscription 名称 | 说明 |
|------------------|------|
| `catalog.video.events.catalog-reader` | Catalog 服务的只读投影消费者 |
| `catalog.video.events.search-indexer` | Search 服务的索引构建器 |
| `catalog.video.events.feed-aggregator` | Feed 服务的内容聚合器 |
| `catalog.video.events.analytics-collector` | 分析服务的数据收集器 |

### 2.3 配置建议

| 配置项 | 推荐值 | 说明 |
|-------|--------|------|
| Ack Deadline | 60s | 消息处理超时时间 |
| Message Retention | 7 days | 消息保留时间 |
| Retry Policy | Exponential Backoff | 指数退避（最小 10s，最大 600s） |
| Dead Letter Topic | `{topic}.dlq` | 死信队列 |
| Max Delivery Attempts | 5 | 最大重试次数 |
| Enable Exactly-Once | true | 启用精确一次语义（推荐） |

---

## 3. Message Attributes 规范

### 3.1 必需属性（Required）

所有消息必须包含以下 attributes：

| 属性名 | 类型 | 说明 | 示例 |
|--------|------|------|------|
| `event_id` | UUID | 事件唯一标识（幂等键） | `550e8400-e29b-41d4-a716-446655440000` |
| `event_type` | String | 事件类型（过去式） | `video.created` |
| `aggregate_id` | UUID | 聚合根 ID | `a3d5e6f7-1234-5678-9abc-def012345678` |
| `aggregate_type` | String | 聚合根类型 | `video` |
| `version` | Integer | 聚合版本号（乐观锁） | `1` |
| `occurred_at` | RFC3339 | 事件发生时间（UTC） | `2025-10-24T10:30:00Z` |
| `schema_version` | String | Payload Schema 版本 | `v1` |

### 3.2 可选属性（Optional）

| 属性名 | 类型 | 说明 | 示例 |
|--------|------|------|------|
| `correlation_id` | UUID | 关联 ID（跟踪调用链） | `b4e6f8a9-2345-6789-bcde-f01234567890` |
| `causation_id` | UUID | 因果 ID（触发此事件的事件 ID） | `c5f7a9b0-3456-789a-cdef-012345678901` |
| `user_id` | UUID | 操作用户 ID | `d6a8b0c1-4567-89ab-def0-123456789012` |
| `source_service` | String | 源服务名称 | `catalog-service` |
| `source_version` | String | 源服务版本 | `v1.2.3` |
| `trace_id` | String | OpenTelemetry Trace ID | `4bf92f3577b34da6a3ce929d0e0e4736` |
| `span_id` | String | OpenTelemetry Span ID | `00f067aa0ba902b7` |

### 3.3 Attributes 构造示例（Go）

```go
func BuildMessageAttributes(evt *events.DomainEvent, traceID string) map[string]string {
    return events.BuildAttributes(evt, events.SchemaVersionV1, traceID)
}
```

---

## 4. Ordering Key 策略

### 4.1 Ordering Key 定义

**目的：** 确保同一聚合根的事件按照发生顺序被消费者处理。

**规则：** 使用 `aggregate_id` 作为 Ordering Key。

```go
orderingKey := event.AggregateId  // UUID string
```

### 4.2 为什么使用 aggregate_id？

✅ **优势：**
1. **保证顺序**: 同一视频的所有事件（Created → Updated → Deleted）按顺序处理
2. **并行处理**: 不同视频的事件可以并行消费
3. **避免竞态**: 防止乱序更新导致的数据不一致

❌ **不推荐：**
- 使用固定值（如 "default"）：会导致串行处理，失去并发性能
- 使用 `event_id`：无法保证同一聚合的事件顺序

### 4.3 Ordering Key 使用示例（Go）

```go
import (
    "cloud.google.com/go/pubsub"
    "google.golang.org/protobuf/proto"
)

func publishEvent(ctx context.Context, topic *pubsub.Topic, evt *events.DomainEvent) error {
    protoEvent, err := events.ToProto(evt)
    if err != nil {
        return fmt.Errorf("encode proto: %w", err)
    }
    payload, err := proto.Marshal(protoEvent)
    if err != nil {
        return fmt.Errorf("marshal event: %w", err)
    }

    msg := &pubsub.Message{
        Data:        payload,
        Attributes:  BuildMessageAttributes(evt, ""),
        OrderingKey: evt.AggregateID.String(), // 关键：使用 aggregate_id 作为 Ordering Key
    }

    _, err = topic.Publish(ctx, msg).Get(ctx)
    return err
}
```

### 4.4 Topic 配置要求

启用 Ordering Key 需要在 Topic 创建时设置：

```bash
gcloud pubsub topics create catalog.video.events \
    --message-ordering
```

---

## 5. Message 格式（Payload）

### 5.1 Payload 结构

**格式：** Protobuf 序列化的 `Event` Envelope

**定义位置：** `api/video/v1/events.proto`

```protobuf
message Event {
  string event_id = 1;                               // 事件唯一标识
  EventType event_type = 2;                          // 事件类型枚举
  string aggregate_id = 3;                           // 聚合根 ID
  string aggregate_type = 4;                         // 聚合类型
  int64 version = 5;                                 // 聚合版本号
  google.protobuf.Timestamp occurred_at = 6;         // 事件发生时间

  // 使用 oneof 实现多态
  oneof payload {
    VideoCreated created = 10;
    VideoUpdated updated = 11;
    VideoDeleted deleted = 12;
  }
}
```

### 5.2 Payload 序列化

**编码方式：** Protobuf Binary（不使用 JSON）

**理由：**
- 更小的消息体积（减少网络传输成本）
- 更快的序列化/反序列化速度
- 强类型保证和向后兼容性

```go
// 序列化
payload, err := proto.Marshal(event)
if err != nil {
    return fmt.Errorf("marshal event: %w", err)
}

// 反序列化
var event videov1.Event
if err := proto.Unmarshal(msg.Data, &event); err != nil {
    return fmt.Errorf("unmarshal event: %w", err)
}
```

### 5.3 完整 Message 结构示例

```json
{
  "data": "<protobuf binary>",
  "attributes": {
    "event_id": "550e8400-e29b-41d4-a716-446655440000",
    "event_type": "video.created",
    "aggregate_id": "a3d5e6f7-1234-5678-9abc-def012345678",
    "aggregate_type": "video",
    "version": "1",
    "occurred_at": "2025-10-24T10:30:00Z",
    "schema_version": "v1",
    "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"
  },
  "orderingKey": "a3d5e6f7-1234-5678-9abc-def012345678",
  "messageId": "1234567890",
  "publishTime": "2025-10-24T10:30:01.123Z"
}
```

---

## 6. 版本管理策略

### 6.1 Schema 版本控制

**原则：** 遵循 Protobuf 的向后兼容规则。

**版本标识：**
- `schema_version` attribute: `v1`, `v2`, `v3`...
- 对应不同的 proto package 或 message 版本

### 6.2 兼容性规则

✅ **允许的变更（向后兼容）：**
- 添加新的 optional 字段
- 添加新的 enum 值
- 添加新的 message 类型

❌ **禁止的变更（破坏兼容性）：**
- 删除字段
- 修改字段类型
- 修改字段编号
- 将 optional 改为 required

### 6.3 版本升级流程

1. **添加新版本 Schema**
   ```protobuf
   // api/video/v2/events.proto
   message VideoCreatedV2 {
       // 新字段
   }
   ```

2. **Producer 同时发布两个版本**
   - `schema_version: v1` → 旧 payload
   - `schema_version: v2` → 新 payload

3. **Consumer 按版本解析**
   ```go
   switch msg.Attributes["schema_version"] {
   case "v1":
       // 解析 v1 格式
   case "v2":
       // 解析 v2 格式
   default:
       // 未知版本，记录错误
   }
   ```

4. **逐步淘汰旧版本**
   - 所有消费者升级后，Producer 停止发布 v1 版本

---

## 7. 幂等性保证

### 7.1 Producer 端幂等

**机制：** 使用 Outbox 表的 `event_id` 作为唯一约束。

```sql
CREATE TABLE catalog.outbox_events (
  event_id UUID PRIMARY KEY,  -- 防止重复插入
  ...
);
```

**保证：** 相同的业务操作生成相同的 `event_id`，确保事件不会重复发布。

### 7.2 Consumer 端幂等

**机制：** 使用 Inbox 表的 `event_id` 作为唯一约束。

```sql
CREATE TABLE catalog.inbox_events (
  event_id UUID PRIMARY KEY,  -- 防止重复处理
  ...
);

-- 插入时使用 ON CONFLICT DO NOTHING
INSERT INTO catalog.inbox_events (event_id, ...)
VALUES ($1, ...)
ON CONFLICT (event_id) DO NOTHING;
```

**处理流程：**
```go
// 1. 尝试插入 Inbox（幂等检查）
affected, err := insertInboxEvent(ctx, event.EventId)
if err != nil {
    return err
}

// 2. 如果 affected == 0，说明已处理过，直接 Ack
if affected == 0 {
    log.Infof("Event already processed: %s", event.EventId)
    msg.Ack()
    return nil
}

// 3. 执行业务逻辑
if err := applyProjection(ctx, event); err != nil {
    msg.Nack()
    return err
}

// 4. 标记为已处理
markInboxEventProcessed(ctx, event.EventId)
msg.Ack()
```

### 7.3 version 字段的作用

**乐观锁机制：** 防止乱序更新。

```sql
-- 投影表的更新逻辑（只有 version 更大时才更新）
UPDATE catalog.videos_projection
SET
    title = $2,
    status = $3,
    version = $4,
    updated_at = now()
WHERE video_id = $1
  AND version < $4;  -- 关键：只有版本号更大才更新
```

---

## 8. 错误处理和重试策略

### 8.1 Producer 端错误处理

**Outbox 表的 delivery_attempts 字段：**

```sql
CREATE TABLE catalog.outbox_events (
  ...
  delivery_attempts INT NOT NULL DEFAULT 0,
  last_error TEXT,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**重试策略：**
1. 首次发布失败 → `delivery_attempts++`, `last_error` 记录错误信息
2. 设置 `available_at = now() + exponential_backoff(delivery_attempts)`
3. 最大重试 5 次，之后标记为 failed（需人工介入）

**退避算法：**
```go
func calculateBackoff(attempts int) time.Duration {
    base := 10 * time.Second
    max := 10 * time.Minute

    backoff := base * time.Duration(math.Pow(2, float64(attempts)))
    if backoff > max {
        return max
    }
    return backoff
}
```

### 8.2 Consumer 端错误处理

**错误分类：**

| 错误类型 | 处理策略 | 示例 |
|---------|---------|------|
| **瞬时错误** | Nack + 重试 | 数据库连接超时、网络抖动 |
| **业务错误** | Ack + 记录日志 | Schema 不兼容、版本回退 |
| **毒消息** | 发送到 DLQ | 无法解析的 payload |

**处理流程：**
```go
func handleMessage(ctx context.Context, msg *pubsub.Message) error {
    // 1. 反序列化
    var event videov1.Event
    if err := proto.Unmarshal(msg.Data, &event); err != nil {
        // 毒消息：无法解析，发送到 DLQ
        sendToDLQ(ctx, msg)
        msg.Ack()
        return nil
    }

    // 2. 幂等性检查（Inbox）
    if alreadyProcessed(ctx, event.EventId) {
        msg.Ack()
        return nil
    }

    // 3. 业务逻辑处理
    if err := applyProjection(ctx, &event); err != nil {
        // 瞬时错误：Nack，让 Pub/Sub 重试
        if isTransientError(err) {
            msg.Nack()
            return err
        }

        // 业务错误：Ack + 记录错误
        recordInboxError(ctx, event.EventId, err.Error())
        msg.Ack()
        return nil
    }

    // 4. 成功处理
    markInboxEventProcessed(ctx, event.EventId)
    msg.Ack()
    return nil
}
```

### 8.3 Dead Letter Queue (DLQ)

**配置：**
```bash
# 创建 DLQ Topic
gcloud pubsub topics create catalog.video.events.dlq

# 创建 DLQ Subscription
gcloud pubsub subscriptions create catalog.video.events.dlq-monitor \
    --topic=catalog.video.events.dlq

# 配置主 Subscription 的 DLQ
gcloud pubsub subscriptions update catalog.video.events.catalog-reader \
    --dead-letter-topic=catalog.video.events.dlq \
    --max-delivery-attempts=5
```

**监控：**
- 定期检查 DLQ 消息数量
- 超过阈值（如 10 条）触发告警
- 分析 DLQ 消息，修复问题后重新发布

---

## 9. 配置示例

### 9.1 Outbox Publisher 配置

```yaml
# configs/config.yaml
pubsub:
  project_id: "smiling-landing-472320-q0"
  emulator_host: ""  # 留空使用真实 Pub/Sub，本地开发可设置 "localhost:8085"

  topics:
    video_events: "catalog.video.events"

  publisher:
    # Outbox 扫描配置
    scan_interval: "5s"           # 每 5 秒扫描一次 Outbox
    batch_size: 100               # 每次最多认领 100 条事件
    max_retry_attempts: 5         # 最大重试次数
    base_backoff: "10s"           # 基础退避时间
    max_backoff: "10m"            # 最大退避时间

    # Pub/Sub Publisher 配置
    num_goroutines: 4             # 发布器协程数
    publish_timeout: "30s"        # 单条消息发布超时
    enable_ordering: true         # 启用消息顺序保证
```

### 9.2 Projection Consumer 配置

```yaml
# configs/config.yaml
pubsub:
  subscriptions:
    video_events_reader:
      subscription_id: "catalog.video.events.catalog-reader"
      max_outstanding_messages: 100    # 最大未确认消息数
      max_outstanding_bytes: 10485760  # 10MB
      num_goroutines: 4                # 消费者协程数
      ack_deadline: "60s"              # Ack 超时时间
      enable_exactly_once: true        # 启用精确一次语义
      sync_mode: false                 # 异步消费（推荐）
```

### 9.3 环境变量配置

```bash
# .env
GCP_PROJECT_ID=smiling-landing-472320-q0
PUBSUB_EMULATOR_HOST=  # 本地开发设置为 "localhost:8085"

# Topic IDs
PUBSUB_TOPIC_VIDEO_EVENTS=catalog.video.events

# Subscription IDs
PUBSUB_SUB_VIDEO_READER=catalog.video.events.catalog-reader
```

---

## 10. 监控和告警

### 10.1 关键指标

**Producer 端（Outbox Publisher）：**

| 指标 | 说明 | 告警阈值 |
|------|------|---------|
| `outbox_pending_count` | Outbox 待发布事件数 | > 1000 |
| `outbox_publish_success_total` | 发布成功总数 | - |
| `outbox_publish_error_total` | 发布失败总数 | > 10/min |
| `outbox_publish_latency` | 发布延迟（秒） | > 30s |
| `outbox_max_delivery_attempts` | 达到最大重试次数的事件 | > 0 |

**Consumer 端（Projection Consumer）：**

| 指标 | 说明 | 告警阈值 |
|------|------|---------|
| `subscription_num_undelivered_messages` | 未投递消息数（Pub/Sub 原生） | > 1000 |
| `subscription_oldest_unacked_message_age` | 最老未确认消息年龄（秒） | > 300 |
| `projection_process_success_total` | 处理成功总数 | - |
| `projection_process_error_total` | 处理失败总数 | > 10/min |
| `projection_process_latency` | 处理延迟（秒） | > 60s |
| `projection_version_lag` | 版本滞后（event version - projection version） | > 10 |
| `dlq_message_count` | DLQ 消息数 | > 10 |

### 10.2 日志规范

**结构化日志字段：**

```go
log.WithFields(log.Fields{
    "event_id":       event.EventId,
    "event_type":     event.EventType,
    "aggregate_id":   event.AggregateId,
    "aggregate_type": event.AggregateType,
    "version":        event.Version,
    "message_id":     msg.ID,
    "trace_id":       msg.Attributes["trace_id"],
}).Info("Event processed successfully")
```

### 10.3 告警规则示例

```yaml
# alerting_rules.yaml
groups:
  - name: pubsub_outbox
    interval: 1m
    rules:
      - alert: OutboxBacklogHigh
        expr: outbox_pending_count > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Outbox 积压过多"
          description: "Outbox 待发布事件数超过 1000 条，持续 5 分钟"

      - alert: OutboxPublishFailureRate
        expr: rate(outbox_publish_error_total[5m]) > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Outbox 发布失败率过高"
          description: "5 分钟内发布失败率超过 10%"

  - name: pubsub_consumer
    interval: 1m
    rules:
      - alert: SubscriptionBacklogHigh
        expr: subscription_num_undelivered_messages > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Subscription 积压过多"
          description: "未投递消息数超过 1000 条，持续 5 分钟"

      - alert: DLQMessagesDetected
        expr: dlq_message_count > 10
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "检测到 DLQ 消息"
          description: "DLQ 中有超过 10 条消息，需要人工介入"
```

---

## 附录 A：快速参考

### Topic 配置命令

```bash
# 创建 Topic（启用消息顺序）
gcloud pubsub topics create catalog.video.events \
    --message-ordering \
    --project=smiling-landing-472320-q0

# 创建 Subscription
gcloud pubsub subscriptions create catalog.video.events.catalog-reader \
    --topic=catalog.video.events \
    --ack-deadline=60 \
    --message-retention-duration=7d \
    --enable-exactly-once-delivery \
    --dead-letter-topic=catalog.video.events.dlq \
    --max-delivery-attempts=5 \
    --project=smiling-landing-472320-q0

# 创建 DLQ
gcloud pubsub topics create catalog.video.events.dlq \
    --project=smiling-landing-472320-q0

gcloud pubsub subscriptions create catalog.video.events.dlq-monitor \
    --topic=catalog.video.events.dlq \
    --project=smiling-landing-472320-q0
```

### 本地开发（Pub/Sub Emulator）

```bash
# 启动 Emulator
gcloud beta emulators pubsub start --project=smiling-landing-472320-q0

# 设置环境变量
export PUBSUB_EMULATOR_HOST=localhost:8085

# 创建 Topic 和 Subscription
gcloud pubsub topics create catalog.video.events \
    --project=smiling-landing-472320-q0

gcloud pubsub subscriptions create catalog.video.events.catalog-reader \
    --topic=catalog.video.events \
    --project=smiling-landing-472320-q0
```

---

## 附录 B：Protobuf Schema 示例

完整的事件定义见 `api/video/v1/events.proto`。

**Event Envelope:**
```protobuf
message Event {
  string event_id = 1;
  EventType event_type = 2;
  string aggregate_id = 3;
  string aggregate_type = 4;
  int64 version = 5;
  google.protobuf.Timestamp occurred_at = 6;

  oneof payload {
    VideoCreated created = 10;
    VideoUpdated updated = 11;
    VideoDeleted deleted = 12;
  }
}
```

**VideoCreated 示例:**
```protobuf
message VideoCreated {
  string video_id = 1;
  string uploader_id = 2;
  string title = 3;
  google.protobuf.StringValue description = 4;
  google.protobuf.Int64Value duration_micros = 5;
  google.protobuf.Timestamp published_at = 6;
  int64 version = 7;
  google.protobuf.Timestamp occurred_at = 8;
  string status = 9;
  string media_status = 10;
  string analysis_status = 11;
}
```

---

## 版本历史

| 版本 | 日期 | 变更说明 |
|------|------|---------|
| v1.0 | 2025-10-24 | 初始版本，定义 Topic/Subscription 命名、Message Attributes、Ordering Key、幂等性、错误处理规范 |

---

**文档维护者**: Catalog Service Team
**联系方式**: catalog-team@example.com
**下次审核**: 2025-11-24
