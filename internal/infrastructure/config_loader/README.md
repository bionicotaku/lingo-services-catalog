# Configuration Schema

- `pb/conf.proto` 定义配置结构（Bootstrap/Server/Data 等字段），运行 `buf generate --path internal/infrastructure/config_loader/pb` 会在 `pb/` 下生成强类型结构 `conf.pb.go` 与 PGV 校验代码。
- Kratos 的配置加载器（见 `loader.go`）会读取 YAML/TOML/JSON 配置并调用 `config.Scan(&configpb.Bootstrap{})`，将内容填充到这些结构体中，提供类型安全的访问。
- 当需要新增配置项时，修改 `pb/conf.proto` 并重新生成即可，业务代码通过 `configpb.Bootstrap` 获取最新字段。
