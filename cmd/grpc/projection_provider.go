package main

import (
	loader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/tasks/projection"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
)

func provideProjectionTask(
	subscriber gcpubsub.Subscriber,
	inboxRepo *repositories.InboxRepository,
	projectionRepo *repositories.VideoProjectionRepository,
	txManager txmanager.Manager,
	_ loader.ProjectionConsumerConfig,
	logger log.Logger,
) *projection.Task {
	if subscriber == nil || inboxRepo == nil || projectionRepo == nil || txManager == nil || logger == nil {
		return nil
	}
	return projection.NewTask(subscriber, inboxRepo, projectionRepo, txManager, logger)
}
