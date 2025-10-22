package main

import (
	"flag"
	"os"

	loader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
	loginfra "github.com/bionicotaku/kratos-template/internal/infrastructure/logger"

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

	// Assemble all dependencies (logger, servers, repositories, etc.) via Wire and create the Kratos app.
	app, cleanupApp, err := wireApp(cfgLoader.Bootstrap.Server, cfgLoader.Bootstrap.Data, loggr)
	if err != nil {
		panic(err)
	}
	defer cleanupApp()

	// Start the application and block until a stop signal is received.
	if err := app.Run(); err != nil {
		panic(err)
	}
}
