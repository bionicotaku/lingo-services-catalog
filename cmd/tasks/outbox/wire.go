//go:build wireinject
// +build wireinject

// Package main 为 outbox 任务 CLI 提供 Wire 依赖注入定义。
package main

import (
	"context"
	"fmt"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	outboxtasks "github.com/bionicotaku/lingo-services-catalog/internal/tasks/outbox"

	"github.com/bionicotaku/lingo-utils/gclog"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	outboxpublisher "github.com/bionicotaku/lingo-utils/outbox/publisher"
	"github.com/bionicotaku/lingo-utils/pgxpoolx"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

//go:generate go run github.com/google/wire/cmd/wire

var outboxRepositorySet = wire.NewSet(repositories.NewOutboxRepository)

func wireOutboxTask(context.Context, configloader.Params) (*outboxTaskApp, func(), error) {
	panic(wire.Build(
		configloader.ProviderSet,
		gclog.ProviderSet,
		pgxpoolx.ProviderSet,
		gcpubsub.ProviderSet,
		outboxRepositorySet,
		outboxtasks.ProvideRunner,
		newOutboxTaskApp,
	))
}

func newOutboxTaskApp(logger log.Logger, runner *outboxpublisher.Runner) (*outboxTaskApp, error) {
	if runner == nil {
		return &outboxTaskApp{Logger: logger}, nil
	}
	if logger == nil {
		return nil, fmt.Errorf("logger not initialized")
	}
	return &outboxTaskApp{
		Runner: runner,
		Logger: logger,
	}, nil
}
