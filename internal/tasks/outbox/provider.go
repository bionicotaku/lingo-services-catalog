package outbox

import (
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	outboxpublisher "github.com/bionicotaku/lingo-utils/outbox/publisher"

	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel"
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
	if pubCfg.TopicID == "" {
		return nil
	}

	meter := otel.GetMeterProvider().Meter("kratos-template.outbox")
	runner, err := outboxpublisher.NewRunner(outboxpublisher.RunnerParams{
		Store:     repo.Shared(),
		Publisher: publisher,
		Config:    cfg.Publisher,
		Logger:    logger,
		Meter:     meter,
	})
	if err != nil {
		log.NewHelper(logger).Errorw("msg", "init outbox runner failed", "error", err)
		return nil
	}
	return runner
}
