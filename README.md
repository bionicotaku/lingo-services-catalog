# Services-Catalog

**Catalog 微服务** - 维护视频权威元数据，协调上传→转码→AI分析→上架的完整流程。

---

## 概览

Catalog 服务负责：

- **视频元数据管理**：维护视频的基础信息、媒体属性、AI分析结果
- **生命周期协调**：管理视频从上传到发布的状态流转（`pending_upload` → `processing` → `ready` → `published`）
- **事件驱动集成**：通过 Outbox + GCP Pub/Sub 发布领域事件，供下游服务（Search/Feed/Progress）消费

**架构特点：**
- DDD-lite + CQRS + Event Sourcing
- Kratos 微服务框架 + Wire 依赖注入
- PostgreSQL (Supabase) + SQLC
- Outbox Pattern 驱动下游集成
- OpenTelemetry 可观测性

---

## 快速开始

### 1. 环境准备

**必需：**
- Go 1.22+
- PostgreSQL 15+ (推荐 Supabase)
- GCP Pub/Sub (或本地 emulator)

**工具链：**
```bash
# 安装开发工具
make init

# 安装的工具包括：
# - buf (Protocol Buffers 管理)
# - wire (依赖注入代码生成)
# - sqlc (SQL 查询代码生成)
# - gofumpt, goimports (代码格式化)
# - staticcheck, revive (静态检查)
```

### 2. 配置

创建 `configs/.env` 文件：

```env
# 数据库连接（必需）
DATABASE_URL=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable&search_path=catalog

# 服务配置
SERVICE_NAME=services-catalog
SERVICE_VERSION=0.1.0
APP_ENV=development
PORT=9000

# GCP Pub/Sub (可选，不配置则仅输出日志)
PUBSUB_PROJECT_ID=your-project-id
PUBSUB_VIDEO_TOPIC=video-events

# 可观测性 (可选)
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

### 3. 数据库迁移

```bash
# 执行迁移脚本
psql "$DATABASE_URL" -f migrations/001_init_catalog_schema.sql
psql "$DATABASE_URL" -f migrations/002_create_catalog_event_tables.sql
psql "$DATABASE_URL" -f migrations/003_create_catalog_videos_table.sql
psql "$DATABASE_URL" -f migrations/004_create_catalog_video_user_engagements_projection.sql
```

### 4. 启动服务

```bash
# 开发模式
go run ./cmd/grpc -conf configs/config.yaml

# 或构建后运行
make build
./bin/grpc -conf configs/config.yaml
```

服务启动后监听：
- **gRPC**: `0.0.0.0:9000`
- **Metrics**: `0.0.0.0:9090/metrics` (如果启用)

### 5. 请求元数据头（Headers）

Catalog 服务统一使用 `x-md-*` 前缀的 gRPC metadata 传递身份与操作上下文，核心字段包括：

| Header 名称           | 说明 | 示例 |
| ---------------------- | ---- | ---- |
| `x-md-global-user-id`  | 终端用户标识（UUID）。匿名请求可缺省。 | `7b61d0ed-…`
| `x-md-actor-type`      | （Post-MVP 预留）操作者类型，当前服务忽略，可缺省。 | — |
| `x-md-actor-id`        | （Post-MVP 预留）操作者标识，当前服务忽略，可缺省。 | — |
| `x-md-idempotency-key` | 幂等写请求标识，仅写接口使用。 | `req-20251026-001`
| `x-md-if-match` / `x-md-if-none-match` | 条件请求/缓存控制，读写接口按需携带。 | `"W/\"etag\""`

`configs/config.yaml` 与 `data.grpc_client.metadata_keys` 只需保留正在使用的字段（MVP 阶段仅需要 `x-md-global-user-id`、幂等与条件请求相关 Header）。`internal/metadata` 包统一解析这些值；`x-md-actor-*` 作为未来审计扩展保留但当前不会被消费。

### 6. 独立运行 Outbox 发布器

在本地调试或分离式部署场景下，可单独启动 Outbox Runner：

```bash
go run ./cmd/tasks/outbox -conf configs/config.yaml
```

该命令会读取与主服务相同的配置，复用 Pub/Sub 参数、数据库连接与观测设置，仅负责扫描 `catalog.outbox_events` 并发布到 `messaging.pubsub.topic_id`。

### 7. 独立运行 Engagement 投影

```bash
go run ./cmd/tasks/engagement -conf configs/config.yaml
```

该任务订阅 `messaging.engagement.subscription_id` 中的用户互动事件，持续更新 `catalog.video_user_engagements_projection` 投影，可单独部署于后台。

---

## 项目结构

```
services-catalog/
├── cmd/grpc/               # Kratos gRPC 服务入口
│   ├── main.go             # 主程序
│   ├── wire.go             # Wire 依赖注入配置
│   └── wire_gen.go         # Wire 生成代码（自动）
├── configs/                # 配置文件
│   ├── config.yaml         # 基础配置
│   ├── conf.proto          # 配置结构定义
│   └── .env                # 环境变量覆盖（不提交到 Git）
├── api/
│   └── video/v1/           # gRPC Proto 定义
│       ├── video.proto     # 视频查询/命令服务
│       ├── events.proto    # 领域事件定义
│       └── error_reason.proto
├── internal/
│   ├── controllers/        # gRPC Handler（API 层）
│   │   ├── video_command_handler.go
│   │   ├── video_query_handler.go
│   │   └── dto/            # 请求/响应转换
│   ├── services/           # 业务逻辑层
│   │   ├── video_command_service.go
│   │   ├── video_query_service.go
│   │   └── video_types.go
│   ├── repositories/       # 数据访问层
│   │   ├── video_repo.go
│   │   ├── outbox_repo.go
│   │   ├── sqlc/           # SQLC 生成代码
│   │   └── mappers/        # DB 模型映射
│   ├── models/
│   │   ├── po/             # 持久化对象（数据库模型）
│   │   ├── vo/             # 视图对象（返回给客户端）
│   │   └── outbox_events/  # 领域事件构建器
│   ├── infrastructure/     # 基础设施
│   │   ├── configloader/   # 配置加载
│   │   ├── grpc_server/    # gRPC 服务器配置
│   │   └── grpc_client/    # gRPC 客户端配置
│   └── tasks/              # 后台任务
│       └── outbox/         # Outbox 发布器
├── migrations/             # 数据库迁移脚本
├── sqlc/
│   └── schema/             # SQLC 使用的 Schema 定义
├── test/                   # 端到端测试脚本
├── Makefile                # 常用命令
├── buf.yaml                # Buf 配置
├── sqlc.yaml               # SQLC 配置
└── catalog design.md       # 详细设计文档
```

---

## API 接口

### VideoQueryService (只读查询)

```protobuf
service VideoQueryService {
  // 获取视频详情（从投影表读取）
  rpc GetVideoDetail(GetVideoDetailRequest) returns (GetVideoDetailResponse);
}
```

**示例调用：**
```bash
grpcurl -plaintext \
  -d '{"video_id":"550e8400-e29b-41d4-a716-446655440000"}' \
  localhost:9000 video.v1.VideoQueryService/GetVideoDetail
```

### VideoCommandService (写操作)

```protobuf
service VideoCommandService {
  // 创建新视频记录
  rpc CreateVideo(CreateVideoRequest) returns (CreateVideoResponse);

  // 更新视频元数据
  rpc UpdateVideo(UpdateVideoRequest) returns (UpdateVideoResponse);

  // 删除视频记录
  rpc DeleteVideo(DeleteVideoRequest) returns (DeleteVideoResponse);
}
```

**示例调用：**
```bash
# 创建视频
grpcurl -plaintext \
  -d '{
    "upload_user_id":"123e4567-e89b-12d3-a456-426614174000",
    "title":"My Video",
    "description":"Test video",
    "raw_file_reference":"gs://bucket/videos/test.mp4"
  }' \
  localhost:9000 video.v1.VideoCommandService/CreateVideo

# 更新视频（例如：媒体处理完成后）
grpcurl -plaintext \
  -d '{
    "video_id":"550e8400-e29b-41d4-a716-446655440000",
    "media_status":"ready",
    "duration_micros":120000000,
    "thumbnail_url":"gs://bucket/thumbnails/test.jpg",
    "hls_master_playlist":"gs://bucket/hls/test/master.m3u8"
  }' \
  localhost:9000 video.v1.VideoCommandService/UpdateVideo
```

---

## 数据模型

### 主表：catalog.videos

**核心字段：**
- `video_id` (UUID): 主键
- `upload_user_id` (UUID): 上传者
- `title`, `description`: 基础信息
- `raw_file_reference`: 原始文件路径（GCS）
- `status` (enum): 总体状态（`pending_upload` → `processing` → `ready` → `published`）
- `version` (bigint): 乐观锁版本号

**媒体处理字段：**
- `media_status`, `media_job_id`, `media_emitted_at`
- `duration_micros`, `encoded_resolution`, `thumbnail_url`, `hls_master_playlist`

**AI 分析字段：**
- `analysis_status`, `analysis_job_id`, `analysis_emitted_at`
- `difficulty`, `summary`, `tags[]`, `raw_subtitle_url`

## 事件驱动集成

### 发布的事件

| 事件类型 | 触发时机 | 主要字段 | 订阅方 |
|---------|---------|---------|--------|
| `catalog.video.created` | 创建视频 | `video_id`, `title`, `upload_user_id` | Search, Feed, Reporting |
| `catalog.video.updated` | 更新元数据 | `video_id`, 更新字段 | Search, Feed |
| `catalog.video.deleted` | 删除视频 | `video_id` | Search, Feed |

**事件流程：**
1. Service 层在事务内同时写入业务数据 + Outbox 表
2. Outbox 后台任务定期扫描未发布的事件
3. 通过 GCP Pub/Sub 发布到 `video-events` Topic
4. 下游服务（Search/Feed 等）订阅事件并维护各自的读模型

---

## 开发指南

### 常用命令

```bash
# 格式化代码
make fmt

# 静态检查
make lint

# 运行测试
make test

# 构建
make build

# 生成 gRPC 代码
buf generate

# 生成 SQLC 代码
sqlc generate

# 重新生成 Wire 代码
wire ./cmd/grpc
```

### 添加新字段

1. **更新数据库 Schema**：
   - 在 `migrations/` 创建新迁移脚本
   - 更新 `sqlc/schema/` 中的 Schema 定义

2. **更新 SQLC 查询**：
   - 在 `internal/repositories/sqlc/*.sql` 添加查询
   - 运行 `sqlc generate`

3. **更新模型**：
   - 更新 `internal/models/po/video.go`
   - 更新 `internal/models/vo/video.go`

4. **更新 API**：
   - 修改 `api/video/v1/*.proto`
   - 运行 `buf generate`

5. **更新业务逻辑**：
   - 在 `internal/services/` 添加处理逻辑

## 生产部署

### 环境变量（必需）

```env
DATABASE_URL=postgres://...          # 数据库连接
PUBSUB_PROJECT_ID=production-project # GCP 项目 ID
PUBSUB_VIDEO_TOPIC=video-events      # Pub/Sub Topic
SERVICE_NAME=services-catalog
APP_ENV=production
```

### 健康检查

服务提供 gRPC Health Check：

```bash
grpc-health-probe -addr=localhost:9000
```

## 故障排查

### 监控指标

通过 OpenTelemetry 暴露关键指标：
- `catalog_outbox_publish_success_total` / `_failure_total`
- `catalog_outbox_publish_latency_ms`
- `catalog_engagement_apply_success_total` / `_failure_total`
- `catalog_engagement_event_lag_ms`

---

## 故障排查

### 问题：事件未发布到 Pub/Sub

**检查：**
1. 查询 Outbox 表：
   ```sql
   SELECT * FROM catalog.outbox_events WHERE published_at IS NULL;
   ```
2. 检查日志是否有发布错误

**解决：**
- 确认 GCP 凭证配置正确
- 检查 Pub/Sub Topic 权限
- 重启 Outbox 后台任务

### 问题：视频状态卡在 processing

**检查：**
```sql
SELECT video_id, status, media_status, analysis_status, error_message
FROM catalog.videos
WHERE status = 'processing' AND updated_at < NOW() - INTERVAL '1 hour';
```

**解决：**
- 检查 Media/AI 服务是否正常回调
- 查看 `error_message` 字段
- 手动更新状态或触发重试

---

## 参考文档

- [详细设计文档](./catalog%20design.md)
- [只读投影方案](./docs/只读投影方案.md)
- [GCP Pub/Sub 设置](./docs/gcp-pubsub-setup.md)
- [Pub/Sub 约定](./docs/pubsub-conventions.md)
- [Go-Kratos 官方文档](https://go-kratos.dev/)
- [lingo-utils 工具库](https://github.com/bionicotaku/lingo-utils)

---

## 许可证

内部项目，保留所有权利。
