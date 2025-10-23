# Kratos Template · Cloud Run JWT 集成路线图

> 目标：在 `kratos-template` 服务中集成 `lingo-utils/gcjwt` 组件，为 Cloud Run 内部微服务 gRPC 调用提供自动化 ID Token 获取与校验能力。以下为分阶段实施清单，按顺序执行可逐步完成落地与验证。

---

## 阶段 0 · 环境准备
- [ ] **确认依赖版本**
  - Go 1.22+。
  - `github.com/go-kratos/kratos/v2` ≥ v2.9.1。
  - `github.com/bionicotaku/lingo-utils/gcjwt` 最新主线。
- [ ] **同步模块依赖**
  ```bash
  cd kratos-template
  go get github.com/bionicotaku/lingo-utils/gcjwt@latest
  go mod tidy
  ```
- [ ] **协同文档**  
  记录 PR / 需求编号，确保 README/配置变更同步更新。

---

## 阶段 1 · 配置 Schema 扩展
- [x] **Proto 增加字段**（`internal/infrastructure/config_loader/pb/conf.proto`）
  - `Server.JWT`：`expected_audience`、`skip_validate`、`required`、`header_key`。
  - `Data.Client.JWT`：`audience`、`disabled`、`header_key`。
  - 为新增字段添加 PGV 规则（例如 `min_len` 校验）。
- [x] **重新生成配置代码**
  ```bash
  make config  # 或直接 protoc/buf generate
  ```
- [x] **默认值注入**（`config_loader/defaults.go`）
  - 若未配置 `expected_audience`，开发模式可置空并将 `skip_validate` 默认 true。
  - 客户端默认 `disabled: true`，防止未配置的服务强制获取 Token。
- [x] **配置示例更新**
  - `configs/config.yaml`：添加 `server.jwt.*` 与 `data.grpc_client.jwt.*` 注释、占位值。
  - `configs/config.instance-*.yaml`：根据实际环境填写 audience。

---

## 阶段 2 · Wire Provider 调整
- [x] **引入 ProviderSet**
  - `cmd/grpc/wire.go`：`import github.com/bionicotaku/lingo-utils/gcjwt` 并在 `wire.NewSet` 中加入 `gcjwt.ProviderSet`。
- [x] **配置装配**
  - `internal/infrastructure/config_loader/provider.go`：
    - 新增 `ProvideJWTServerConfig(*configpb.Server) (*gcjwt.ServerConfig, error)`。
    - 新增 `ProvideJWTClientConfig(*configpb.Data) (*gcjwt.ClientConfig, error)`.
  - 在 `cmd/grpc/wire.go`、`wire_gen.go` 订正函数签名，确保 `ProvideServerMiddleware` 与 `ProvideClientMiddleware` 注入成功。
- [x] **运行 wire 生成**
  ```bash
  cd cmd/grpc
  wire
  ```

---

- [x] **修改 `grpc_server/grpc_server.go`**
  - 在 `grpc.Middleware` 链中插入 `gcjwt.Server(...)`，建议位置：`metadata.Server` 与 `ratelimit.Server` 之间。
  - 选项来源：`gcjwt.WithExpectedAudience(cfg.ExpectedAudience)`、`gcjwt.WithServerLogger(logger)`、`gcjwt.WithSkipValidate(cfg.SkipValidate)`、`gcjwt.WithTokenRequired(cfg.Required)`、`gcjwt.WithServerHeaderKey(cfg.HeaderKey)`。
- [x] **注入配置**
  - `grpc_server/init.go` 或 Wire 构造函数中将 `*gcjwt.ServerConfig` 传给 `ProvideServerMiddleware`。
- [ ] **启动验证**
  - `make run`（或 `go run ./cmd/grpc`）确保启动成功、不 panic。

---

## 阶段 4 · gRPC Client 中间件接入
- [ ] **修改 `grpc_client/grpc_client.go`**
  - 在 `kgrpc.WithMiddleware(...)` 列表中追加 `gcjwt.Client(...)`。
  - 根据配置设置：`WithAudience`、`WithClientLogger`、`WithHeaderKey`，若 `cfg.Disabled` 则追加 `WithClientDisabled(true)`。
- [ ] **注入配置**
  - Wire 中确保 `ProvideClientMiddleware` 获取 `*gcjwt.ClientConfig`。
- [ ] **兼容无下游场景**
  - 当 `grpc_client.target` 为空时，继续返回 `nil` 连接，此时 `ProvideClientMiddleware` 不应 panic：可在获取配置前判断 `cfg == nil || cfg.Disabled`。

---

## 阶段 5 · 自动化测试
- [ ] **单元测试覆盖**
  - `internal/infrastructure/grpc_server/test`：新增用例验证携带/缺失 Token、audience mismatch、skipValidate 情况。
  - `internal/infrastructure/grpc_client/test`：验证 Header 注入、禁用模式、Token 获取失败。
- [ ] **集成测试（可选）**
  - 利用 `gcjwt/test` 的 mock 或本地 `gcloud auth print-identity-token` 构造请求，确认完整链路。
- [ ] **运行测试**
  ```bash
  go test ./...
  ```

---

## 阶段 6 · 文档与脚本更新
- [ ] `kratos-template/README.md`：新增“服务间鉴权”章节，说明如何配置 Audience 与在 Cloud Run 上部署。
- [ ] `configs/` 示例文件注释，强调生产必须配置真实 audience、开发可通过 `skip_validate`/`disabled` 放宽。
- [ ] `scripts/`（可选）：添加 `debug_token.sh` 脚本，输出 Token payload 以便排查。

---

## 阶段 7 · 验收清单
- [ ] 项目可在开发环境加载配置且成功启动 gRPC 服务。
- [ ] 客户端请求携带 Cloud Run ID Token，可通过 `FromContext` 获取调用方 email。
- [ ] 未配置/配置错误时能收到明确的 401 响应或启动期错误提示。
- [ ] `go test ./...`、`make lint` 均通过。
- [ ] README、配置文件与 TODO 文档保持一致。

完毕后即可将集成方案推广到其余微服务，参考此文档执行相同步骤。完成后请在仓库根 README 或变更记录中注明，引导团队统一使用 gcjwt 方案。
