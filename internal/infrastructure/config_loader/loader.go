// Package loader 提供配置加载工具，负责从 YAML/TOML/JSON 文件解析配置并生成类型安全的配置对象。
// 支持通过命令行参数、环境变量控制配置路径，并自动推导服务元信息。
package loader

import (
	"fmt"
	"os"
	"time"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	"github.com/bionicotaku/lingo-utils/gclog"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ServiceMetadata 保存服务标识信息，供日志和可观测性组件使用。
type ServiceMetadata struct {
	Name        string // 服务名称（来自编译期 -ldflags 或默认值）
	Version     string // 服务版本（来自编译期 -ldflags 或默认值）
	Environment string // 运行环境（来自 APP_ENV 环境变量或默认值 "development"）
	InstanceID  string // 实例 ID（来自主机名或默认值）
}

// Params 包含构造配置 Bundle 所需的运行时输入参数。
type Params struct {
	ConfPath       string // 配置文件路径（可为空，使用默认值）
	ServiceName    string // 服务名称（可为空，使用默认值）
	ServiceVersion string // 服务版本（可为空，使用默认值）
}

// ObservabilityInfo 将服务元信息转换为 observability.ServiceInfo。
// 用于 Provider 中向 observability 组件传递服务标识。
func (m ServiceMetadata) ObservabilityInfo() obswire.ServiceInfo {
	return obswire.ServiceInfo{
		Name:        m.Name,
		Version:     m.Version,
		Environment: m.Environment,
	}
}

// LoggerConfig 将服务元信息转换为 gclog.Config，应用一致的默认值。
// 自动添加 service.id 标签，启用源码位置记录。
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

// Bundle 聚合强类型的配置片段，供下游 Wire 注入使用。
type Bundle struct {
	Bootstrap *configpb.Bootstrap         // Proto 定义的配置结构（Server/Data/Observability）
	ObsConfig obswire.ObservabilityConfig // 规范化的可观测性配置
	Service   ServiceMetadata             // 服务元信息
}

// BuildError 捕获配置构建过程中的上下文错误信息。
// 实现 error 接口，并支持 errors.Unwrap。
type BuildError struct {
	Stage string // 错误阶段：load（加载）、scan（解析）、validate（校验）
	Path  string // 配置文件路径
	Err   error  // 底层错误
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

// ResolveConfPath 应用回退规则确定要加载的配置目录/文件路径。
// 优先级：显式传入的路径 > CONF_PATH 环境变量 > 默认路径。
func ResolveConfPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv(envConfPath); env != "" {
		return env
	}
	return defaultConfPath
}

// Build 从 bootstrap 配置文件构建 Bundle，包含配置对象和服务元信息。
//
// 流程：
// 1. 解析配置路径（应用回退规则）
// 2. 加载 YAML/TOML/JSON 配置文件到 Kratos Config
// 3. 反序列化为 Proto 定义的 Bootstrap 结构
// 4. 执行 Proto-Gen-Validate 校验
// 5. 推导服务元信息（Name/Version/Environment/InstanceID）
// 6. 转换 Observability 配置为规范化结构
//
// 注意：不支持热重载，配置加载后立即关闭 loader。
func Build(params Params) (*Bundle, error) {
	confPath := ResolveConfPath(params.ConfPath)
	// Build Kratos config loader backed by file source (supports YAML/TOML/JSON under the conf path).
	c := config.New(config.WithSource(file.NewSource(confPath)))
	if err := c.Load(); err != nil {
		return nil, BuildError{Stage: "load", Path: confPath, Err: err}
	}
	var bc configpb.Bootstrap
	if err := c.Scan(&bc); err != nil {
		c.Close()
		return nil, BuildError{Stage: "scan", Path: confPath, Err: err}
	}
	if err := bc.ValidateAll(); err != nil {
		c.Close()
		return nil, BuildError{Stage: "validate", Path: confPath, Err: err}
	}
	// No hot-reload support: configuration is fully materialized, close loader immediately.
	c.Close()
	serviceName := resolveServiceName(params.ServiceName)
	serviceVersion := resolveServiceVersion(params.ServiceVersion)
	env := resolveEnvironment(os.Getenv("APP_ENV"))
	host, _ := os.Hostname()
	host = resolveInstanceID(host)

	meta := ServiceMetadata{
		Name:        serviceName,
		Version:     serviceVersion,
		Environment: env,
		InstanceID:  host,
	}
	return &Bundle{
		Bootstrap: &bc,
		ObsConfig: toObservabilityConfig(bc.Observability),
		Service:   meta,
	}, nil
}

// toObservabilityConfig 将 Proto 定义的 Observability 配置转换为 observability 包的规范化结构。
// 处理默认值、时间格式转换和字段映射。
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

// cloneStringMap 深拷贝 map[string]string，避免配置对象间的意外修改。
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

// durationValue 将 Proto Duration 转换为 Go time.Duration，nil 时返回零值。
func durationValue(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}
