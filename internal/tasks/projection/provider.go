package projection

import (
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"

	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
)

// ProvideTask 根据共享组件构造投影消费者任务。
func ProvideTask(
	subscriber gcpubsub.Subscriber,
	inboxRepo *repositories.InboxRepository,
	projectionRepo *repositories.VideoProjectionRepository,
	txManager txmanager.Manager,
	cfg outboxcfg.Config,
	logger log.Logger,
) *Task {
	if subscriber == nil || inboxRepo == nil || projectionRepo == nil || txManager == nil || logger == nil {
		return nil
	}
	return NewTask(subscriber, inboxRepo, projectionRepo, txManager, logger, cfg.Inbox)
}
