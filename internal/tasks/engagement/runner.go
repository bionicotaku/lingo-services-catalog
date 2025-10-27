package engagement

import (
	"context"
	"fmt"

	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
)

// Runner 封装 Engagement 事件消费循环。
type Runner struct {
	subscriber gcpubsub.Subscriber
	handler    *EventHandler
	decoder    *eventDecoder
	logger     *log.Helper
}

// RunnerParams 注入 Runner 所需依赖。
type RunnerParams struct {
	Subscriber gcpubsub.Subscriber
	Repository videoUserStatesStore
	TxManager  txmanager.Manager
	Logger     log.Logger
}

// NewRunner 构造 Engagement Runner。
func NewRunner(params RunnerParams) (*Runner, error) {
	if params.Subscriber == nil {
		return nil, fmt.Errorf("engagement: subscriber is required")
	}
	if params.Repository == nil {
		return nil, fmt.Errorf("engagement: repository is required")
	}
	if params.TxManager == nil {
		return nil, fmt.Errorf("engagement: tx manager is required")
	}
	metrics := newMetrics()
	handler := NewEventHandler(params.Repository, params.TxManager, params.Logger, metrics)
	return &Runner{
		subscriber: params.Subscriber,
		handler:    handler,
		decoder:    newEventDecoder(),
		logger:     log.NewHelper(params.Logger),
	}, nil
}

// Run 启动消费循环，直到 context 取消。
func (r *Runner) Run(ctx context.Context) error {
	if r == nil || r.subscriber == nil {
		return nil
	}
	return r.subscriber.Receive(ctx, r.processMessage)
}

func (r *Runner) processMessage(ctx context.Context, msg *gcpubsub.Message) error {
	if msg == nil {
		return nil
	}
	evt, err := r.decoder.Decode(msg.Data)
	if err != nil {
		if r.logger != nil {
			r.logger.WithContext(ctx).Warnw("msg", "decode engagement event failed", "error", err)
		}
		return nil
	}
	if err := r.handler.Handle(ctx, evt); err != nil {
		return err
	}
	return nil
}
