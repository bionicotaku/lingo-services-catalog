# Kratos-Template

> **基于 Kratos v2 的微服务模板 | DDD-lite 架构 | 类型安全的数据访问**

[![Go Version](https://img.shields.io/badge/Go-1.25.3-blue)](https://golang.org/)
[![Kratos Version](https://img.shields.io/badge/Kratos-v2.9.1-green)](https://go-kratos.dev/)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

本项目是一个**生产级 Kratos 微服务模板**，展示如何构建一个遵循 DDD-lite 原则的视频目录服务（Video Catalog Service），包含完整的分层架构、类型安全的数据访问、全链路可观测性和依赖注入等企业级特性。

---

## 📖 目录

- [项目概览](#项目概览)
- [核心特性](#核心特性)
- [技术栈](#技术栈)
- [快速开始](#快速开始)
- [项目结构](#项目结构)
- [架构设计](#架构设计)
- [数据流转](#数据流转)
- [配置说明](#配置说明)
- [测试](#测试)
- [部署](#部署)
- [文档](#文档)
- [开发指南](#开发指南)
- [常见问题](#常见问题)

---

## 项目概览

Kratos-Template 是一个**视频目录微服务**示例，实现了以下业务功能：

- ✅ **视频元数据管理** - 完整的视频信息存储与查询
- ✅ **状态流转控制** - 支持视频生命周期管理（pending_upload → processing → ready → published）
- ✅ **AI 字段支持** - 包含难度评估、摘要生成、标签提取等 AI 分析字段
- ✅ **媒体字段管理** - HLS 播放列表、缩略图、时长等媒体属性

### 业务场景

适用于需要构建**视频/媒体平台**的场景，如：

- 在线教育平台（视频课程管理）
- 短视频/UGC 平台（内容目录服务）
- 企业培训系统（培训视频库）
- 媒体资产管理（MAM）系统

---

## 核心特性

### 🏗 架构特性

- **DDD-lite 分层架构** - Controller → Service → Repository 清晰分离
- **依赖倒置原则** - Service 层定义接口，Repository 层实现
- **Wire 依赖注入** - 编译期注入，零运行时反射
- **类型安全的数据访问** - 使用 sqlc 生成 SQL 查询代码
- **Multi-Module 依赖管理** - 清晰的模块边界与版本控制

### 🔭 可观测性

- **OpenTelemetry 全链路追踪** - 自动注入 trace_id/span_id
- **结构化日志（gclog）** - JSON 格式，兼容 Cloud Logging
- **gRPC 指标采集** - RPC 调用次数、延迟、错误率
- **运行时指标** - CPU、内存、goroutine 监控

### 🛡 生产级特性

- **超时控制** - Context 超时保护（5 秒查询超时）
- **错误分层处理** - Repository → Service → Controller 逐层映射
- **中间件栈** - 追踪、恢复、限流、参数校验、日志完整覆盖
- **健康检查** - gRPC Health Check Protocol 支持
- **优雅关闭** - Wire cleanup 机制保证资源释放顺序
- **数据模型隔离** - DB → PO → VO → Proto 四层转换

### 🧪 测试支持

- **单元测试** - 覆盖 Service 层核心逻辑（≥80%）
- **集成测试** - 连接真实数据库测试 Repository 层
- **测试隔离** - 使用 `package xxx_test` 避免循环依赖

---

## 技术栈

### 后端框架

| 组件                 | 版本     | 用途         |
| -------------------- | -------- | ------------ |
| **Go**               | 1.25.3   | 编程语言     |
| **Kratos**           | v2.9.1   | 微服务框架   |
| **gRPC**             | v1.76.0  | 服务间通信   |
| **Protocol Buffers** | v1.36.10 | 接口定义语言 |
| **Wire**             | v0.7.0   | 依赖注入     |

### 数据存储

| 组件                      | 版本    | 用途            |
| ------------------------- | ------- | --------------- |
| **PostgreSQL (Supabase)** | 15+     | 主数据库        |
| **pgx**                   | v5.7.6  | PostgreSQL 驱动 |
| **sqlc**                  | v1.30.0 | SQL 代码生成器  |

### 可观测性

| 组件              | 版本    | 用途                      |
| ----------------- | ------- | ------------------------- |
| **OpenTelemetry** | v1.38.0 | 分布式追踪与指标          |
| **gclog**         | v0.1.0  | 结构化日志（lingo-utils） |
| **observability** | v0.1.0  | 观测组件（lingo-utils）   |

### 中间件与工具

| 组件                         | 版本    | 用途                          |
| ---------------------------- | ------- | ----------------------------- |
| **gcjwt**                    | v0.1.0  | JWT 认证中间件（lingo-utils） |
| **buf**                      | -       | Protobuf 管理工具             |
| **protovalidate**            | v1.0.0  | 参数校验（运行时反射验证）    |
| **protovalidate middleware** | v2.27.0 | Kratos protovalidate 中间件   |

---

## 快速开始

### 前置要求

- Go 1.22+ ([安装指南](https://golang.org/dl/))
- PostgreSQL 数据库（推荐 Supabase）
- Buf CLI ([安装指南](https://docs.buf.build/installation))
- Make 工具

### 1. 克隆项目

```bash
git clone https://github.com/bionicotaku/kratos-template.git
cd kratos-template
```

### 2. 安装开发工具

```bash
make init
```

这将安装：

- `wire` - 依赖注入代码生成
- `sqlc` - SQL 代码生成
- `gofumpt`, `goimports` - 代码格式化
- `staticcheck`, `revive` - 静态检查工具

### 3. 配置数据库

创建 `.env` 文件（或编辑 `configs/config.yaml`）：

```bash
# 复制配置模板
cp configs/config.yaml.example configs/config.yaml

# 编辑配置
vim configs/config.yaml
```

配置示例：

```yaml
server:
  grpc:
    addr: "0.0.0.0:9090"
    timeout: "5s"

data:
  database:
    driver: "postgres"
    source: "postgres://user:password@db.supabase.co:5432/postgres?sslmode=require"
```

### 4. 执行数据库迁移

```bash
# 创建 catalog 基线
psql $DATABASE_URL < migrations/001_init_catalog_schema.sql

# 创建事件表（Outbox/Inbox）
psql $DATABASE_URL < migrations/002_create_catalog_event_tables.sql

# 创建视频主表及索引/触发器
psql $DATABASE_URL < migrations/003_create_catalog_videos_table.sql

# 创建只读视图
psql $DATABASE_URL < migrations/004_create_catalog_videos_ready_view.sql
```

### 5. 生成代码

```bash
# 生成 Proto 代码
make api

# 生成 sqlc（含 sqlc generate）+ go generate（含 wire）+ go mod tidy
make generate
```

### 6. 运行服务

```bash
# 开发模式（热重载）
make run

# 或直接运行二进制
go run cmd/grpc/main.go -conf configs/
```

服务将在 `:9090` 端口启动 gRPC Server。

### 7. 测试接口

使用 grpcurl 测试：

```bash
# 查看服务列表
grpcurl -plaintext localhost:9090 list

# 调用 GetVideoDetail
grpcurl -plaintext -d '{"video_id": "550e8400-e29b-41d4-a716-446655440000"}' \
  localhost:9090 video.v1.VideoQueryService/GetVideoDetail
```

---

## 项目结构

```
kratos-template/
├── api/                              # gRPC 接口定义
│   └── video/v1/
│       ├── video.proto               # 视频服务 Proto 定义
│       ├── video.pb.go               # 生成的 Go 代码
│       └── video_grpc.pb.go          # 生成的 gRPC 服务端代码
│
├── cmd/                              # 应用入口
│   └── grpc/
│       ├── main.go                   # 主入口（加载配置并启动服务）
│       ├── wire.go                   # Wire 依赖注入定义
│       └── wire_gen.go               # Wire 生成的装配代码
│
├── configs/                          # 配置文件
│   └── config.yaml                   # 服务配置（地址、数据库等）
│
├── internal/                         # 内部实现（不可被外部引用）
│   ├── controllers/                  # 控制器层（HTTP/gRPC Handler）
│   │   ├── video_handler.go          # 视频查询 Handler
│   │   └── init.go                   # Wire ProviderSet
│   │
│   ├── services/                     # 服务层（业务逻辑）
│   │   ├── video.go                  # 视频业务用例
│   │   ├── init.go                   # Wire ProviderSet
│   │   └── test/                     # 单元测试
│   │
│   ├── repositories/                 # 仓储层（数据访问）
│   │   ├── video_repo.go             # 视频仓储实现
│   │   ├── sqlc/                     # sqlc 生成的代码
│   │   │   ├── db.go
│   │   │   ├── models.go
│   │   │   ├── video.sql             # SQL 查询定义
│   │   │   └── video.sql.go          # 生成的查询代码
│   │   ├── mappers/                  # 数据模型转换
│   │   │   └── video.go              # CatalogVideo → po.Video
│   │   └── init.go                   # Wire ProviderSet
│   │
│   ├── models/                       # 数据模型
│   │   ├── po/                       # 持久化对象（Persistent Object）
│   │   │   └── video.go              # 视频数据库模型
│   │   └── vo/                       # 视图对象（View Object）
│   │       └── video.go              # 视频业务视图
│   │
│   ├── views/                        # 视图转换层
│   │   └── video.go                  # VO → Proto Response
│   │
│   ├── infrastructure/               # 基础设施层
│   │   ├── config_loader/            # 配置加载
│   │   ├── database/                 # 数据库连接池
│   │   ├── grpc_server/              # gRPC Server 配置
│   │   └── grpc_client/              # gRPC Client 配置
│   │
│   ├── clients/                      # 外部服务客户端
│   └── tasks/                        # 异步任务/定时任务
│
├── migrations/                       # 数据库迁移脚本
│   └── 001_create_catalog_schema.sql # 初始化 Schema
│
├── test/                             # 测试
│   ├── integration/                  # 集成测试
│   │   └── video_integration_test.go # 视频服务集成测试
│   └── fixtures/                     # 测试数据
│
├── docs/                             # 项目文档
│   ├── ARCHITECTURE.md               # 架构设计文档（⭐推荐阅读）
│   ├── ARCHITECTURE_PATTERNS.md      # 架构模式详解
│   ├── WIRE_DEPENDENCY_INJECTION.md  # Wire 使用指南
│   └── HEXAGONAL_ARCHITECTURE.md     # 六边形架构分析
│
├── scripts/                          # 脚本工具
├── Makefile                          # 构建任务
├── go.mod                            # Go 模块定义
├── sqlc.yaml                         # sqlc 配置
├── buf.yaml                          # Buf 配置
└── README.md                         # 本文件
```

---

## 架构设计

### 分层架构（DDD-lite）

```
┌──────────────────────────────────────────┐
│  Adapter 层 (Controllers + Views)        │  ← 协议适配（gRPC/HTTP）
├──────────────────────────────────────────┤
│  Application 层 (Services)               │  ← 业务用例编排
├──────────────────────────────────────────┤
│  Domain 层 (Models: PO/VO)               │  ← 领域模型
├──────────────────────────────────────────┤
│  Infrastructure 层 (Repositories)        │  ← 基础设施（数据库/缓存）
└──────────────────────────────────────────┘
```

### 依赖方向（单向依赖）

```
Controllers  →  Services  →  Repositories  →  Database
     ↓              ↓              ↓
  (参数校验)    (业务逻辑)     (数据访问)
     ↓              ↓              ↓
  Views          VO/PO         sqlc/pgx
```

**关键规则**：

- ✅ 上层可依赖下层
- ❌ 下层**禁止**依赖上层
- ✅ Service 通过**接口**依赖 Repository（依赖倒置）

### Wire 依赖注入图

本项目使用 **Google Wire** 进行依赖注入，以下是完整的依赖关系图（箭头表示数据流向：提供者 → 使用者）：

```mermaid
graph LR
    subgraph Input["输入层"]
        Params["configloader.Params<br/>(命令行参数)"]
    end

    subgraph Config["配置层"]
        Bundle["*loader.Bundle<br/>(配置包)"]
        Metadata["ServiceMetadata<br/>(服务元信息)"]
        ServerCfg["*configpb.Server"]
        DataCfg["*configpb.Data"]
    end

    subgraph Foundation["基础组件层"]
        Logger["log.Logger<br/>(结构化日志)"]
        Obs["*observability.Component<br/>(追踪/指标)"]
        JWT["*gcjwt.Component<br/>(JWT 中间件)"]
        DB["*pgxpool.Pool<br/>(数据库连接池)"]
    end

    subgraph Business["业务层"]
        VR["*VideoRepository<br/>(仓储实现)"]
        VU["*VideoUsecase<br/>(业务用例)"]
        VH["*VideoHandler<br/>(gRPC Handler)"]
    end

    subgraph Server["服务器层"]
        GRPCServer["*grpc.Server<br/>(gRPC 服务器)"]
        App["*kratos.App<br/>(应用实例)"]
    end

    %% 配置流
    Params --> Bundle
    Bundle --> Metadata
    Bundle --> ServerCfg
    Bundle --> DataCfg

    %% 基础组件流
    Metadata --> Logger
    Metadata --> Obs
    ServerCfg --> JWT
    DataCfg --> JWT
    DataCfg --> DB

    %% 业务流（自底向上）
    DB --> VR
    Logger --> VR
    VR --> VU
    Logger --> VU
    VU --> VH

    %% 服务器流
    VH --> GRPCServer
    JWT --> GRPCServer
    Logger --> GRPCServer
    ServerCfg --> GRPCServer

    GRPCServer --> App
    Logger --> App
    Metadata --> App
    Obs --> App

    %% 样式
    style App fill:#e74c3c,color:#fff,stroke:#c0392b,stroke-width:4px
    style Params fill:#95a5a6,color:#fff
    style Config fill:#95a5a6,color:#fff
    style Foundation fill:#2ecc71,color:#fff
    style Business fill:#3498db,color:#fff
    style Server fill:#f39c12,color:#fff
```

**依赖流向说明**（自底向上构建）：

1. **配置层** (灰色)

   - `Params` (输入) → `Bundle` (配置包)
   - `Bundle` → `ServiceMetadata` + `Server/Data Config`

2. **基础组件层** (绿色)

   - `ServiceMetadata` → `Logger` (gclog)
   - `ServiceMetadata` → `Obs` (observability)
   - `Server/Data Config` → `JWT` (gcjwt)
   - `Data Config` → `DB` (pgxpool)

3. **业务层** (蓝色)

   - `DB + Logger` → `VideoRepository`
   - `VideoRepository + TxManager + Logger` → `VideoUsecase` (通过接口)
   - `VideoUsecase` → `VideoHandler`

4. **服务器层** (橙色/红色)
   - `VideoHandler + JWT + Logger + Config` → `gRPC Server`
   - `gRPC Server + Logger + Metadata + Obs` → `kratos.App` (最终目标)

**依赖倒置体现**：

```go
// 1. Service 层定义接口
type VideoRepo interface {
    FindByID(ctx context.Context, videoID uuid.UUID) (*po.Video, error)
}

// 2. Repository 层实现接口
type VideoRepository struct { ... }
func (r *VideoRepository) FindByID(...) (*po.Video, error) { ... }

// 3. Wire 绑定（接口 ← 实现）
wire.Bind(new(services.VideoRepo), new(*repositories.VideoRepository))

// 4. Service 依赖接口 + TxManager，运行时注入实现
func NewVideoUsecase(repo VideoRepo, tx txmanager.Manager, logger log.Logger) *VideoUsecase {
    return &VideoUsecase{
        repo:      repo,
        txManager: tx,
        log:       log.NewHelper(logger),
    }
}
```

### 详细架构图

完整的系统架构图、时序图、数据流转图请参考：

📖 **[docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)** - 包含 10+ 张 mermaid 详细架构图

---

## 数据流转

### 完整请求处理流程

```
1. gRPC Client 发送请求
   ↓
2. gRPC Server 中间件处理
   - 追踪中间件（创建 Span）
   - 日志中间件（记录请求）
   - 参数校验（protovalidate 运行时验证）
   ↓
3. Controller 层 (video_handler.go)
   - 参数校验（video_id 非空）
   - UUID 解析（string → uuid.UUID）
   - 设置超时（5 秒）
   - 调用 Service 层
   ↓
4. Service 层 (video.go)
   - 调用 Repository 接口
   - 错误分类（404/504/500）
   - 构造 VO（View Object）
   ↓
5. Repository 层 (video_repo.go)
   - 调用 sqlc 生成的查询方法
   - 处理数据库错误（pgx.ErrNoRows → ErrVideoNotFound）
   - Mapper 转换（CatalogVideo → po.Video）
   ↓
6. sqlc 查询层 (video.sql.go)
   - 执行 SQL: SELECT ... FROM catalog.videos WHERE video_id = $1
   - 扫描结果到 CatalogVideo 结构体
   ↓
7. PostgreSQL 数据库
   - 通过主键索引快速查询
   - 返回完整记录
   ↓
8. 数据模型转换
   - CatalogVideo (pgtype 类型) → po.Video (Go 原生类型)
   - po.Video → vo.VideoDetail (精简视图)
   - vo.VideoDetail → videov1.VideoDetail (Protobuf)
   ↓
9. gRPC Server 返回响应
   - 日志记录响应
   - 追踪记录 Span 结束
```

### 数据模型转换链

```
catalog.videos (PostgreSQL)
    ↓ sqlc.Scan
CatalogVideo (pgtype.Text, pgtype.Timestamptz)
    ↓ mappers.VideoFromCatalog
po.Video (*string, time.Time) [完整字段]
    ↓ vo.NewVideoDetail
vo.VideoDetail (*string, time.Time) [精简字段]
    ↓ views.NewVideoDetail
videov1.VideoDetail (wrapperspb.StringValue, timestamppb.Timestamp)
```

---

## 配置说明

### 配置文件结构

`configs/config.yaml`：

```yaml
# 服务配置
server:
  grpc:
    addr: "0.0.0.0:9090" # gRPC 监听地址
    timeout: "5s" # 全局超时
    network: "tcp" # 网络协议

# 数据库配置
data:
  database:
    driver: "postgres"
    source: "postgres://user:password@host:5432/dbname?sslmode=require"
    max_idle_conns: 10
    max_open_conns: 100

# 可观测性配置
observability:
  # 追踪配置
  tracing:
    enabled: true
    exporter: "otlp_grpc" # stdout | otlp_grpc
    endpoint: "localhost:4317"
    sampling_rate: 1.0

  # 指标配置
  metrics:
    enabled: true
    grpc_enabled: true
    grpc_include_health: false # 是否采集健康检查指标
    runtime_enabled: true # 运行时指标（CPU/内存/goroutine）

# 日志配置（gclog）
log:
  level: "info" # debug | info | warn | error
  format: "json" # json | console
```

### 环境变量

支持通过环境变量覆盖配置：

```bash
# 应用配置
export APP_ENV=production
export APP_LOG_LEVEL=info

# 数据库配置
export DATABASE_URL="postgres://..."

# 配置文件路径
export CONF_PATH=./configs/
```

---

## 测试

### 运行所有测试

```bash
# 运行所有测试（包含集成测试）
make test

# 仅运行单元测试
go test ./internal/... -short

# 生成覆盖率报告
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 测试分类

| 测试类型        | 位置                                  | 覆盖范围              | 依赖            |
| --------------- | ------------------------------------- | --------------------- | --------------- |
| **单元测试**    | `internal/services/test/`             | Service 层业务逻辑    | Mock Repository |
| **集成测试**    | `test/integration/`                   | Repository 层数据访问 | 真实数据库      |
| **Mapper 测试** | `internal/repositories/mappers/test/` | 数据模型转换          | 无              |
| **VO 测试**     | `internal/models/vo/test/`            | 视图对象构造          | 无              |

### 测试约定

- ✅ 测试文件放在 `test/` 子目录
- ✅ 使用 `package xxx_test` 避免循环依赖
- ✅ Service 层覆盖率 ≥ 80%
- ✅ 集成测试连接真实数据库（独立 schema）

---

## 部署

### 本地构建

```bash
# 编译二进制
make build

# 运行
./bin/grpc -conf configs/
```

### Docker 部署

```bash
# 构建镜像
docker build -t kratos-template:latest .

# 运行容器
docker run -d \
  -p 9090:9090 \
  -e DATABASE_URL="postgres://..." \
  -v $(pwd)/configs:/app/configs \
  kratos-template:latest
```

### 健康检查

```bash
# gRPC Health Check
grpcurl -plaintext localhost:9090 grpc.health.v1.Health/Check
```

---

## 文档

### 核心文档

| 文档                                                                | 说明                            | 推荐阅读顺序 |
| ------------------------------------------------------------------- | ------------------------------- | ------------ |
| **[ARCHITECTURE.md](./docs/ARCHITECTURE.md)**                       | 完整架构设计文档（⭐ 推荐首读） | 1            |
| [ARCHITECTURE_PATTERNS.md](./docs/ARCHITECTURE_PATTERNS.md)         | 架构模式深度分析                | 2            |
| [WIRE_DEPENDENCY_INJECTION.md](./docs/WIRE_DEPENDENCY_INJECTION.md) | Wire 依赖注入完整指南           | 3            |
| [HEXAGONAL_ARCHITECTURE.md](./docs/HEXAGONAL_ARCHITECTURE.md)       | 六边形架构应用                  | 4            |

### 外部资源

- [Kratos 官方文档](https://go-kratos.dev/)
- [sqlc 官方文档](https://docs.sqlc.dev/)
- [Wire 依赖注入指南](https://github.com/google/wire)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)

---

## 开发指南

### 代码风格

本项目遵循以下规范：

- **格式化**：`gofumpt` + `goimports`
- **静态检查**：`staticcheck` + `revive`
- **命名约定**：Go 官方风格指南
- **注释规范**：所有导出函数必须有文档注释

### 提交前检查清单

```bash
# 1. 格式化代码
make fmt

# 2. 静态检查
make lint

# 3. 运行测试
make test

# 4. 生成覆盖率
go test ./... -cover

# 5. 检查 Proto 契约
buf lint
buf breaking --against .git#branch=main
```

### 如何添加新接口

1. **定义 Proto 接口**

   ```protobuf
   // api/video/v1/video.proto
   rpc CreateVideo(CreateVideoRequest) returns (CreateVideoResponse);
   ```

2. **生成代码**

   ```bash
   make api
   ```

3. **实现 Service 层**

   ```go
   // internal/services/video.go
   func (uc *VideoUsecase) CreateVideo(ctx context.Context, req *CreateVideoRequest) (*vo.Video, error) {
       // 业务逻辑
   }
   ```

4. **实现 Repository 层（如需）**

   ```sql
   -- internal/repositories/sqlc/video_write.sql
   -- name: CreateVideo :one
   INSERT INTO catalog.videos (...) VALUES (...) RETURNING *;
   ```

5. **实现 Controller 层**

   ```go
   // internal/controllers/video_handler.go
   func (h *VideoHandler) CreateVideo(ctx context.Context, req *videov1.CreateVideoRequest) (*videov1.CreateVideoResponse, error) {
       // 调用 Service 层
   }
   ```

6. **在 Wire 中注册**（自动生成的代码无需手动修改）

### 最佳实践

| 场景         | 推荐做法                         | 禁止做法                    |
| ------------ | -------------------------------- | --------------------------- |
| **参数校验** | 在 Controller 层完成             | 在 Service 层校验 HTTP 参数 |
| **业务逻辑** | 在 Service 层实现                | 在 Controller 写业务代码    |
| **数据访问** | 在 Repository 层封装             | Service 层直接调用 SQL      |
| **错误处理** | 分层映射（404/504/500）          | 统一返回 500                |
| **超时控制** | 使用 `context.WithTimeout`       | 无限等待                    |
| **数据模型** | 多层隔离（DB → PO → VO → Proto） | 直接暴露数据库模型          |

详见：[docs/ARCHITECTURE.md - 第 12 节：最佳实践与反模式](./docs/ARCHITECTURE.md#12-最佳实践与反模式)

---

## 常见问题

### Q1: 为什么 Service 层要定义 Repository 接口？

**A**: 这是**依赖倒置原则**的体现。Service 层依赖接口而非具体实现，便于：

- 单元测试时使用 Mock Repository
- 切换不同的数据源实现
- 解耦业务逻辑与基础设施

详见：[docs/ARCHITECTURE.md - 第 7 节：依赖注入](./docs/ARCHITECTURE.md#7-依赖注入与组件装配)

### Q2: 为什么需要这么多数据模型（CatalogVideo/PO/VO/Proto）？

**A**: 多层数据模型隔离的目的是：

- **CatalogVideo**: sqlc 生成的数据库模型（含 pgtype 类型）
- **po.Video**: 领域持久化对象（Go 原生类型），不泄漏数据库细节
- **vo.VideoDetail**: 业务视图对象（精简字段），过滤内部字段
- **Proto**: 传输对象（Protobuf），面向 API 消费者

详见：[docs/ARCHITECTURE.md - 第 6 节：数据模型转换](./docs/ARCHITECTURE.md#6-数据模型转换)

### Q3: Controller 层应该做什么？

**A**: Controller 层**仅做**：

- ✅ 参数校验（非空、格式检查）
- ✅ 类型转换（string → UUID）
- ✅ 设置超时上下文
- ✅ 调用 Service 层
- ❌ **禁止**包含业务逻辑

详见：[docs/ARCHITECTURE.md - 第 4.1 节：Adapter 层](./docs/ARCHITECTURE.md#41-adapter-层适配器层)

### Q4: 如何调试 Wire 依赖注入问题？

**A**:

1. 查看生成的 `wire_gen.go` 文件
2. 检查 `ProviderSet` 是否正确导出
3. 确认接口绑定：`wire.Bind(new(ServiceInterface), new(*RepositoryImpl))`
4. 运行 `wire` 查看错误信息

详见：[docs/WIRE_DEPENDENCY_INJECTION.md](./docs/WIRE_DEPENDENCY_INJECTION.md)

### Q5: 如何连接 Supabase 数据库？

**A**: 在 Supabase 项目中：

1. 进入 **Settings → Database**
2. 复制 **Connection String (URI)**
3. 确保包含 `?sslmode=require`
4. 配置到 `configs/config.yaml` 的 `data.database.source`

示例：

```
postgres://postgres.xxx:password@aws-0-us-west-1.pooler.supabase.com:5432/postgres?sslmode=require
```

### Q6: sqlc 生成代码报错怎么办？

**A**:

1. 检查 `sqlc.yaml` 配置是否正确
2. 确保 SQL 查询语法正确（PostgreSQL 方言）
3. 运行 `sqlc vet` 检查 SQL 质量
4. 查看 sqlc 版本：`sqlc version`（推荐 v1.30.0+）

---

## 生产特性清单

### ✅ 已实现

- ✅ **分层架构** - Controller → Service → Repository 清晰分离
- ✅ **依赖注入** - Wire 编译期注入
- ✅ **可观测性** - OpenTelemetry 追踪/指标 + 结构化日志
- ✅ **类型安全的 SQL** - sqlc 生成查询代码
- ✅ **配置管理** - Proto Schema + protovalidate 运行时校验
- ✅ **中间件栈** - 追踪、恢复、限流、参数校验、日志
- ✅ **超时控制** - Context 超时保护
- ✅ **错误分层处理** - 404/504/500 分类映射
- ✅ **数据模型隔离** - DB → PO → VO → Proto 四层转换
- ✅ **健康检查** - gRPC Health Check Protocol
- ✅ **优雅关闭** - Wire cleanup 机制

### 🚧 待实现（生产必备）

- ⬜ **幂等性支持** - `Idempotency-Key` header 处理
- ⬜ **并发控制** - 乐观锁（ETag/Version）
- ⬜ **分页限制** - 游标分页（cursor-based）
- ⬜ **缓存层** - Redis 缓存（Cache-Aside 模式）
- ⬜ **事务支持** - 跨 Repository 事务
- ⬜ **API 限流** - Token Bucket 算法
- ⬜ **监控告警** - Prometheus + Grafana

---

**最后更新**: 2025-10-23
**文档版本**: v2.0
