//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"fmt"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/tasks/engagement"

	"github.com/bionicotaku/lingo-utils/gclog"
	"github.com/bionicotaku/lingo-utils/pgxpoolx"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

//go:generate go run github.com/google/wire/cmd/wire

func wireEngagementTask(context.Context, configloader.Params) (*engagementApp, func(), error) {
	panic(wire.Build(
		configloader.ProviderSet,
		gclog.ProviderSet,
		pgxpoolx.ProviderSet,
		txmanager.ProviderSet,
		repositories.ProviderSet,
		engagement.ProvideRunner,
		newEngagementApp,
	))
}

func newEngagementApp(logger log.Logger, runner *engagement.Runner) (*engagementApp, error) {
	if runner == nil {
		return &engagementApp{Logger: logger}, nil
	}
	if logger == nil {
		return nil, fmt.Errorf("logger not initialized")
	}
	return &engagementApp{
		Runner: runner,
		Logger: logger,
	}, nil
}
