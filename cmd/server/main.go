package main

import (
	"context"
	"flag"
	"os"

	"github.com/bionicotaku/kratos-template/internal/conf"
	gclog "github.com/bionicotaku/lingo-utils/gclog"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	oteltrace "go.opentelemetry.io/otel/trace"

	_ "go.uber.org/automaxprocs"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string
	// Version is the version of the compiled software.
	Version string
	// flagconf is the config flag.
	flagconf string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(logger log.Logger, gs *grpc.Server) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			gs,
		),
	)
}

func main() {
	flag.Parse()
	if Name == "" {
		Name = "kratos-template"
	}
	if Version == "" {
		Version = "dev"
	}
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}

	baseLogger, err := gclog.NewLogger(
		gclog.WithService(Name),
		gclog.WithVersion(Version),
		gclog.WithEnvironment(appEnv),
		gclog.WithStaticLabels(map[string]string{"service.id": id}),
		gclog.EnableSourceLocation(),
	)
	if err != nil {
		panic(err)
	}

	logger := log.With(
		baseLogger,
		"trace_id", log.Valuer(func(ctx context.Context) interface{} {
			sc := oteltrace.SpanContextFromContext(ctx)
			if sc.HasTraceID() {
				return sc.TraceID().String()
			}
			return ""
		}),
		"span_id", log.Valuer(func(ctx context.Context) interface{} {
			sc := oteltrace.SpanContextFromContext(ctx)
			if sc.HasSpanID() {
				return sc.SpanID().String()
			}
			return ""
		}),
	)

	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	app, cleanup, err := wireApp(bc.Server, bc.Data, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// start and wait for stop signal
	if err := app.Run(); err != nil {
		panic(err)
	}
}
