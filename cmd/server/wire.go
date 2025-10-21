//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"github.com/bionicotaku/kratos-template/internal/biz"
	"github.com/bionicotaku/kratos-template/internal/client"
	"github.com/bionicotaku/kratos-template/internal/conf"
	"github.com/bionicotaku/kratos-template/internal/data"
	"github.com/bionicotaku/kratos-template/internal/server"
	"github.com/bionicotaku/kratos-template/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(server.ProviderSet, client.ProviderSet, data.ProviderSet, biz.ProviderSet, service.ProviderSet, newApp))
}
