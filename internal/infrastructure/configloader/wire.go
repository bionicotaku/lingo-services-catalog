package configloader

import (
	"context"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/gclog"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/pgxpoolx"
	txconfig "github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"github.com/bionicotaku/lingo-services-catalog/internal/controllers"
)

// EngagementPubSubConfig 包装 engagement 订阅所需的 gcpubsub.Config，避免与主 Pub/Sub 冲突。
type EngagementPubSubConfig gcpubsub.Config

// EngagementSubscriber 标签化 engagement 订阅者实例，避免与主订阅者冲突。
type EngagementSubscriber gcpubsub.Subscriber

// ProviderSet 暴露配置加载相关的依赖注入入口。
var ProviderSet = wire.NewSet(
	LoadRuntimeConfig,
	ProvideServiceInfo,
	ProvideLoggerConfig,
	ProvideObservabilityConfig,
	ProvideObservabilityInfo,
	ProvideServerConfig,
	ProvideDatabaseConfig,
	ProvidePgxConfig,
	ProvideTxConfig,
	ProvideJWTConfig,
	ProvideClientConfig,
	ProvideMessagingConfig,
	ProvidePubSubConfig,
	ProvideEngagementConfig,
	ProvidePubSubDependencies,
	ProvideEngagementSubscriber,
	ProvideOutboxConfig,
	ProvideHandlerTimeouts,
)

// LoadRuntimeConfig 调用 Load 并供 Wire 使用。
func LoadRuntimeConfig(params Params) (RuntimeConfig, error) {
	return Load(params)
}

// ProvideServiceInfo 返回服务元信息。
func ProvideServiceInfo(cfg RuntimeConfig) ServiceInfo {
	return cfg.Service
}

// ProvideLoggerConfig 构造 gclog.Config。
func ProvideLoggerConfig(info ServiceInfo) gclog.Config {
	return gclog.Config{
		Service:              info.Name,
		Version:              info.Version,
		Environment:          info.Environment,
		InstanceID:           info.InstanceID,
		EnableSourceLocation: true,
		StaticLabels: map[string]string{
			"service.id": info.InstanceID,
		},
	}
}

// ProvideObservabilityConfig 将 ObservabilityConfig 转换为 obswire.ObservabilityConfig。
func ProvideObservabilityConfig(cfg RuntimeConfig) obswire.ObservabilityConfig {
	tracing := cfg.Observability.Tracing
	metrics := cfg.Observability.Metrics

	var tracingCfg *obswire.TracingConfig
	if tracing.Enabled || tracing.Endpoint != "" || tracing.Exporter != "" {
		tracingCfg = &obswire.TracingConfig{
			Enabled:            tracing.Enabled,
			Exporter:           tracing.Exporter,
			Endpoint:           tracing.Endpoint,
			Headers:            tracing.Headers,
			Insecure:           tracing.Insecure,
			SamplingRatio:      tracing.SamplingRatio,
			Attributes:         tracing.Attributes,
			BatchTimeout:       tracing.BatchTimeout,
			ExportTimeout:      tracing.ExportTimeout,
			MaxQueueSize:       tracing.MaxQueueSize,
			MaxExportBatchSize: tracing.MaxExportBatchSize,
			Required:           tracing.Required,
		}
	}

	var metricsCfg *obswire.MetricsConfig
	if metrics.Enabled || metrics.Exporter != "" || metrics.Endpoint != "" {
		metricsCfg = &obswire.MetricsConfig{
			Enabled:             metrics.Enabled,
			Exporter:            metrics.Exporter,
			Endpoint:            metrics.Endpoint,
			Headers:             metrics.Headers,
			Insecure:            metrics.Insecure,
			Interval:            metrics.Interval,
			ResourceAttributes:  metrics.ResourceAttributes,
			DisableRuntimeStats: metrics.DisableRuntimeStats,
			Required:            metrics.Required,
			GRPCEnabled:         metrics.GRPCEnabled,
			GRPCIncludeHealth:   metrics.GRPCIncludeHealth,
		}
	}

	return obswire.ObservabilityConfig{
		Tracing:          tracingCfg,
		Metrics:          metricsCfg,
		GlobalAttributes: cfg.Observability.GlobalAttributes,
	}
}

// ProvideObservabilityInfo 转换为 obswire.ServiceInfo。
func ProvideObservabilityInfo(info ServiceInfo) obswire.ServiceInfo {
	return obswire.ServiceInfo{
		Name:        info.Name,
		Version:     info.Version,
		Environment: info.Environment,
	}
}

// ProvideServerConfig 返回服务端 gRPC 配置。
func ProvideServerConfig(cfg RuntimeConfig) ServerConfig {
	return cfg.Server
}

// ProvideDatabaseConfig 返回数据库配置。
func ProvideDatabaseConfig(cfg RuntimeConfig) DatabaseConfig {
	return cfg.Database
}

// ProvidePgxConfig 将 DatabaseConfig 转换为 pgxpoolx.Config。
func ProvidePgxConfig(dbCfg DatabaseConfig) pgxpoolx.Config {
	enablePrepared := dbCfg.PreparedStmts
	metricsEnabled := dbCfg.PoolMetrics
	return pgxpoolx.Config{
		DSN:                dbCfg.DSN,
		MaxConns:           int32(dbCfg.MaxOpenConns),
		MinConns:           int32(dbCfg.MinOpenConns),
		MaxConnLifetime:    dbCfg.MaxConnLifetime,
		MaxConnIdleTime:    dbCfg.MaxConnIdleTime,
		HealthCheckPeriod:  dbCfg.HealthCheckPeriod,
		Schema:             dbCfg.Schema,
		EnablePreparedStmt: &enablePrepared,
		MetricsEnabled:     &metricsEnabled,
	}
}

// ProvideTxConfig 构造 txmanager.Config。
func ProvideTxConfig(cfg RuntimeConfig) txconfig.Config {
	tx := cfg.Database.Transaction
	return txconfig.Config{
		DefaultIsolation: tx.DefaultIsolation,
		DefaultTimeout:   tx.DefaultTimeout,
		LockTimeout:      tx.LockTimeout,
		MaxRetries:       tx.MaxRetries,
		MetricsEnabled:   boolPtr(tx.MetricsEnabled),
	}
}

// ProvideHandlerTimeouts 将 Server 层配置映射为控制层使用的超时策略。
func ProvideHandlerTimeouts(cfg RuntimeConfig) controllers.HandlerTimeouts {
	handlers := cfg.Server.Handlers
	return controllers.HandlerTimeouts{
		Default: handlers.Default,
		Command: handlers.Command,
		Query:   handlers.Query,
	}
}

// ProvideJWTConfig 汇总客户端与服务端 JWT 配置。
func ProvideJWTConfig(cfg RuntimeConfig) gcjwt.Config {
	var serverCfg *gcjwt.ServerConfig
	if cfg.Server.JWT.ExpectedAudience != "" || cfg.Server.JWT.Required || !cfg.Server.JWT.SkipValidate {
		serverCfg = &gcjwt.ServerConfig{
			ExpectedAudience: cfg.Server.JWT.ExpectedAudience,
			SkipValidate:     cfg.Server.JWT.SkipValidate,
			Required:         cfg.Server.JWT.Required,
			HeaderKey:        cfg.Server.JWT.HeaderKey,
		}
	}

	var clientCfg *gcjwt.ClientConfig
	if cfg.GRPCClient.Target != "" {
		clientCfg = &gcjwt.ClientConfig{
			Audience:  cfg.GRPCClient.JWT.Audience,
			Disabled:  cfg.GRPCClient.JWT.Disabled,
			HeaderKey: cfg.GRPCClient.JWT.HeaderKey,
		}
	}

	return gcjwt.Config{
		Server: serverCfg,
		Client: clientCfg,
	}
}

// ProvideClientConfig 返回 gRPC 客户端配置。
func ProvideClientConfig(cfg RuntimeConfig) GRPCClientConfig {
	return cfg.GRPCClient
}

// ProvideMessagingConfig 返回消息相关配置。
func ProvideMessagingConfig(cfg RuntimeConfig) MessagingConfig {
	return cfg.Messaging
}

// ProvidePubSubConfig 将 MessagingConfig 转换为 gcpubsub.Config。
func ProvidePubSubConfig(msg MessagingConfig) gcpubsub.Config {
	return toGCPubSubConfig(msg.PubSub)
}

// ProvideEngagementConfig 返回 engagement Pub/Sub 配置包装。
func ProvideEngagementConfig(msg MessagingConfig) EngagementPubSubConfig {
	cfg := toGCPubSubConfig(msg.Engagement)
	return EngagementPubSubConfig(cfg)
}

func toGCPubSubConfig(cfg PubSubConfig) gcpubsub.Config {
	if cfg.ProjectID == "" {
		return gcpubsub.Config{}
	}
	result := gcpubsub.Config{
		ProjectID:           cfg.ProjectID,
		TopicID:             cfg.TopicID,
		SubscriptionID:      cfg.SubscriptionID,
		PublishTimeout:      cfg.PublishTimeout,
		OrderingKeyEnabled:  boolPtr(cfg.OrderingKeyEnabled),
		EnableLogging:       boolPtr(cfg.LoggingEnabled),
		EnableMetrics:       boolPtr(cfg.MetricsEnabled),
		EmulatorEndpoint:    cfg.EmulatorEndpoint,
		ExactlyOnceDelivery: cfg.ExactlyOnceDelivery,
		Receive: gcpubsub.ReceiveConfig{
			NumGoroutines:          cfg.Receive.NumGoroutines,
			MaxOutstandingMessages: cfg.Receive.MaxOutstandingMessages,
			MaxOutstandingBytes:    cfg.Receive.MaxOutstandingBytes,
			MaxExtension:           cfg.Receive.MaxExtension,
			MaxExtensionPeriod:     cfg.Receive.MaxExtensionPeriod,
		},
	}
	return result.Normalize()
}

// ProvidePubSubDependencies 注入 Pub/Sub 依赖。
func ProvidePubSubDependencies(logger log.Logger) gcpubsub.Dependencies {
	return gcpubsub.Dependencies{Logger: logger}
}

// ProvideEngagementSubscriber 构造 engagement Subscriber。
func ProvideEngagementSubscriber(ctx context.Context, cfg EngagementPubSubConfig, deps gcpubsub.Dependencies) (EngagementSubscriber, func(), error) {
	base := gcpubsub.Config(cfg)
	if base.ProjectID == "" || base.SubscriptionID == "" {
		return EngagementSubscriber(nil), func() {}, nil
	}
	comp, cleanup, err := gcpubsub.NewComponent(ctx, base, deps)
	if err != nil {
		return EngagementSubscriber(nil), cleanup, err
	}
	return EngagementSubscriber(gcpubsub.ProvideSubscriber(comp)), cleanup, nil
}

// ProvideOutboxConfig 构造 outboxcfg.Config。
func ProvideOutboxConfig(msg MessagingConfig) outboxcfg.Config {
	cfg := outboxcfg.Config{
		Schema: msg.Schema,
		Publisher: outboxcfg.PublisherConfig{
			BatchSize:      msg.Outbox.BatchSize,
			TickInterval:   msg.Outbox.TickInterval,
			InitialBackoff: msg.Outbox.InitialBackoff,
			MaxBackoff:     msg.Outbox.MaxBackoff,
			MaxAttempts:    msg.Outbox.MaxAttempts,
			PublishTimeout: msg.Outbox.PublishTimeout,
			Workers:        msg.Outbox.Workers,
			LockTTL:        msg.Outbox.LockTTL,
			LoggingEnabled: msg.Outbox.LoggingEnabled,
			MetricsEnabled: msg.Outbox.MetricsEnabled,
		},
		Inbox: outboxcfg.InboxConfig{
			SourceService:  msg.Inbox.SourceService,
			MaxConcurrency: msg.Inbox.MaxConcurrency,
			LoggingEnabled: msg.Inbox.LoggingEnabled,
			MetricsEnabled: msg.Inbox.MetricsEnabled,
		},
	}

	cfg = cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return cfg
}

func boolPtr(v bool) *bool {
	return &v
}
