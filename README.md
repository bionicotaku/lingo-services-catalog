# kratos-template

`kratos-template` 是一份 **单服务仓库模板**，用于快速构建基于 [Go-Kratos](https://go-kratos.dev/) 的微服务。每个业务服务（Catalogue、Progress、Media 等）都应从本模板派生，保持统一的目录结构、工具链和质量基线。

> ✅ 模板仅定位于“单服务仓库”。生产环境中，每个微服务拥有独立仓库，但都共享这里的工程骨架与脚本。

---

## 总览

```mermaid
mindmap
  root((kratos-template))
    cmd
      grpc<br/>(主入口)
      http<br/>(可选调试入口)
    configs
      config.yaml
      .env
    internal
      controllers
      services
      repositories
      models
        po
        vo
      clients
      infrastructure
      tasks
    api
      openapi
      proto
    migrations
    test
      full_e2e_projection.sh
    Makefile
    go.mod
```

模板开箱即用地提供：

- **Kratos + Wire**：gRPC/HTTP 服务启动、依赖注入、统一中间件。
- **配置加载器**：`configloader` 支持 YAML + `.env`，环境变量覆盖后进行 protovalidate 校验。
- **数据库 & Outbox**：`pgx/sqlc` 访问 Postgres，内置 Outbox + 投影任务骨架。
- **统一脚本**：`make fmt/lint/test/build`、`buf`/`spectral` 校验、`wire` 生成。
- **端到端脚本**：`test/full_e2e_projection.sh` 模拟创建/更新视频并验证投影结果。

---

## 配置流程

```mermaid
flowchart LR
    A[config.yaml] -->|基础配置| B(configloader.Load)
    A2(.env/.env.local/configs/.env) -->|环境变量| B
    B --> C{apply env overrides}
    C --> D((protovalidate))
    D --> E[RuntimeConfig]
    E -->|Wire ProviderSet| F[cmd/grpc/main.go]
    F --> G[Kratos App]
```

- 默认读取 `configs/config.yaml` 与 `configs/.env`（若存在）。
- 环境变量优先级最高，可覆盖 YAML。
- 通过 protovalidate 进行结构化校验，生成 `RuntimeConfig`，在 Wire 中拆分为 Logger、Observability、Database、Messaging 等依赖。

---

## Wire 依赖拓扑

```mermaid
flowchart LR
    subgraph Wire Providers
      A[configloader.ProviderSet]
      B[gclog.ProviderSet]
      C[gcjwt.ProviderSet]
      D[observability.ProviderSet]
      E[pgxpoolx.ProviderSet]
      F[txmanager.ProviderSet]
      G[gcpubsub.ProviderSet]
      H[grpc_server.ProviderSet]
      I[repositories.ProviderSet]
      J[services.ProviderSet]
      K[controllers.ProviderSet]
      L[tasks.outbox ProvideRunner]
      M[tasks.projection ProvideTask]
    end

    A --> B
    A --> C
    A --> D
    A --> E
    A --> G

    E --> F
    B & C & D & E & F & G --> H
    E --> I
    F --> I
    I --> J
    J --> K
    I --> L
    I --> M
    J --> L
    J --> M
    H & K & L & M --> N[newApp]
    N --> O[kratos.App]
```

> `wire ./cmd/grpc` 会根据上述依赖图生成 `wire_gen.go`：先加载 `RuntimeConfig`，再初始化日志、JWT、观测、数据库、Outbox/投影任务，最终组装成 `kratos.App`。

---

## 目录结构详解

| 目录/文件 | 说明 | 常见内容 |
| ---------- | ---- | -------- |
| `cmd/grpc/` | Kratos gRPC 服务入口（必备）。`main.go` 调用 Wire 生成的 `wireApp`，组装所有依赖并运行；`wire.go` 列出 ProviderSet。 | `main.go`, `wire.go`, `wire_gen.go` |
| `cmd/http/` | 可选 HTTP/REST 调试入口（默认不开启）。需要启用时，可复制 `cmd/grpc` 并调整中间件与监听端口。 | `main.go`（可选） |
| `configs/` | 配置目录：`config.yaml` 提供基础配置，`.env` 存放环境变量覆盖项。configloader 与脚本会自动加载。 | `config.yaml`, `.env` |
| `internal/controllers/` | Kratos gRPC/HTTP Handler，只负责解析请求、鉴权校验、调用 Service、输出 Problem/ETag/分页。 | `video_command_handler.go`, `video_query_handler.go`, `dto/` |
| `internal/services/` | 业务用例层：事务、幂等、组合仓储与外部客户端；生成领域事件并写入 Outbox。 | `video_command_service.go`, `video_query_service.go`, `video_types.go` |
| `internal/repositories/` | 数据访问层：通过 `pgx/sqlc` 操作 Postgres，或整合 GCS、Pub/Sub、第三方 API。所有 SQL 生成物位于 `internal/repositories/sqlc/`。 | `video_repo.go`, `outbox_repo.go`, `sqlc/` |
| `internal/models/po` | Plain Object（数据库映射对象），通常由 `sqlc` 生成或手动维护。 | `video.go` |
| `internal/models/vo` | View Object，面向上层返回或跨服务传输的数据结构。 | `video.go` |
| `internal/clients/` | 访问其他服务/第三方的客户端封装，统一处理鉴权、超时、重试。需要注入时在目录下提供 `init.go` 注册 Wire Provider。 | `grpc_client.go`, `init.go`（可选） |
| `internal/infrastructure/` | 底层设施：配置加载、Kratos server/client、日志、追踪、消息队列、JWT 等。服务启动时由 Wire 注入。 | `configloader/`, `grpc_server/`, `grpc_client/` |
| `internal/tasks/` | Outbox 发布器、投影消费者、定时/延迟任务。所有任务必须监听 `ctx.Done()` 并保证幂等。 | `outbox/`, `projection/` |
| `api/openapi/` | REST 契约（YAML）。使用 `spectral lint api/openapi/*.yaml` 校验。 | `video.yaml` 等 |
| `api/proto/` | gRPC Proto 契约。使用 `buf lint` / `buf breaking` 管理，生成代码位于 `api/video/v1/*.pb.go`。 | `video.proto`, `events.proto` |
| `migrations/` | SQL 迁移脚本（服务独有 schema）。开发期可通过 `psql` 执行，或结合 `golang-migrate/atlas`。 | `001_init_catalog_schema.sql` 等 |
| `test/` | 集成/端到端脚本。模板内提供 `full_e2e_projection.sh` 验证 Outbox → 投影链路。 | `full_e2e_projection.sh` |
| `Makefile` | 常用命令聚合：格式化、静态检查、测试、构建、代码生成等。 | `fmt`, `lint`, `test`, `build`, `proto`, `openapi` |
| `go.mod` / `go.sum` | Go Module 配置。派生仓库需更新 module 名称并在初始化后运行 `go mod tidy`。 | — |
示例目录树（核心文件）：

```
├── cmd/
│   ├── grpc/main.go          # Kratos gRPC 入口，解析配置并调用 wire 生成的依赖
│   └── grpc/wire.go          # 定义 ProviderSet，wire 自动生成 wire_gen.go
├── configs/config.yaml       # 基础配置（gRPC 监听、Database、Observability、Messaging 等）
├── configs/.env              # 环境变量覆盖项，推荐只在本地保存（勿提交敏感信息）
├── internal/controllers/     # Handler；按领域拆分文件/子目录，也包含 dto/ 响应转换
├── internal/services/        # 业务用例实现，组合仓储/客户端，统一返回 VO 或 Problem
├── internal/repositories/    # 数据访问与映射（含 sqlc 生成代码、Outbox/InBox 仓储）
├── internal/models/po        # 数据库存储模型（Plain Object）
├── internal/models/vo        # 视图模型（View Object/DTO）
├── internal/clients/         # 外部/跨服务客户端封装与 Wire Provider
├── internal/infrastructure/  # configloader、grpc server/client、JWT、Pub/Sub 等底层设施
├── internal/tasks/           # Outbox 发布器、投影消费者、定时任务实现
├── api/openapi/              # REST 契约，运行 spectral lint 校验
├── api/proto/                # gRPC Proto 契约，使用 buf lint/breaking 守护
├── migrations/               # SQL 迁移脚本，遵循递增编号
├── test/full_e2e_projection.sh # 端到端脚本：启动服务、调用 gRPC、验证投影/Outbox
├── Makefile                  # 常用命令入口（fmt/lint/test/build/proto/openapi）
├── go.mod / go.sum           # Go module 定义与依赖锁定
└── buf.yaml / buf.gen.yaml   # buf 配置与生成模板
```



> 建议将此 README 作为团队风格指南：新建仓库后保持上述职责划分一致，便于脚手架和自动化工具复用。


---

## 快速开始

### 1. 克隆并初始化

```bash
git clone --depth=1 git@github.com:your-org/kratos-template.git your-service
cd your-service
rm -rf .git && git init
go mod edit -module github.com/your-org/your-service
go mod tidy
```

### 2. 配置环境

1. 复制或创建 `configs/.env`，至少设置 `DATABASE_URL`：
   ```env
   DATABASE_URL=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable&search_path=catalog
   SERVICE_NAME=your-service
   SERVICE_VERSION=0.1.0
   APP_ENV=development
   ```
2. 如需本地 Pub/Sub / Kafka，可在 `.env` 中追加相关配置。

### 3. 安装工具

```bash
make init    # 安装 buf、wire、protoc 插件等
```

### 4. 运行服务

```bash
go run ./cmd/grpc -conf configs/config.yaml
# 或
make build && ./bin/grpc
```

---

## 常用命令

| 命令 | 作用 |
| ---- | ---- |
| `make init`  | 安装 buf、wire、protoc 插件等依赖工具 |
| `make fmt`   | `gofumpt` + `goimports` + `go mod tidy` |
| `make lint`  | `staticcheck` + `revive` + `buf lint` + `spectral lint` |
| `make test`  | `go test ./...`（默认包含 race/free） |
| `make build` | 构建 `./bin/grpc` |
| `buf generate` | 生成 gRPC 代码 |
| `spectral lint api/openapi/*.yaml` | 校验 REST 契约 |
| `wire ./cmd/grpc` | 重新生成依赖注入代码 |

> 执行任何提交前，务必确保 `make fmt lint test` 全部通过。

---

## 数据库与迁移

迁移脚本位于 `migrations/`，推荐使用 `golang-migrate` 或 `atlas`：

```bash
psql "$DATABASE_URL" -f migrations/001_init_catalog_schema.sql
psql "$DATABASE_URL" -f migrations/002_create_catalog_event_tables.sql
# ...
```

如需 Testcontainers 集成测试，可参考模板中 `internal/repositories/test` 示例。

---

## 测试策略

| 层级 | 目标 | 推荐工具 |
| ---- | ---- | -------- |
| Controllers | 验证 Problem/鉴权/ETag 分支 | `httptest`, `testify/require` |
| Services | 业务逻辑、事务、幂等（覆盖率 ≥ 80%） | `testing`, gomock/testify |
| Repositories | SQL/外部服务集成 | Testcontainers + Supabase/Postgres |
| Tasks | Outbox 发布、重试、死信 | Fake dispatcher + 集成测试 |
| 契约 | REST & gRPC 一致性 | `spectral lint`, `buf lint`, `buf breaking` |

### 端到端脚本

`test/full_e2e_projection.sh` 会：

1. 自动加载 `configs/.env`，回收占用端口；
2. 启动 `cmd/grpc` 并等待端口就绪；
3. 执行 Create/Update gRPC 调用；
4. 检查投影表与 Outbox 条目；
5. 优雅关闭服务并清理临时文件。

运行方式：

```bash
./test/full_e2e_projection.sh
```

脚本成功会输出：

```
✅ 全链路验证成功：video_id=..., outbox_events=3
```

---

## 自定义指南

1. **模块名 & 依赖**：调整 `go.mod` 后，更新 import 路径与 README。
2. **新增 API**：同时维护 `api/openapi` 和/或 `api/proto`，并运行 `make lint`。
3. **数据库扩展**：在 `migrations/` 编写 SQL，同步更新 `internal/models/po` 与 `sqlc` 查询。
4. **外部客户端**：在 `internal/clients` 创建子包并暴露 Wire Provider，Service 层通过接口注入。
5. **任务/Outbox**：在 `internal/tasks` 注册消费者，重用 `lingo-utils/outbox` 的 Runner 与配置。
6. **日志 & Trace**：使用 `lingo-utils/gclog`，确保敏感信息脱敏；通过 OTel 导出追踪与指标。

---

## 生产部署提示

- 容器镜像构建直接使用 `cmd/grpc`，通过环境变量注入配置。
- `DATABASE_URL`、Pub/Sub/Kafka 地址、GCS 凭证等敏感配置建议使用 Secret Manager / Vault。
- 配合 Cloud Run/GKE 时，可复用模板内的健康检查与优雅停机逻辑。
- 建议在 CI 中执行：`make fmt lint test` → `buf lint` → 构建镜像 → 部署。

---

## 常见问题

**Q: 为什么服务启动时报 `data.postgres.dsn` 校验失败？**  
A: 模板要求在 `.env`（或环境变量）中设置 `DATABASE_URL`；configloader 会在 protovalidate 前应用覆盖，否则会报错。

**Q: 如何调试 gRPC？**  
A: 安装 `grpcurl`，例如：
```bash
grpcurl -plaintext -d '{"video_id":"..."}' localhost:9000 video.v1.VideoQueryService/GetVideoDetail
```

**Q: 如何修改监听端口？**  
A: 在 `.env` 设置 `PORT=9100`（或任意值）；`full_e2e_projection.sh` 会自动回收占用该端口的旧进程。

---

## 参考资源

- [Go-Kratos 官方文档](https://go-kratos.dev/)
- [buf.build](https://buf.build/)
- [lingo-utils](https://github.com/bionicotaku/lingo-utils)（内部复用库：配置、日志、Outbox、JWT 等）
- [Supabase Postgres](https://supabase.com/)
- [Google Cloud Pub/Sub Emulator](https://cloud.google.com/pubsub/docs/emulator)

如需更多背景与架构说明，可参考根目录 `docs/` 目录或项目概述文档。欢迎基于模板扩展并提交改进建议。祝开发顺利！
