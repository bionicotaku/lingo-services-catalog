package logger

import (
	"context"
	"os"

	gclog "github.com/bionicotaku/lingo-utils/gclog"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel/trace"
)

// Config captures runtime metadata used to annotate logs.
type Config struct {
	Service string
	Version string
	HostID  string
	Env     string
}

// NewLogger builds a Kratos-compatible logger with trace/span enrichment.
func NewLogger(cfg Config) (log.Logger, error) {
	baseLogger, err := gclog.NewLogger(
		gclog.WithService(cfg.Service),
		gclog.WithVersion(cfg.Version),
		gclog.WithEnvironment(cfg.Env),
		gclog.WithStaticLabels(map[string]string{"service.id": cfg.HostID}),
		gclog.EnableSourceLocation(),
	)
	if err != nil {
		return nil, err
	}
	return log.With(
		baseLogger,
		"trace_id", log.Valuer(func(ctx context.Context) interface{} {
			sc := trace.SpanContextFromContext(ctx)
			if sc.HasTraceID() {
				return sc.TraceID().String()
			}
			return ""
		}),
		"span_id", log.Valuer(func(ctx context.Context) interface{} {
			sc := trace.SpanContextFromContext(ctx)
			if sc.HasSpanID() {
				return sc.SpanID().String()
			}
			return ""
		}),
	), nil
}

// DefaultConfig builds Config from environment defaults.
func DefaultConfig(service, version string) Config {
	if service == "" {
		service = "template"
	}
	if version == "" {
		version = "dev"
	}
	host, _ := os.Hostname()
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	return Config{Service: service, Version: version, HostID: host, Env: env}
}
