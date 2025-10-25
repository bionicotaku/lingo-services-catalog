package loader

import (
	"fmt"
	"strings"
	"time"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/gclog"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	"github.com/bionicotaku/lingo-utils/pgxpoolx"
	txconfig "github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ProviderSet 向 Wire 依赖图暴露配置相关的所有 Provider 函数。
//
// 该 ProviderSet 提供：
//   - Bundle: 完整配置包（包含 Bootstrap、ObsConfig、ServiceMetadata）
//   - ServiceMetadata: 服务元信息（用于日志、追踪标签）
//   - Bootstrap: 强类型的 protobuf 配置根对象
//   - ServerConfig: 服务器配置（gRPC、JWT 等）
//   - DataConfig: 数据层配置（PostgreSQL、gRPC Client 等）
//   - ObservabilityConfig: 可观测性配置（追踪、指标）
//   - ObservabilityInfo: 观测组件所需的服务信息
//   - LoggerConfig: 日志组件所需的配置
//   - JWTConfig: JWT 中间件配置（Server + Client）
//
// Wire 使用示例：
//
//	wire.Build(
//	    configloader.ProviderSet,  // 注入配置相关依赖
//	    gclog.ProviderSet,         // 日志组件依赖 LoggerConfig
//	    observability.ProviderSet, // 观测组件依赖 ObservabilityConfig
//	    // ...
//	)
var ProviderSet = wire.NewSet(
	ProvideBundle,
	ProvideServiceMetadata,
	ProvideBootstrap,
	ProvideServerConfig,
	ProvideDataConfig,
	ProvideObservabilityConfig,
	ProvideObservabilityInfo,
	ProvideLoggerConfig,
	ProvidePgxPoolConfig,
	ProvideJWTConfig,
	ProvideTxManagerConfig,
	ProvideMessagingConfig,
	ProvidePubsubConfig,
	ProvidePubsubDependencies,
	ProvideOutboxPublisherConfig,
	ProvideProjectionConsumerConfig,
)

const (
	defaultOutboxBatchSize      = 100
	defaultOutboxTickInterval   = time.Second
	defaultOutboxInitialBackoff = 2 * time.Second
	defaultOutboxMaxBackoff     = 120 * time.Second
	defaultOutboxMaxAttempts    = 20
	defaultOutboxPublishTimeout = 10 * time.Second
	defaultOutboxWorkers        = 4
	defaultOutboxLockTTL        = 2 * time.Minute
)

// ProvideBundle 从运行时参数构造配置 Bundle。
//
// 该函数是配置加载的入口点，负责：
// 1. 解析配置文件路径
// 2. 加载并验证配置
// 3. 应用环境变量覆盖
// 4. 构建服务元信息
//
// 参数：
//   - p: Params 包含 ConfPath（配置文件路径）
//
// 返回：
//   - *Bundle: 包含 Bootstrap、ObsConfig、ServiceMetadata 的配置包
//   - error: 配置加载或验证失败时返回错误
//
// Wire 依赖：
//   - 输入：Params（由 main.go 或命令行参数提供）
//   - 输出：*Bundle（供下游 Provider 使用）
func ProvideBundle(p Params) (*Bundle, error) {
	return Build(p)
}

// ProvideServiceMetadata 从 Bundle 中提取服务元信息。
//
// 服务元信息用途：
//   - 日志组件：通过 LoggerConfig() 转换为 gclog.Config
//   - 观测组件：通过 ObservabilityInfo() 转换为 observability.ServiceInfo
//   - 指标标签：service_name、service_version、environment
//   - 实例标识：用于分布式环境下区分不同实例
//
// 参数：
//   - b: Bundle 配置包（nil-safe）
//
// 返回：
//   - ServiceMetadata: 包含 Name、Version、Environment、InstanceID
//     nil 时返回零值（空字符串）
func ProvideServiceMetadata(b *Bundle) ServiceMetadata {
	if b == nil {
		return ServiceMetadata{}
	}
	return b.Service
}

// ProvideBootstrap 暴露强类型的 Bootstrap 配置根对象。
//
// Bootstrap 是所有配置的根节点，包含：
//   - Server: 服务器配置（gRPC 地址、超时、JWT）
//   - Data: 数据层配置（PostgreSQL、gRPC Client）
//   - Observability: 可观测性配置（追踪、指标）
//
// 参数：
//   - b: Bundle 配置包（nil-safe）
//
// 返回：
//   - *configpb.Bootstrap: protobuf 定义的配置对象，nil 时返回 nil
//
// Wire 用途：
//   - 供其他 Provider 函数拆分为更细粒度的配置片段
//   - 避免直接依赖 Bundle，降低耦合度
func ProvideBootstrap(b *Bundle) *configpb.Bootstrap {
	if b == nil {
		return nil
	}
	return b.Bootstrap
}

// ProvideServerConfig 提取 Bootstrap 中的 Server 配置节。
//
// Server 配置包含：
//   - grpc.addr: gRPC 服务器监听地址（如 "0.0.0.0:9000"）
//   - grpc.timeout: 请求超时时间
//   - jwt: 服务端 JWT 验证配置（expected_audience、skip_validate 等）
//
// 参数：
//   - bc: Bootstrap 配置对象（nil-safe）
//
// 返回：
//   - *configpb.Server: 服务器配置，nil 时返回 nil
//
// Wire 依赖链：
//   - Bootstrap -> Server -> gRPC Server、JWT Server Middleware
func ProvideServerConfig(bc *configpb.Bootstrap) *configpb.Server {
	if bc == nil {
		return nil
	}
	return bc.GetServer()
}

// ProvideDataConfig 提取 Bootstrap 中的 Data 配置节。
//
// Data 配置包含：
//   - postgres: PostgreSQL 连接配置（DSN、连接池参数、schema）
//   - grpc_client: 出站 gRPC 客户端配置（target、jwt）
//
// 参数：
//   - bc: Bootstrap 配置对象（nil-safe）
//
// 返回：
//   - *configpb.Data: 数据层配置，nil 时返回 nil
//
// Wire 依赖链：
//   - Bootstrap -> Data -> Database Pool、gRPC Client、JWT Client Middleware
func ProvideDataConfig(bc *configpb.Bootstrap) *configpb.Data {
	if bc == nil {
		return nil
	}
	return bc.GetData()
}

// ProvideObservabilityConfig 暴露规范化后的可观测性配置。
//
// 规范化处理：
//   - 将 protobuf 配置转换为 observability 包所需的 Go 结构体
//   - 应用默认值（如 gRPC 指标默认启用）
//   - 转换 Duration 类型（protobuf.Duration -> time.Duration）
//
// 配置内容：
//   - GlobalAttributes: 全局标签（附加到所有追踪和指标）
//   - Tracing: 追踪配置（exporter、endpoint、sampling_ratio 等）
//   - Metrics: 指标配置（exporter、interval、gRPC 指标开关）
//
// 参数：
//   - b: Bundle 配置包（nil-safe）
//
// 返回：
//   - obswire.ObservabilityConfig: 规范化配置，nil 时返回零值
//
// Wire 依赖链：
//   - Bundle -> ObservabilityConfig -> observability.Component
func ProvideObservabilityConfig(b *Bundle) obswire.ObservabilityConfig {
	if b == nil {
		return obswire.ObservabilityConfig{}
	}
	return b.ObsConfig
}

// ProvideObservabilityInfo 将服务元信息转换为观测组件所需格式。
//
// 转换内容：
//   - ServiceMetadata.Name -> ServiceInfo.Name
//   - ServiceMetadata.Version -> ServiceInfo.Version
//   - ServiceMetadata.Environment -> ServiceInfo.Environment
//   - InstanceID 不传递（观测组件通过 hostname 自动获取）
//
// 用途：
//   - OpenTelemetry Resource Attributes
//   - Tracer/Meter 初始化时的服务标识
//
// 参数：
//   - meta: 服务元信息
//
// 返回：
//   - obswire.ServiceInfo: 观测组件所需的服务信息
//
// Wire 依赖链：
//   - ServiceMetadata -> ServiceInfo -> observability.Component
func ProvideObservabilityInfo(meta ServiceMetadata) obswire.ServiceInfo {
	return meta.ObservabilityInfo()
}

// ProvideMessagingConfig 提取 Messaging 配置节点。
func ProvideMessagingConfig(bc *configpb.Bootstrap) *configpb.Messaging {
	if bc == nil {
		return nil
	}
	return bc.GetMessaging()
}

// ProvidePubsubConfig 将 protobuf 配置转换为 gcpubsub.Config 并填充默认值。
func ProvidePubsubConfig(msg *configpb.Messaging) gcpubsub.Config {
	if msg == nil || msg.GetPubsub() == nil {
		return gcpubsub.Config{}
	}

	pc := msg.GetPubsub()
	receive := pc.GetReceive()
	receiveCfg := gcpubsub.ReceiveConfig{}
	if receive != nil {
		receiveCfg.NumGoroutines = int(receive.GetNumGoroutines())
		receiveCfg.MaxOutstandingMessages = int(receive.GetMaxOutstandingMessages())
		receiveCfg.MaxOutstandingBytes = int(receive.GetMaxOutstandingBytes())
		receiveCfg.MaxExtension = durationFromProto(receive.GetMaxExtension())
		receiveCfg.MaxExtensionPeriod = durationFromProto(receive.GetMaxExtensionPeriod())
	}

	cfg := gcpubsub.Config{
		ProjectID:           pc.GetProjectId(),
		TopicID:             pc.GetTopicId(),
		SubscriptionID:      pc.GetSubscriptionId(),
		PublishTimeout:      durationFromProto(pc.GetPublishTimeout()),
		OrderingKeyEnabled:  boolPtr(pc.GetOrderingKeyEnabled()),
		EnableLogging:       boolPtr(pc.GetLoggingEnabled()),
		EnableMetrics:       boolPtr(pc.GetMetricsEnabled()),
		MeterName:           "kratos-template.gcpubsub",
		EmulatorEndpoint:    pc.GetEmulatorEndpoint(),
		Receive:             receiveCfg,
		ExactlyOnceDelivery: pc.GetExactlyOnceDelivery(),
	}

	cfg = cfg.Normalize()

	required := map[string]string{
		"messaging.pubsub.project_id":      cfg.ProjectID,
		"messaging.pubsub.topic_id":        cfg.TopicID,
		"messaging.pubsub.subscription_id": cfg.SubscriptionID,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			panic(fmt.Errorf("%s is required; please update configs/config.yaml", field))
		}
	}

	return cfg
}

// ProvidePubsubDependencies 构造 gcpubsub.Dependencies。
func ProvidePubsubDependencies(logger log.Logger) gcpubsub.Dependencies {
	return gcpubsub.Dependencies{
		Logger: logger,
	}
}

// ProjectionConsumerConfig 描述 StreamingPull 投影消费者需要的附加配置。
type ProjectionConsumerConfig struct {
	DeadLetterTopicID string
}

// ProvideProjectionConsumerConfig 返回投影消费者配置。
func ProvideProjectionConsumerConfig(msg *configpb.Messaging) ProjectionConsumerConfig {
	if msg == nil || msg.GetPubsub() == nil {
		return ProjectionConsumerConfig{}
	}
	return ProjectionConsumerConfig{
		DeadLetterTopicID: msg.GetPubsub().GetDeadLetterTopicId(),
	}
}

// OutboxPublisherConfig 描述 Outbox 发布器的运行参数。
type OutboxPublisherConfig struct {
	BatchSize      int
	TickInterval   time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	MaxAttempts    int
	PublishTimeout time.Duration
	Workers        int
	LockTTL        time.Duration
}

// ProvideOutboxPublisherConfig 返回 Outbox 发布器配置，使用默认值兜底。
func ProvideOutboxPublisherConfig(msg *configpb.Messaging) OutboxPublisherConfig {
	cfg := OutboxPublisherConfig{
		BatchSize:      defaultOutboxBatchSize,
		TickInterval:   defaultOutboxTickInterval,
		InitialBackoff: defaultOutboxInitialBackoff,
		MaxBackoff:     defaultOutboxMaxBackoff,
		MaxAttempts:    defaultOutboxMaxAttempts,
		PublishTimeout: defaultOutboxPublishTimeout,
		Workers:        defaultOutboxWorkers,
		LockTTL:        defaultOutboxLockTTL,
	}
	if msg == nil || msg.GetOutbox() == nil {
		return cfg
	}

	ob := msg.GetOutbox()
	if ob.GetBatchSize() > 0 {
		cfg.BatchSize = int(ob.GetBatchSize())
	}
	if d := durationFromProto(ob.GetTickInterval()); d > 0 {
		cfg.TickInterval = d
	}
	if d := durationFromProto(ob.GetInitialBackoff()); d > 0 {
		cfg.InitialBackoff = d
	}
	if d := durationFromProto(ob.GetMaxBackoff()); d > 0 {
		cfg.MaxBackoff = d
	}
	if ob.GetMaxAttempts() > 0 {
		cfg.MaxAttempts = int(ob.GetMaxAttempts())
	}
	if d := durationFromProto(ob.GetPublishTimeout()); d > 0 {
		cfg.PublishTimeout = d
	}
	if ob.GetWorkers() > 0 {
		cfg.Workers = int(ob.GetWorkers())
	}
	if d := durationFromProto(ob.GetLockTtl()); d > 0 {
		cfg.LockTTL = d
	}
	return cfg
}

func durationFromProto(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}

func boolPtr(b bool) *bool {
	v := b
	return &v
}

// ProvideLoggerConfig 将服务元信息转换为日志组件所需格式。
//
// 转换内容：
//   - ServiceMetadata -> gclog.Config
//   - 自动添加 StaticLabels（如 service.id）
//   - 启用 SourceLocation（日志包含文件名和行号）
//
// 日志配置包含：
//   - Service: 服务名称
//   - Version: 服务版本
//   - Environment: 运行环境（development/staging/production）
//   - InstanceID: 实例标识（用于区分同一服务的不同实例）
//   - StaticLabels: 静态标签（附加到所有日志条目）
//
// 参数：
//   - meta: 服务元信息
//
// 返回：
//   - gclog.Config: 日志组件所需的配置
//
// Wire 依赖链：
//   - ServiceMetadata -> LoggerConfig -> gclog.Component -> log.Logger
func ProvideLoggerConfig(meta ServiceMetadata) gclog.Config {
	return meta.LoggerConfig()
}

// ProvidePgxPoolConfig 暴露 pgxpoolx 组件所需配置。
func ProvidePgxPoolConfig(b *Bundle) pgxpoolx.Config {
	if b == nil {
		return pgxpoolx.Config{}
	}
	return b.PgxConfig
}

// ProvideJWTConfig 从 Server 和 Data 配置中提取 JWT 配置并合并。
//
// JWT 配置分为两部分：
// 1. Server JWT（入站验证）：
//   - expected_audience: 期望的 JWT 受众（aud 字段）
//   - skip_validate: 是否跳过验证（本地开发用）
//   - required: 是否强制要求携带 Token（false 允许匿名请求）
//   - header_key: 从哪个 Header 读取 Token（默认 "authorization"）
//
// 2. Client JWT（出站注入）：
//   - audience: 目标服务的 URL（用于获取 Identity Token）
//   - disabled: 是否禁用中间件（本地开发用）
//   - header_key: 注入到哪个 Header（默认 "authorization"）
//
// 参数：
//   - server: Server 配置（可为 nil）
//   - data: Data 配置（可为 nil）
//
// 返回：
//   - gcjwt.Config: 包含 Server 和 Client 两部分的 JWT 配置
//     如果对应节点不存在，对应字段为 nil
//
// Wire 依赖链：
//   - Server + Data -> JWTConfig -> gcjwt.Component -> ServerMiddleware + ClientMiddleware
//
// 使用场景：
//   - Gateway: Server 验证用户 JWT，Client 注入 Identity Token 调用后端
//   - Catalog: Server 验证 Identity Token，Client 不启用（不调用其他服务）
func ProvideJWTConfig(server *configpb.Server, data *configpb.Data) gcjwt.Config {
	var cfg gcjwt.Config

	if server != nil {
		if jwt := server.GetJwt(); jwt != nil {
			cfg.Server = &gcjwt.ServerConfig{
				ExpectedAudience: jwt.GetExpectedAudience(),
				SkipValidate:     jwt.GetSkipValidate(),
				Required:         jwt.GetRequired(),
				HeaderKey:        jwt.GetHeaderKey(),
			}
		}
	}

	if data != nil && data.GetGrpcClient() != nil {
		if jwt := data.GetGrpcClient().GetJwt(); jwt != nil {
			cfg.Client = &gcjwt.ClientConfig{
				Audience:  jwt.GetAudience(),
				Disabled:  jwt.GetDisabled(),
				HeaderKey: jwt.GetHeaderKey(),
			}
		}
	}

	return cfg
}

// ProvideTxManagerConfig 暴露 TxManager 组件所需的配置。
func ProvideTxManagerConfig(b *Bundle) txconfig.Config {
	if b == nil {
		return txconfig.Config{}
	}
	return b.TxConfig
}
