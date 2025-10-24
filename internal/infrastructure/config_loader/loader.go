package loader

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	"github.com/bionicotaku/lingo-utils/gclog"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	txconfig "github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/bufbuild/protovalidate-go"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/joho/godotenv"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	envConfPath       = "CONF_PATH"
	envServiceName    = "SERVICE_NAME"
	envServiceVersion = "SERVICE_VERSION"
	envAppEnv         = "APP_ENV"
	envDatabaseURL    = "DATABASE_URL"
	envPort           = "PORT"
)

var envFileNames = []string{".env.local", ".env"}

// Params 包含构造配置 Bundle 所需的运行时输入参数。
type Params struct {
	ConfPath string // 配置文件路径（可为空，使用默认值）
}

// ServiceMetadata 保存服务标识信息，供日志和可观测性组件使用。
type ServiceMetadata struct {
	Name        string
	Version     string
	Environment string
	InstanceID  string
}

// Bundle 聚合强类型的配置片段，供下游 Wire 注入使用。
type Bundle struct {
	Bootstrap *configpb.Bootstrap
	ObsConfig obswire.ObservabilityConfig
	Service   ServiceMetadata
	TxConfig  txconfig.Config
}

// BuildError 捕获配置构建过程中的上下文错误信息。
type BuildError struct {
	Stage string
	Path  string
	Err   error
}

// Error 实现 error 接口，提供包含上下文的错误信息。
func (e BuildError) Error() string {
	if e.Stage == "" {
		return e.Err.Error()
	}
	if e.Path != "" {
		return fmt.Sprintf("config %s at %q: %v", e.Stage, e.Path, e.Err)
	}
	return fmt.Sprintf("config %s: %v", e.Stage, e.Err)
}

// Unwrap 暴露底层错误，支持 errors.Is/As 链式查询。
func (e BuildError) Unwrap() error {
	return e.Err
}

// ObservabilityInfo 将服务元信息转换为 observability.ServiceInfo。
func (m ServiceMetadata) ObservabilityInfo() obswire.ServiceInfo {
	return obswire.ServiceInfo{
		Name:        m.Name,
		Version:     m.Version,
		Environment: m.Environment,
	}
}

// LoggerConfig 将服务元信息转换为 gclog.Config。
func (m ServiceMetadata) LoggerConfig() gclog.Config {
	labels := map[string]string{}
	if m.InstanceID != "" {
		labels["service.id"] = m.InstanceID
	}
	return gclog.Config{
		Service:              m.Name,
		Version:              m.Version,
		Environment:          m.Environment,
		InstanceID:           m.InstanceID,
		StaticLabels:         labels,
		EnableSourceLocation: true,
	}
}

// Build 从 bootstrap 配置文件构建 Bundle，包含配置对象和服务元信息。
//
// 流程：
// 1. 解析配置路径（应用回退规则）
// 2. 加载配置并执行 protovalidate 校验
// 3. 推导服务元信息（来自环境变量/默认值）
// 4. 转换可观测性配置
func Build(params Params) (*Bundle, error) {
	confPath := ResolveConfPath(params.ConfPath)
	loadEnvFiles(confPath)

	bootstrap, err := loadBootstrap(confPath)
	if err != nil {
		return nil, err
	}

	meta := buildServiceMetadata()
	obsCfg := toObservabilityConfig(bootstrap.GetObservability())
	txCfg := toTxManagerConfig(bootstrap.GetData().GetPostgres())

	return &Bundle{
		Bootstrap: bootstrap,
		ObsConfig: obsCfg,
		Service:   meta,
		TxConfig:  txCfg,
	}, nil
}

// ResolveConfPath 应用回退规则确定要加载的配置目录/文件路径。
// 优先级：显式传入路径 > CONF_PATH 环境变量 > 默认路径。
func ResolveConfPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv(envConfPath); env != "" {
		return env
	}
	return defaultConfPath
}

// loadBootstrap 从指定路径加载并解析 Bootstrap 配置。
//
// 处理流程：
// 1. 使用 Kratos config 加载 YAML/JSON 配置文件
// 2. 将配置扫描到 Bootstrap protobuf 结构体
// 3. 应用环境变量覆盖（DATABASE_URL、PORT 等）
// 4. 使用 protovalidate 验证配置完整性和正确性
//
// 参数：
//   - confPath: 配置文件路径或目录（支持目录、文件、通配符）
//
// 返回：
//   - *configpb.Bootstrap: 加载并验证后的配置对象
//   - error: 加载、解析或验证失败时返回 BuildError
//
// 错误阶段：
//   - "load": 文件读取失败（文件不存在、权限不足）
//   - "scan": YAML/JSON 解析失败（格式错误、类型不匹配）
//   - "init_validator": protovalidate 初始化失败
//   - "validate": 配置验证失败（必填字段缺失、约束不满足）
func loadBootstrap(confPath string) (*configpb.Bootstrap, error) {
	c := config.New(config.WithSource(file.NewSource(confPath)))
	if err := c.Load(); err != nil {
		return nil, BuildError{Stage: "load", Path: confPath, Err: err}
	}
	defer c.Close()

	var bc configpb.Bootstrap
	if err := c.Scan(&bc); err != nil {
		return nil, BuildError{Stage: "scan", Path: confPath, Err: err}
	}
	applyEnvOverrides(&bc)

	// 使用 protovalidate 进行运行时验证
	validator, err := protovalidate.New()
	if err != nil {
		return nil, BuildError{Stage: "init_validator", Path: confPath, Err: err}
	}
	if err := validator.Validate(&bc); err != nil {
		return nil, BuildError{Stage: "validate", Path: confPath, Err: err}
	}
	return &bc, nil
}

// applyEnvOverrides 应用环境变量覆盖配置文件中的特定字段。
//
// 设计目标：
//   - 12-Factor App 原则：配置通过环境变量注入
//   - 保持配置文件简洁，敏感信息从环境变量读取
//   - 支持不同环境（开发/测试/生产）无需修改配置文件
//
// 支持的环境变量：
//
//   - DATABASE_URL: 覆盖 data.postgres.dsn（数据库连接字符串）
//     示例: postgresql://user:pass@host:5432/db?sslmode=require
//
//   - PORT: 覆盖 server.grpc.addr 的端口部分（保留 host）
//     示例: PORT=8080 -> "0.0.0.0:9000" 变为 "0.0.0.0:8080"
//     用途: Cloud Run 动态端口分配、本地开发多实例
//
// 参数：
//   - bc: Bootstrap 配置对象（nil-safe）
//
// 注意：
//   - 环境变量为空时不覆盖，保留配置文件原值
//   - 仅覆盖存在的配置节点，不会创建缺失的节点
func applyEnvOverrides(bc *configpb.Bootstrap) {
	if bc == nil {
		return
	}
	// 覆盖数据库连接字符串
	if dsn := os.Getenv(envDatabaseURL); dsn != "" {
		if data := bc.GetData(); data != nil {
			if pg := data.GetPostgres(); pg != nil {
				pg.Dsn = dsn
			}
		}
	}
	// 覆盖 gRPC 服务器监听端口（支持 Cloud Run $PORT）
	if port := os.Getenv(envPort); port != "" {
		if server := bc.GetServer(); server != nil {
			if grpc := server.GetGrpc(); grpc != nil {
				grpc.Addr = replacePort(grpc.GetAddr(), port)
			}
		}
	}
}

// buildServiceMetadata 构建服务元信息，用于日志、追踪和指标标签。
//
// 数据来源优先级：
// 1. 环境变量（SERVICE_NAME、SERVICE_VERSION、APP_ENV）
// 2. 默认值（name: "template", version: "dev", env: "development"）
//
// 元信息用途：
//   - 日志标签：service.name、service.version、environment
//   - 分布式追踪：Resource Attributes
//   - 指标标签：service_name、service_version、deployment_environment
//   - 实例标识：用于区分同一服务的不同实例（hostname 或 K8s pod name）
//
// 返回：
//   - ServiceMetadata: 包含 Name、Version、Environment、InstanceID
//
// 使用示例：
//
//	meta := buildServiceMetadata()
//	logger := gclog.New(meta.LoggerConfig())
//	tracer := otel.New(meta.ObservabilityInfo())
func buildServiceMetadata() ServiceMetadata {
	name := resolveServiceName(os.Getenv(envServiceName))
	version := resolveServiceVersion(os.Getenv(envServiceVersion))
	env := resolveEnvironment(os.Getenv(envAppEnv))
	host, _ := os.Hostname()
	host = resolveInstanceID(host)

	return ServiceMetadata{
		Name:        name,
		Version:     version,
		Environment: env,
		InstanceID:  host,
	}
}

// loadEnvFiles best-effort 加载配置相关的 .env 文件，失败时忽略以保持幂等。
func loadEnvFiles(confPath string) {
	files := envFileCandidates(confPath)
	if len(files) == 0 {
		return
	}
	_ = godotenv.Load(files...)
}

// envFileCandidates 搜索并返回所有可用的 .env 文件路径。
//
// 搜索策略：
// 1. 按优先级遍历目录：confPath 目录 -> 当前工作目录
// 2. 在每个目录中查找：.env.local（高优先级）、.env（低优先级）
// 3. 去重：同一文件路径仅保留第一次出现的位置
//
// 文件优先级：
//   - .env.local: 本地开发专用，通常在 .gitignore 中（不提交到版本控制）
//   - .env: 默认配置，提交到 git 仓库作为示例模板
//
// 参数：
//   - confPath: 配置文件路径（用于推导搜索目录）
//
// 返回：
//   - []string: 存在的 .env 文件的绝对路径列表（按优先级排序）
//
// 注意：
//   - 仅返回实际存在的文件（通过 os.Stat 验证）
//   - godotenv 会按列表顺序加载，后加载的文件不会覆盖已设置的变量
func envFileCandidates(confPath string) []string {
	dirs := orderedDirs(confPath)
	seen := make(map[string]struct{})
	var files []string
	for _, dir := range dirs {
		for _, name := range envFileNames {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err != nil {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			files = append(files, candidate)
			seen[candidate] = struct{}{}
		}
	}
	return files
}

// orderedDirs 按优先级返回用于搜索 .env 文件的目录列表。
//
// 目录优先级（从高到低）：
// 1. confPath 所在目录（如果 confPath 是文件，取其父目录）
// 2. 当前工作目录（os.Getwd）
//
// 去重策略：
//   - 使用 filepath.Clean 规范化路径
//   - 同一目录仅保留第一次出现的位置
//
// 参数：
//   - confPath: 配置文件路径或目录
//
// 返回：
//   - []string: 规范化后的目录路径列表（按优先级排序，已去重）
//
// 示例：
//
//	confPath = "/app/configs/config.yaml"
//	返回: ["/app/configs", "/app"]  // 假设 cwd 是 /app
//
//	confPath = "/app/configs"
//	返回: ["/app/configs", "/app"]
//
// 注意：
//   - 目录不存在不会报错，仅跳过
//   - 路径解析失败不会导致 panic，仅跳过该路径
func orderedDirs(confPath string) []string {
	var dirs []string
	appendUnique := func(path string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		for _, existing := range dirs {
			if existing == clean {
				return
			}
		}
		dirs = append(dirs, clean)
	}

	if confPath != "" {
		if info, err := os.Stat(confPath); err == nil {
			if info.IsDir() {
				appendUnique(confPath)
			} else {
				appendUnique(filepath.Dir(confPath))
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		appendUnique(cwd)
	}

	return dirs
}

// toObservabilityConfig 将 Proto 定义的 Observability 配置转换为 observability 包的规范化结构。
func toObservabilityConfig(src *configpb.Observability) obswire.ObservabilityConfig {
	if src == nil {
		return obswire.ObservabilityConfig{}
	}
	cfg := obswire.ObservabilityConfig{
		GlobalAttributes: cloneStringMap(src.GetGlobalAttributes()),
	}
	if tr := src.GetTracing(); tr != nil {
		cfg.Tracing = &obswire.TracingConfig{
			Enabled:            tr.GetEnabled(),
			Exporter:           tr.GetExporter(),
			Endpoint:           tr.GetEndpoint(),
			Headers:            cloneStringMap(tr.GetHeaders()),
			Insecure:           tr.GetInsecure(),
			SamplingRatio:      tr.GetSamplingRatio(),
			BatchTimeout:       durationValue(tr.GetBatchTimeout()),
			ExportTimeout:      durationValue(tr.GetExportTimeout()),
			MaxQueueSize:       int(tr.GetMaxQueueSize()),
			MaxExportBatchSize: int(tr.GetMaxExportBatchSize()),
			Required:           tr.GetRequired(),
			ServiceName:        tr.GetServiceName(),
			ServiceVersion:     tr.GetServiceVersion(),
			Environment:        tr.GetEnvironment(),
			Attributes:         cloneStringMap(tr.GetAttributes()),
		}
	}
	if mt := src.GetMetrics(); mt != nil {
		grpcEnabled := defaultGRPCMetricsEnabled
		if mt.GrpcEnabled != nil {
			grpcEnabled = mt.GetGrpcEnabled()
		}
		grpcIncludeHealth := defaultGRPCIncludeHealth
		if mt.GrpcIncludeHealth != nil {
			grpcIncludeHealth = mt.GetGrpcIncludeHealth()
		}
		cfg.Metrics = &obswire.MetricsConfig{
			Enabled:             mt.GetEnabled(),
			Exporter:            mt.GetExporter(),
			Endpoint:            mt.GetEndpoint(),
			Headers:             cloneStringMap(mt.GetHeaders()),
			Insecure:            mt.GetInsecure(),
			Interval:            durationValue(mt.GetInterval()),
			DisableRuntimeStats: mt.GetDisableRuntimeStats(),
			Required:            mt.GetRequired(),
			ResourceAttributes:  cloneStringMap(mt.GetResourceAttributes()),
			GRPCEnabled:         grpcEnabled,
			GRPCIncludeHealth:   grpcIncludeHealth,
		}
	}
	return cfg
}

// cloneStringMap 创建字符串映射的深拷贝，避免共享底层数据。
//
// 使用场景：
//   - 将 protobuf map 字段转换为 Go map 时避免意外修改
//   - 传递配置数据到可观测性组件时防止数据竞争
//
// 参数：
//   - src: 源 map（可为 nil 或空）
//
// 返回：
//   - map[string]string: 深拷贝的新 map，源为空时返回 nil（而非空 map）
//
// 性能：
//   - 预分配容量避免扩容：make(map, len(src))
//   - O(n) 时间复杂度，n 为键值对数量
func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// durationValue 将 protobuf Duration 转换为 Go time.Duration。
//
// 安全处理：
//   - nil 值返回零值（0），避免 panic
//   - 使用 protobuf 官方方法 AsDuration() 确保正确转换
//
// 参数：
//   - d: protobuf Duration 指针（可为 nil）
//
// 返回：
//   - time.Duration: 转换后的 Go 时间间隔，nil 时返回 0
//
// 使用示例：
//
//	timeout := durationValue(config.GetTimeout())
//	ctx, cancel := context.WithTimeout(ctx, timeout)
func durationValue(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}

// replacePort 替换地址中的端口部分，保留 host。
// 支持格式：
//   - "0.0.0.0:9090" -> "0.0.0.0:8080"
//   - ":9090" -> ":8080"
//   - "127.0.0.1:9090" -> "127.0.0.1:8080"
//   - "[::1]:9090" -> "[::1]:8080"
func replacePort(addr, newPort string) string {
	if addr == "" {
		return "0.0.0.0:" + newPort
	}

	// 使用 net.SplitHostPort 解析
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// 解析失败，可能是只有端口 ":9090" 或格式错误，尝试保留原值并追加端口
		return "0.0.0.0:" + newPort
	}

	// 重新组合 host:port
	return net.JoinHostPort(host, newPort)
}

func toTxManagerConfig(pg *configpb.Data_PostgreSQL) txconfig.Config {
	if pg == nil {
		return txconfig.Config{}
	}
	tx := pg.GetTransaction()
	if tx == nil {
		return txconfig.Config{}
	}

	cfg := txconfig.Config{
		DefaultIsolation: tx.GetDefaultIsolation(),
		MaxRetries:       int(tx.GetMaxRetries()),
	}
	if d := tx.GetDefaultTimeout(); d != nil {
		cfg.DefaultTimeout = d.AsDuration()
	}
	if d := tx.GetLockTimeout(); d != nil {
		cfg.LockTimeout = d.AsDuration()
	}
	if tx.MetricsEnabled != nil {
		v := tx.GetMetricsEnabled()
		cfg.MetricsEnabled = &v
	}
	return cfg
}
