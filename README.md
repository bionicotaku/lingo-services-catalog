# Kratos Project Template 目录说明

本模板基于 go-kratos 官方骨架，配合 Buf 做 proto 依赖与代码生成管理。以下对每个目录与核心文件逐一说明，便于在现有骨架上扩展真实业务。

## 根目录文件

- `README.md`：当前文档，概览整个模板结构与各层职责，可在接入真实业务前更新本说明。
- `LICENSE`：MIT 协议文本，继承上游 go-kratos 项目的授权条款。
- `Makefile`：集中管理常用任务。`make init` 安装开发所需工具（含 Buf/kratos/protoc 插件），`make api` 与 `make config` 通过 Buf 生成 gRPC/HTTP/OpenAPI 代码，`make build` 则输出二进制到 `bin/`。
- `buf.yaml`：Buf 工作区配置，声明本仓库为单模块并引入 `googleapis` 作为远程依赖，统一 lint/breaking 规则。
- `buf.gen.yaml`：Buf 生成规则，配置 `go`、`go-grpc`、`go-http`、`openapi` 四类插件及输出目录，其中 OpenAPI 文档会写入 `api/openapi.yaml`。
- `buf.lock`：Buf 依赖锁文件，记录精确的远程 proto 版本，保证不同环境下生成一致。
- `go.mod` / `go.sum`：Go Module 与依赖锁定文件，模块名默认是 `github.com/go-kratos/kratos-layout`，落地业务时可按需修改。
- `Dockerfile`：多阶段构建镜像示例，Stage1 使用官方 Go 镜像编译，Stage2 基于 debian slim 运行产物并暴露 8000/9000 端口。

## API 层（`api/`）

- `api/helloworld/v1/*.proto`：示例 gRPC 契约，当前仅包含 `Greeter` 场景与错误枚举，展示如何声明 RPC 及 HTTP 注解。
- `api/helloworld/v1/*_pb.go` / `*_grpc.pb.go` / `*_http.pb.go`：通过 `buf generate --path api` 自动生成的 Go 代码，分别用于消息结构、gRPC 服务端接口与 HTTP 适配层。
- `api/openapi.yaml`：由 `protoc-gen-openapi` 生成的 REST 契约文档，可被 Swagger UI 或工具链消费。

## 入口层（`cmd/`）

- `cmd/server/main.go`：服务启动入口，加载配置、初始化日志、执行 Wire 注入并启动 HTTP/gRPC Server。
- `cmd/server/wire.go` / `wire_gen.go`：依赖注入配置与自动生成文件。开发时修改 `wire.go` 声明 ProviderSet，执行 `wire` 重新生成 `wire_gen.go`。

## 配置（`configs/`）

- `configs/config.yaml`：本地样例配置，展示 HTTP/GRPC 监听地址与数据源参数。`make run` 或二进制启动时可通过 `-conf` 指定目录。

## 内部实现（`internal/`）

- `internal/biz/`：
  - `biz.go`：收集业务 ProviderSet。
  - `greeter.go`：示例 Usecase 与领域接口定义，强调通过依赖倒置让数据访问层实现接口。
- `internal/conf/`：
  - `conf.proto`：配置 schema，采用 proto 定义以便生成强类型结构体。
  - `conf.pb.go`：由 Buf 生成的配置结构体，可被 `config.Scan` 直接填充。
- `internal/data/`：
  - `data.go`：封装数据库等基础资源的生命周期，并暴露 ProviderSet。
  - `greeter.go`：`GreeterRepo` 的具体实现示例，当前为 stub，展示如何连接 biz Usecase。
- `internal/server/`：
  - `grpc.go` / `http.go`：分别创建 gRPC 与 HTTP Server，挂载恢复中间件与配置项。
  - `server.go`：Server ProviderSet，方便 Wire 装配。
- `internal/service/`：
  - `service.go`：Service ProviderSet。
  - `greeter.go`：实现 gRPC/HTTP Service，承担 DTO ↔ 用例的转换。

## 其它

- （已移除）`third_party/`：原模板中的本地 proto 依赖目录已由 Buf 远程依赖代替，如需新增第三方 proto，请在 `buf.yaml` 的 `deps` 中声明并执行 `buf dep update`。

```text
├── Dockerfile           // 多阶段构建示例（Go 编译阶段 + Debian 运行阶段）
├── LICENSE              // 模板沿用的 MIT 授权文本
├── Makefile             // 常用构建/生成命令集合（init、api、config 等）
├── README.md            // 本文件，记录结构与使用说明
├── api                  // Proto 契约与生成代码所在目录
│   ├── helloworld       // 示例服务命名空间
│   │   └── v1           // API 版本目录
│   │       ├── error_reason.pb.go    // 错误枚举生成代码
│   │       ├── error_reason.proto    // 错误枚举 proto 定义
│   │       ├── greeter.pb.go         // Greeter 消息结构生成代码
│   │       ├── greeter.proto         // Greeter 服务 proto 契约
│   │       ├── greeter_grpc.pb.go    // Greeter gRPC 服务器/客户端桩
│   │       └── greeter_http.pb.go    // Greeter HTTP 适配层（google.api.http 注解）
│   └── openapi.yaml     // 自动生成的 REST OpenAPI 文档
├── buf.gen.yaml         // Buf 代码生成配置（插件与输出位置）
├── buf.lock             // Buf 依赖锁定文件，固定远程 proto 版本
├── buf.yaml             // Buf 模块声明与 lint/breaking 规则
├── cmd                  // 应用入口与依赖注入装配
│   └── server           // 单进程服务入口
│       ├── main.go      // 程序入口：加载配置并运行 HTTP/gRPC
│       ├── wire.go      // Wire 依赖注入定义
│       └── wire_gen.go  // Wire 自动生成的装配实现
├── configs              // 样例配置目录
│   └── config.yaml      // HTTP/GRPC 与数据源示例配置
├── go.mod               // Go Module 元数据
├── go.sum               // Go 依赖版本哈希
├── internal             // 服务内部实现（对外不可见）
│   ├── biz              // 业务用例层，定义领域接口与用例
│   │   ├── README.md    // 层级说明
│   │   ├── biz.go       // Biz ProviderSet
│   │   └── greeter.go   // Greeter 用例及仓储接口
│   ├── conf             // 配置 schema 定义与生成代码
│   │   ├── conf.pb.go   // 配置结构体生成代码
│   │   └── conf.proto   // 配置 proto 契约
│   ├── data             // 数据访问实现层（Repo）
│   │   ├── README.md    // 层级说明
│   │   ├── data.go      // 数据资源初始化
│   │   └── greeter.go   // Greeter 仓储实现示例
│   ├── server           // 传输层服务器配置
│   │   ├── grpc.go      // gRPC Server 初始化
│   │   ├── http.go      // HTTP Server 初始化
│   │   └── server.go    // Server ProviderSet
│   └── service          // gRPC/HTTP 服务实现层
│       ├── README.md    // 层级说明
│       ├── greeter.go   // Greeter Service，实现用例编排
│       └── service.go   // Service ProviderSet
└── (bin/)               // 执行 make build 后生成的二进制输出目录（默认忽略）
```

以上结构提供了一个最小可行的 Kratos 微服务骨架。开发真实业务时，可在此基础上扩展 proto 契约、补全 data 层与 Usecase，实现自定义领域逻辑与配套测试。*** End Patch​
