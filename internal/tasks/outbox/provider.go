// Package outbox wires shared repositories and publisher instances into
// runnable Outbox workers for integration tests and local execution.
package outbox

import (
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	outboxpublisher "github.com/bionicotaku/lingo-utils/outbox/publisher"

	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
)

// ProvideRunner 将共享仓储与 Pub/Sub 发布器包装为 Outbox Runner。
func ProvideRunner(
	repo *repositories.OutboxRepository,
	publisher gcpubsub.Publisher,
	pubCfg gcpubsub.Config,
	cfg outboxcfg.Config,
	logger log.Logger,
) *outboxpublisher.Runner {
	if repo == nil || logger == nil {
		return nil
	}
	helper := log.NewHelper(logger)
	if pubCfg.TopicID == "" {
		helper.Warn("skip initializing outbox runner: pubsub topic not configured")
		return nil
	}

	normalized := cfg.Normalize()
	pubCfgNormalized := normalized.Publisher

	meterProvider := otel.GetMeterProvider()
	if !boolValue(pubCfgNormalized.MetricsEnabled, true) {
		meterProvider = noopmetric.NewMeterProvider()
	}

	if boolValue(pubCfgNormalized.LoggingEnabled, true) {
		helper.Infof("init outbox runner: batch_size=%d, workers=%d, tick_interval=%s",
			pubCfgNormalized.BatchSize, pubCfgNormalized.Workers, pubCfgNormalized.TickInterval)
	} else {
		helper.Debug("init outbox runner with logging disabled by configuration")
	}

	runner, err := outboxpublisher.NewRunner(outboxpublisher.RunnerParams{
		Store:     repo.Shared(),
		Publisher: publisher,
		Config:    pubCfgNormalized,
		Logger:    logger,
		Meter:     meterProvider.Meter("lingo-services-catalog.outbox"),
	})
	if err != nil {
		helper.Errorw("msg", "init outbox runner failed", "error", err)
		return nil
	}
	return runner
}

func boolValue(ptr *bool, def bool) bool {
	if ptr == nil {
		return def
	}
	return *ptr
}
