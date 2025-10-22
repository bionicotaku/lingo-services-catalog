//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"github.com/bionicotaku/kratos-template/internal/clients"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	grpcclient "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_client"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// wireApp init kratos application.
func wireApp(*configpb.Server, *configpb.Data, log.Logger) (*kratos.App, func(), error) {
	// Providers and their dependencies:
	//   - grpc_server.NewGRPCServer(*configpb.Server, *controllers.GreeterHandler, log.Logger) *grpc.Server
	//   - grpc_client.NewGRPCClient(*configpb.Data, log.Logger) (*grpc.ClientConn, func(), error)
	//   - clients.NewGreeterRemote(*grpc.ClientConn, log.Logger) services.GreeterRemote
	//   - repositories.NewGreeterRepo(*data.Data, log.Logger) services.GreeterRepo
	//   - services.NewGreeterUsecase(repositories.GreeterRepo, services.GreeterRemote, log.Logger) *services.GreeterUsecase
	//   - controllers.NewGreeterHandler(*services.GreeterUsecase) *controllers.GreeterHandler
	//   - newApp(log.Logger, *grpc.Server) *kratos.App
	panic(wire.Build(
		grpcserver.ProviderSet,
		grpcclient.ProviderSet,
		clients.ProviderSet,
		repositories.ProviderSet,
		services.ProviderSet,
		controllers.ProviderSet,
		newApp,
	))
}
