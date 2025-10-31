//go:build wireinject
// +build wireinject

// Package main 为 uploads 任务 CLI 提供 Wire 依赖注入定义。
package main

import (
	"context"
	"fmt"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	uploadtasks "github.com/bionicotaku/lingo-services-catalog/internal/tasks/uploads"

	"github.com/bionicotaku/lingo-utils/gclog"
	"github.com/bionicotaku/lingo-utils/pgxpoolx"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

//go:generate go run github.com/google/wire/cmd/wire

func wireUploadsTask(context.Context, configloader.Params) (*uploadsTaskApp, func(), error) {
	panic(wire.Build(
		configloader.ProviderSet,
		gclog.ProviderSet,
		pgxpoolx.ProviderSet,
		txmanager.ProviderSet,
		repositories.ProviderSet,
		services.NewLifecycleWriter,
		wire.Bind(new(services.LifecycleRepo), new(*repositories.VideoRepository)),
		wire.Bind(new(services.LifecycleOutboxWriter), new(*repositories.OutboxRepository)),
		uploadtasks.ProvideRunner,
		newUploadsTaskApp,
	))
}

func newUploadsTaskApp(logger log.Logger, runner *uploadtasks.Runner) (*uploadsTaskApp, error) {
	if runner == nil {
		return &uploadsTaskApp{Logger: logger}, nil
	}
	if logger == nil {
		return nil, fmt.Errorf("logger not initialized")
	}
	return &uploadsTaskApp{
		Runner: runner,
		Logger: logger,
	}, nil
}
