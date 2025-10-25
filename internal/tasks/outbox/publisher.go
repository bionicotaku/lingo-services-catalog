package outbox

import (
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	lpublisher "github.com/bionicotaku/lingo-utils/outbox/publisher"
	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel/metric"
)

// Config 直接复用共享发布器配置。
type Config = lpublisher.Config

// PublisherTask 复用共享任务实现。
type PublisherTask = lpublisher.Task

// NewPublisherTask 构造 Outbox 发布任务。
func NewPublisherTask(repo *repositories.OutboxRepository, pub gcpubsub.Publisher, cfg Config, logger log.Logger, meter metric.Meter) *PublisherTask {
	return lpublisher.NewTask(repo.Shared(), pub, cfg, logger, meter)
}
