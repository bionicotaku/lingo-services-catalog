package engagement

import (
	"github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
)

// ProvideRunner 装配 Engagement Runner。
func ProvideRunner(
	userRepo *repositories.VideoUserStatesRepository,
	inboxRepo *repositories.InboxRepository,
	tx txmanager.Manager,
	sub configloader.EngagementSubscriber,
	outboxCfg outboxcfg.Config,
	logger log.Logger,
) *Runner {
	realSub := gcpubsub.Subscriber(sub)
	if userRepo == nil || inboxRepo == nil || realSub == nil || logger == nil {
		return nil
	}
	runner, err := NewRunner(RunnerParams{
		Subscriber: realSub,
		InboxRepo:  inboxRepo,
		UserRepo:   userRepo,
		TxManager:  tx,
		Logger:     logger,
		Config:     outboxCfg.Inbox,
	})
	if err != nil {
		log.NewHelper(logger).Errorw("msg", "init engagement runner failed", "error", err)
		return nil
	}
	return runner
}
