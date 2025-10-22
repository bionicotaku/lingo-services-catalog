//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"

	"github.com/bionicotaku/kratos-template/internal/clients"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	grpcclient "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_client"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"

	gclog "github.com/bionicotaku/lingo-utils/gclog"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
)

// wireApp init kratos application.
func wireApp(context.Context, *configpb.Server, *configpb.Data, obswire.ObservabilityConfig, obswire.ServiceInfo, gclog.Config) (*kratos.App, func(), error) {
	// Providers and their dependencies:
	//   - gclog.NewComponent(gclog.Config) (*gclog.Component, func(), error)
	//   - gclog.ProvideLogger(*gclog.Component) log.Logger
	//   - observability.NewComponent(context.Context, observability.ObservabilityConfig, observability.ServiceInfo, log.Logger) (*observability.Component, func(), error)
	//   - observability.ProvideMetricsConfig(observability.ObservabilityConfig) *observability.MetricsConfig
	//   - grpc_server.NewGRPCServer(*configpb.Server, *observability.MetricsConfig, *controllers.GreeterHandler, log.Logger) *grpc.Server
	//   - grpc_client.NewGRPCClient(*configpb.Data, *observability.MetricsConfig, log.Logger) (*grpc.ClientConn, func(), error)
	//   - clients.NewGreeterRemote(*grpc.ClientConn, log.Logger) services.GreeterRemote
	//   - repositories.NewGreeterRepo(*data.Data, log.Logger) services.GreeterRepo
	//   - services.NewGreeterUsecase(repositories.GreeterRepo, services.GreeterRemote, log.Logger) *services.GreeterUsecase
	//   - controllers.NewGreeterHandler(*services.GreeterUsecase) *controllers.GreeterHandler
	//   - newApp(log.Logger, *grpc.Server) *kratos.App
	panic(wire.Build(
		gclog.ProviderSet,
		obswire.ProviderSet,
		grpcserver.ProviderSet,
		grpcclient.ProviderSet,
		clients.ProviderSet,
		repositories.ProviderSet,
		services.ProviderSet,
		controllers.ProviderSet,
		newApp,
	))
}
