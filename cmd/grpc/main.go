// Package main boots the Kratos gRPC entrypoint for the template service.
package main

import (
	"context"
	"flag"
	"os"

	loader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
	obswire "github.com/bionicotaku/lingo-utils/observability"
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

// newApp 负责组装 Kratos 应用：注入观测组件、日志器以及 gRPC Server。
func newApp(_ *obswire.Component, logger log.Logger, gs *grpc.Server) *kratos.App {
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
	ctx := context.Background()
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

	// Assemble all dependencies (logger, servers, repositories, etc.) via Wire and create the Kratos app.
	app, cleanupApp, err := wireApp(
		ctx,
		cfgLoader.Bootstrap.GetServer(),
		cfgLoader.Bootstrap.GetData(),
		cfgLoader.ObsConfig,
		cfgLoader.Service,
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
