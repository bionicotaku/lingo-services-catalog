// Package configloader 提供配置加载与归一化能力，供 Wire 装配使用。
package configloader

import "time"

// RuntimeConfig 聚合应用在运行期所需的配置片段。
type RuntimeConfig struct {
	Service       ServiceInfo
	Server        ServerConfig
	Database      DatabaseConfig
	GRPCClient    GRPCClientConfig
	Observability ObservabilityConfig
	Messaging     MessagingConfig
}

// ServiceInfo 描述服务标识与运行环境。
type ServiceInfo struct {
	Name        string
	Version     string
	Environment string
	InstanceID  string
}

// ServerConfig 收敛入站 gRPC 服务所需的网络与鉴权配置。
type ServerConfig struct {
	Network      string
	Address      string
	Timeout      time.Duration
	JWT          ServerJWTConfig
	Handlers     HandlerTimeoutConfig
	MetadataKeys []string
}

// ServerJWTConfig 管理入站请求的 JWT 校验策略。
type ServerJWTConfig struct {
	ExpectedAudience string
	SkipValidate     bool
	Required         bool
	HeaderKey        string
}

// HandlerTimeoutConfig 定义不同类型 Handler 的超时策略。
type HandlerTimeoutConfig struct {
	Default time.Duration
	Command time.Duration
	Query   time.Duration
}

// DatabaseConfig 包含 PostgreSQL 连接池及事务默认值。
type DatabaseConfig struct {
	DSN               string
	MaxOpenConns      int
	MinOpenConns      int
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	Schema            string
	PreparedStmts     bool
	PoolMetrics       bool
	Transaction       TransactionConfig
}

// TransactionConfig 指定事务默认隔离级别与超时策略。
type TransactionConfig struct {
	DefaultIsolation string
	DefaultTimeout   time.Duration
	LockTimeout      time.Duration
	MaxRetries       int
	MetricsEnabled   bool
}

// GRPCClientConfig 描述出站 gRPC 客户端所需信息。
type GRPCClientConfig struct {
	Target       string
	JWT          ClientJWTConfig
	MetadataKeys []string
}

// ClientJWTConfig 控制出站调用的 JWT 注入。
type ClientJWTConfig struct {
	Audience  string
	Disabled  bool
	HeaderKey string
}

// ObservabilityConfig 聚合 tracing 与 metrics 的配置。
type ObservabilityConfig struct {
	GlobalAttributes map[string]string
	Tracing          TracingConfig
	Metrics          MetricsConfig
}

// TracingConfig 描述 OpenTelemetry 追踪导出的行为。
type TracingConfig struct {
	Enabled            bool
	Exporter           string
	Endpoint           string
	Headers            map[string]string
	Insecure           bool
	SamplingRatio      float64
	BatchTimeout       time.Duration
	ExportTimeout      time.Duration
	MaxQueueSize       int
	MaxExportBatchSize int
	Required           bool
	Attributes         map[string]string
}

// MetricsConfig 描述 OpenTelemetry 指标导出的行为。
type MetricsConfig struct {
	Enabled             bool
	Exporter            string
	Endpoint            string
	Headers             map[string]string
	Insecure            bool
	Interval            time.Duration
	DisableRuntimeStats bool
	Required            bool
	ResourceAttributes  map[string]string
	GRPCEnabled         bool
	GRPCIncludeHealth   bool
}

// MessagingConfig 汇总消息系统相关配置。
type MessagingConfig struct {
	Schema     string
	PubSub     PubSubConfig
	Engagement PubSubConfig
	Outbox     OutboxPublisherConfig
	Inbox      InboxConfig
}

// PubSubConfig 提供与 GCP Pub/Sub 兼容的设置。
type PubSubConfig struct {
	ProjectID           string
	TopicID             string
	SubscriptionID      string
	OrderingKeyEnabled  bool
	LoggingEnabled      bool
	MetricsEnabled      bool
	EmulatorEndpoint    string
	PublishTimeout      time.Duration
	ExactlyOnceDelivery bool
	DeadLetterTopicID   string
	Receive             PubSubReceiveConfig
}

// PubSubReceiveConfig 控制订阅者拉取行为。
type PubSubReceiveConfig struct {
	NumGoroutines          int
	MaxOutstandingMessages int
	MaxOutstandingBytes    int
	MaxExtension           time.Duration
	MaxExtensionPeriod     time.Duration
}

// OutboxPublisherConfig 配置 Outbox 发布器的运行参数。
type OutboxPublisherConfig struct {
	BatchSize      int
	TickInterval   time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	MaxAttempts    int
	PublishTimeout time.Duration
	Workers        int
	LockTTL        time.Duration
	LoggingEnabled *bool
	MetricsEnabled *bool
}

// InboxConfig 配置 Inbox 消费者的行为。
type InboxConfig struct {
	SourceService  string
	MaxConcurrency int
	LoggingEnabled *bool
	MetricsEnabled *bool
}
