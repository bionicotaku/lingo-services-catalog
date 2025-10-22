// Package main boots the Kratos gRPC entrypoint for the template service.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	loader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
	loginfra "github.com/bionicotaku/kratos-template/internal/infrastructure/logger"

	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"

	_ "go.uber.org/automaxprocs"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string
	// Version is the version of the compiled software.
	Version string

	id, _ = os.Hostname()
)

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
	// Parse command-line flags (currently only -conf).
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	confPath, err := loader.ParseConfPath(fs, os.Args[1:])
	if err != nil {
		panic(err)
	}

	// Load bootstrap configuration and derive logger settings.
	cfgLoader, cleanupConfig, err := loader.LoadBootstrap(confPath, Name, Version)
	if err != nil {
		panic(err)
	}
	defer cleanupConfig()

	// Build the structured logger used by the entire application.
	loggr, err := loginfra.NewLogger(cfgLoader.LoggerCfg)
	if err != nil {
		panic(err)
	}

	obsShutdown, err := observability.Init(context.Background(), cfgLoader.ObsConfig,
		observability.WithLogger(loggr),
		observability.WithServiceName(cfgLoader.LoggerCfg.Service),
		observability.WithServiceVersion(cfgLoader.LoggerCfg.Version),
		observability.WithEnvironment(cfgLoader.LoggerCfg.Env),
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		if obsShutdown == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := obsShutdown(ctx); err != nil {
			log.NewHelper(loggr).Warnf("shutdown observability: %v", err)
		}
	}()

	// Assemble all dependencies (logger, servers, repositories, etc.) via Wire and create the Kratos app.
	app, cleanupApp, err := wireApp(
		cfgLoader.Bootstrap.GetServer(),
		cfgLoader.Bootstrap.GetData(),
		loggr,
	)
	if err != nil {
		panic(err)
	}
	defer cleanupApp()

	// Start the application and block until a stop signal is received.
	if err := app.Run(); err != nil {
		panic(err)
	}
}
