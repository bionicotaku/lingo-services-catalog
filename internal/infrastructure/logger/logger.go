package logger

import (
	"context"
	"os"

	gclog "github.com/bionicotaku/lingo-utils/gclog"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel/trace"
)

// Options captures runtime metadata used to annotate logs.
type Options struct {
	Service string
	Version string
	HostID  string
	Env     string
}

// NewLogger builds a Kratos-compatible logger with trace/span enrichment.
func NewLogger(opts Options) (log.Logger, error) {
	baseLogger, err := gclog.NewLogger(
		gclog.WithService(opts.Service),
		gclog.WithVersion(opts.Version),
		gclog.WithEnvironment(opts.Env),
		gclog.WithStaticLabels(map[string]string{"service.id": opts.HostID}),
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

// DefaultOptions builds Options from environment defaults.
func DefaultOptions(service, version string) Options {
	if service == "" {
		service = "kratos-template"
	}
	if version == "" {
		version = "dev"
	}
	host, _ := os.Hostname()
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	return Options{Service: service, Version: version, HostID: host, Env: env}
}
