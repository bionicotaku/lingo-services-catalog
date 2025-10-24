package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	"github.com/bionicotaku/lingo-utils/gclog"
	obswire "github.com/bionicotaku/lingo-utils/observability"
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

	return &Bundle{
		Bootstrap: bootstrap,
		ObsConfig: obsCfg,
		Service:   meta,
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

func applyEnvOverrides(bc *configpb.Bootstrap) {
	if bc == nil {
		return
	}
	if dsn := os.Getenv(envDatabaseURL); dsn != "" {
		if data := bc.GetData(); data != nil {
			if pg := data.GetPostgres(); pg != nil {
				pg.Dsn = dsn
			}
		}
	}
}

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

func durationValue(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}
