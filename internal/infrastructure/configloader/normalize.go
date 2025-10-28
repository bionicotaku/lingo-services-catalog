package configloader

import (
	"time"

	configpb "github.com/bionicotaku/lingo-services-catalog/configs"

	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	defaultHandlerTimeout = 5 * time.Second
	defaultQueryTimeout   = 3 * time.Second
)

func fromProto(b *configpb.Bootstrap) RuntimeConfig {
	if b == nil {
		return RuntimeConfig{}
	}
	rc := RuntimeConfig{
		Server:        serverFromProto(b.GetServer()),
		Database:      databaseFromProto(b.GetData().GetPostgres()),
		GRPCClient:    grpcClientFromProto(b.GetData().GetGrpcClient()),
		Observability: observabilityFromProto(b.GetObservability()),
		Messaging:     messagingFromProto(b.GetMessaging(), b.GetData()),
	}
	return rc
}

func serverFromProto(s *configpb.Server) ServerConfig {
	if s == nil {
		return ServerConfig{}
	}
	server := ServerConfig{}
	if grpc := s.GetGrpc(); grpc != nil {
		server.Network = grpc.GetNetwork()
		server.Address = grpc.GetAddr()
		server.Timeout = durationOrZero(grpc.GetTimeout())
	}
	if jwt := s.GetJwt(); jwt != nil {
		server.JWT = ServerJWTConfig{
			ExpectedAudience: jwt.GetExpectedAudience(),
			SkipValidate:     jwt.GetSkipValidate(),
			Required:         jwt.GetRequired(),
			HeaderKey:        firstNonEmpty(jwt.GetHeaderKey(), "authorization"),
		}
	}
	server.Handlers = handlerTimeoutFromProto(s.GetHandlers())
	server.MetadataKeys = append([]string(nil), s.GetMetadataKeys()...)
	return server
}

func handlerTimeoutFromProto(h *configpb.Server_Handlers) HandlerTimeoutConfig {
	cfg := HandlerTimeoutConfig{
		Default: defaultHandlerTimeout,
		Command: defaultHandlerTimeout,
		Query:   defaultQueryTimeout,
	}
	if h == nil {
		cfg.Query = firstNonZero(cfg.Query, cfg.Default)
		return cfg
	}
	if d := durationOrZero(h.GetDefaultTimeout()); d > 0 {
		cfg.Default = d
	}
	if d := durationOrZero(h.GetCommandTimeout()); d > 0 {
		cfg.Command = d
	} else {
		cfg.Command = cfg.Default
	}
	if d := durationOrZero(h.GetQueryTimeout()); d > 0 {
		cfg.Query = d
	} else {
		cfg.Query = firstNonZero(cfg.Query, cfg.Default)
	}
	return cfg
}

func databaseFromProto(pg *configpb.Data_PostgreSQL) DatabaseConfig {
	if pg == nil {
		return DatabaseConfig{}
	}
	cfg := DatabaseConfig{
		DSN:               pg.GetDsn(),
		MaxOpenConns:      int(pg.GetMaxOpenConns()),
		MinOpenConns:      int(pg.GetMinOpenConns()),
		MaxConnLifetime:   durationOrZero(pg.GetMaxConnLifetime()),
		MaxConnIdleTime:   durationOrZero(pg.GetMaxConnIdleTime()),
		HealthCheckPeriod: durationOrZero(pg.GetHealthCheckPeriod()),
		Schema:            pg.GetSchema(),
		PreparedStmts:     pg.GetPreparedStatementsEnabled(),
		PoolMetrics:       pg.GetPoolMetricsEnabled(),
	}
	if tx := pg.GetTransaction(); tx != nil {
		cfg.Transaction = TransactionConfig{
			DefaultIsolation: tx.GetDefaultIsolation(),
			DefaultTimeout:   durationOrZero(tx.GetDefaultTimeout()),
			LockTimeout:      durationOrZero(tx.GetLockTimeout()),
			MaxRetries:       int(tx.GetMaxRetries()),
			MetricsEnabled:   tx.GetMetricsEnabled(),
		}
	}
	return cfg
}

func grpcClientFromProto(client *configpb.Data_Client) GRPCClientConfig {
	if client == nil {
		return GRPCClientConfig{}
	}
	cfg := GRPCClientConfig{
		Target: client.GetTarget(),
	}
	if jwt := client.GetJwt(); jwt != nil {
		cfg.JWT = ClientJWTConfig{
			Audience:  jwt.GetAudience(),
			Disabled:  jwt.GetDisabled(),
			HeaderKey: firstNonEmpty(jwt.GetHeaderKey(), "authorization"),
		}
	}
	cfg.MetadataKeys = append([]string(nil), client.GetMetadataKeys()...)
	return cfg
}

func observabilityFromProto(obs *configpb.Observability) ObservabilityConfig {
	if obs == nil {
		return ObservabilityConfig{}
	}
	cfg := ObservabilityConfig{
		GlobalAttributes: mapCopy(obs.GetGlobalAttributes()),
		Tracing:          tracingFromProto(obs.GetTracing()),
		Metrics:          metricsFromProto(obs.GetMetrics()),
	}
	return cfg
}

func tracingFromProto(t *configpb.Observability_Tracing) TracingConfig {
	if t == nil {
		return TracingConfig{}
	}
	return TracingConfig{
		Enabled:            t.GetEnabled(),
		Exporter:           t.GetExporter(),
		Endpoint:           t.GetEndpoint(),
		Headers:            mapCopy(t.GetHeaders()),
		Insecure:           t.GetInsecure(),
		SamplingRatio:      t.GetSamplingRatio(),
		BatchTimeout:       durationOrZero(t.GetBatchTimeout()),
		ExportTimeout:      durationOrZero(t.GetExportTimeout()),
		MaxQueueSize:       int(t.GetMaxQueueSize()),
		MaxExportBatchSize: int(t.GetMaxExportBatchSize()),
		Required:           t.GetRequired(),
		Attributes:         mapCopy(t.GetAttributes()),
	}
}

func metricsFromProto(m *configpb.Observability_Metrics) MetricsConfig {
	if m == nil {
		return MetricsConfig{}
	}
	cfg := MetricsConfig{
		Enabled:             m.GetEnabled(),
		Exporter:            m.GetExporter(),
		Endpoint:            m.GetEndpoint(),
		Headers:             mapCopy(m.GetHeaders()),
		Insecure:            m.GetInsecure(),
		Interval:            durationOrZero(m.GetInterval()),
		DisableRuntimeStats: m.GetDisableRuntimeStats(),
		Required:            m.GetRequired(),
		ResourceAttributes:  mapCopy(m.GetResourceAttributes()),
		GRPCEnabled:         m.GetGrpcEnabled(),
		GRPCIncludeHealth:   m.GetGrpcIncludeHealth(),
	}
	return cfg
}

func messagingFromProto(msg *configpb.Messaging, data *configpb.Data) MessagingConfig {
	if msg == nil {
		return MessagingConfig{}
	}
	cfg := MessagingConfig{
		PubSub:     pubsubFromProto(msg.GetPubsub()),
		Engagement: pubsubFromProto(msg.GetEngagement()),
		Outbox:     outboxFromProto(msg.GetOutbox()),
		Inbox:      inboxFromProto(msg.GetInbox()),
	}
	if data != nil && data.GetPostgres() != nil {
		cfg.Schema = data.GetPostgres().GetSchema()
	}
	return cfg
}

func pubsubFromProto(pb *configpb.PubSub) PubSubConfig {
	if pb == nil {
		return PubSubConfig{}
	}
	cfg := PubSubConfig{
		ProjectID:           pb.GetProjectId(),
		TopicID:             pb.GetTopicId(),
		SubscriptionID:      pb.GetSubscriptionId(),
		OrderingKeyEnabled:  pb.GetOrderingKeyEnabled(),
		LoggingEnabled:      pb.GetLoggingEnabled(),
		MetricsEnabled:      pb.GetMetricsEnabled(),
		EmulatorEndpoint:    pb.GetEmulatorEndpoint(),
		PublishTimeout:      durationOrZero(pb.GetPublishTimeout()),
		ExactlyOnceDelivery: pb.GetExactlyOnceDelivery(),
		DeadLetterTopicID:   pb.GetDeadLetterTopicId(),
	}
	if r := pb.GetReceive(); r != nil {
		cfg.Receive = PubSubReceiveConfig{
			NumGoroutines:          int(r.GetNumGoroutines()),
			MaxOutstandingMessages: int(r.GetMaxOutstandingMessages()),
			MaxOutstandingBytes:    int(r.GetMaxOutstandingBytes()),
			MaxExtension:           durationOrZero(r.GetMaxExtension()),
			MaxExtensionPeriod:     durationOrZero(r.GetMaxExtensionPeriod()),
		}
	}
	return cfg
}

func outboxFromProto(ob *configpb.OutboxPublisher) OutboxPublisherConfig {
	cfg := OutboxPublisherConfig{}
	if ob == nil {
		return cfg
	}
	cfg.BatchSize = int(ob.GetBatchSize())
	cfg.TickInterval = durationOrZero(ob.GetTickInterval())
	cfg.InitialBackoff = durationOrZero(ob.GetInitialBackoff())
	cfg.MaxBackoff = durationOrZero(ob.GetMaxBackoff())
	cfg.MaxAttempts = int(ob.GetMaxAttempts())
	cfg.PublishTimeout = durationOrZero(ob.GetPublishTimeout())
	cfg.Workers = int(ob.GetWorkers())
	cfg.LockTTL = durationOrZero(ob.GetLockTtl())
	if ob.LoggingEnabled != nil {
		val := ob.GetLoggingEnabled()
		cfg.LoggingEnabled = &val
	}
	if ob.MetricsEnabled != nil {
		val := ob.GetMetricsEnabled()
		cfg.MetricsEnabled = &val
	}
	return cfg
}

func inboxFromProto(in *configpb.InboxConsumer) InboxConfig {
	if in == nil {
		return InboxConfig{}
	}
	cfg := InboxConfig{
		SourceService:  in.GetSourceService(),
		MaxConcurrency: int(in.GetMaxConcurrency()),
	}
	if in.LoggingEnabled != nil {
		val := in.GetLoggingEnabled()
		cfg.LoggingEnabled = &val
	}
	if in.MetricsEnabled != nil {
		val := in.GetMetricsEnabled()
		cfg.MetricsEnabled = &val
	}
	return cfg
}

func durationOrZero(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}

func mapCopy(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func firstNonZero(durations ...time.Duration) time.Duration {
	for _, d := range durations {
		if d > 0 {
			return d
		}
	}
	return 0
}

func fillDefaults(cfg *RuntimeConfig) {
	if cfg.Server.JWT.HeaderKey == "" {
		cfg.Server.JWT.HeaderKey = "authorization"
	}
	if cfg.GRPCClient.JWT.HeaderKey == "" {
		cfg.GRPCClient.JWT.HeaderKey = "authorization"
	}
	defaultKeys := []string{
		"x-apigateway-api-userinfo",
		"x-md-",
		"x-md-idempotency-key",
		"x-md-if-match",
		"x-md-if-none-match",
	}
	if len(cfg.Server.MetadataKeys) == 0 {
		cfg.Server.MetadataKeys = append([]string(nil), defaultKeys...)
	}
	if len(cfg.GRPCClient.MetadataKeys) == 0 {
		cfg.GRPCClient.MetadataKeys = append([]string(nil), cfg.Server.MetadataKeys...)
	}
}
