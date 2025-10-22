//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"github.com/bionicotaku/kratos-template/internal/clients"
	"github.com/bionicotaku/kratos-template/internal/conf"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	grpcclient "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_client"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(grpcserver.ProviderSet, grpcclient.ProviderSet, clients.ProviderSet, repositories.ProviderSet, services.ProviderSet, controllers.ProviderSet, newApp))
}
