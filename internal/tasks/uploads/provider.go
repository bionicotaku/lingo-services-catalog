package uploads

import (
	"github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
)

// ProvideRunner 装配 Uploads Runner。
func ProvideRunner(
	uploadRepo *repositories.UploadRepository,
	inboxRepo *repositories.InboxRepository,
	lifecycle *services.LifecycleWriter,
	tx txmanager.Manager,
	sub configloader.UploadSubscriber,
	outboxCfg outboxcfg.Config,
	logger log.Logger,
) *Runner {
	realSub := gcpubsub.Subscriber(sub)
	if uploadRepo == nil || inboxRepo == nil || lifecycle == nil || realSub == nil || logger == nil {
		return nil
	}

	runner, err := NewRunner(RunnerParams{
		Subscriber: realSub,
		InboxRepo:  inboxRepo,
		UploadRepo: uploadRepo,
		Lifecycle:  lifecycle,
		TxManager:  tx,
		Logger:     logger,
		Config:     outboxCfg.Inbox,
	})
	if err != nil {
		log.NewHelper(logger).Errorw("msg", "init uploads runner failed", "error", err)
		return nil
	}
	return runner
}
