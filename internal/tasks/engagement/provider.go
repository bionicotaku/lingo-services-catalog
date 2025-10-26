package engagement

import (
	"github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
)

// ProvideRunner 装配 Engagement Runner。
func ProvideRunner(repo *repositories.VideoUserStatesRepository, tx txmanager.Manager, sub configloader.EngagementSubscriber, logger log.Logger) *Runner {
	realSub := gcpubsub.Subscriber(sub)
	if repo == nil || realSub == nil || logger == nil {
		return nil
	}
	runner, err := NewRunner(RunnerParams{
		Subscriber: realSub,
		Repository: repo,
		TxManager:  tx,
		Logger:     logger,
	})
	if err != nil {
		log.NewHelper(logger).Errorw("msg", "init engagement runner failed", "error", err)
		return nil
	}
	return runner
}
