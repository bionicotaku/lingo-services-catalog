package main

import (
	loader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/tasks/outbox"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/go-kratos/kratos/v2/log"
)

func provideOutboxTask(
	repo *repositories.OutboxRepository,
	publisher gcpubsub.Publisher,
	pubCfg gcpubsub.Config,
	cfg loader.OutboxPublisherConfig,
	logger log.Logger,
) *outbox.PublisherTask {
	if repo == nil || logger == nil {
		return nil
	}
	if pubCfg.TopicID == "" {
		return nil
	}

	taskCfg := outbox.Config{
		BatchSize:      cfg.BatchSize,
		TickInterval:   cfg.TickInterval,
		InitialBackoff: cfg.InitialBackoff,
		MaxBackoff:     cfg.MaxBackoff,
		MaxAttempts:    cfg.MaxAttempts,
		PublishTimeout: cfg.PublishTimeout,
	}

	return outbox.NewPublisherTask(repo, publisher, taskCfg, logger)
}
