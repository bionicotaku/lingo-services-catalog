package engagement

import (
	"context"
	"fmt"

	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/outbox/inbox"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
)

// Runner 封装 Engagement 事件消费循环（基于 Inbox Runner）。
type Runner struct {
	delegate *inbox.Runner[Event]
	metrics  *metrics
}

// RunnerParams 注入 Runner 所需依赖。
type RunnerParams struct {
	Subscriber gcpubsub.Subscriber
	InboxRepo  *repositories.InboxRepository
	UserRepo   videoUserStatesStore
	TxManager  txmanager.Manager
	Logger     log.Logger
	Config     config.InboxConfig
}

// NewRunner 构造 Engagement Runner。
func NewRunner(params RunnerParams) (*Runner, error) {
	if params.Subscriber == nil {
		return nil, fmt.Errorf("engagement: subscriber is required")
	}
	if params.InboxRepo == nil {
		return nil, fmt.Errorf("engagement: inbox repository is required")
	}
	if params.UserRepo == nil {
		return nil, fmt.Errorf("engagement: user state repository is required")
	}
	if params.TxManager == nil {
		return nil, fmt.Errorf("engagement: tx manager is required")
	}

	metrics := newMetrics()
	handler := NewEventHandler(params.UserRepo, params.Logger, metrics)
	decoder := newEventDecoder()

	delegate, err := inbox.NewRunner[Event](inbox.RunnerParams[Event]{
		Store:      params.InboxRepo.Shared(),
		Subscriber: params.Subscriber,
		TxManager:  params.TxManager,
		Decoder:    decoder,
		Handler:    handler,
		Config:     params.Config,
		Logger:     params.Logger,
	})
	if err != nil {
		return nil, err
	}

	return &Runner{
		delegate: delegate,
		metrics:  metrics,
	}, nil
}

// Run 启动消费循环。
func (r *Runner) Run(ctx context.Context) error {
	if r == nil || r.delegate == nil {
		return nil
	}
	return r.delegate.Run(ctx)
}
