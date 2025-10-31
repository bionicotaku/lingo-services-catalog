package uploads

import (
	"context"
	"fmt"

	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/outbox/config"
	"github.com/bionicotaku/lingo-utils/outbox/inbox"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
)

// Runner 负责消费 GCS OBJECT_FINALIZE 事件。
type Runner struct {
	delegate *inbox.Runner[Event]
}

// RunnerParams 注入构建 Runner 所需的依赖。
type RunnerParams struct {
	Subscriber gcpubsub.Subscriber
	InboxRepo  *repositories.InboxRepository
	UploadRepo *repositories.UploadRepository
	Lifecycle  *services.LifecycleWriter
	TxManager  txmanager.Manager
	Logger     log.Logger
	Config     config.InboxConfig
}

// NewRunner 构造上传事件 Runner。
func NewRunner(params RunnerParams) (*Runner, error) {
	if params.Subscriber == nil {
		return nil, fmt.Errorf("uploads: subscriber is required")
	}
	if params.InboxRepo == nil {
		return nil, fmt.Errorf("uploads: inbox repository is required")
	}
	if params.UploadRepo == nil {
		return nil, fmt.Errorf("uploads: upload repository is required")
	}
	if params.Lifecycle == nil {
		return nil, fmt.Errorf("uploads: lifecycle writer is required")
	}
	if params.TxManager == nil {
		return nil, fmt.Errorf("uploads: transaction manager is required")
	}

	handler := NewHandler(params.UploadRepo, params.Lifecycle, params.Logger)
	decoder := newDecoder()

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

	return &Runner{delegate: delegate}, nil
}

// Run 启动消费循环。
func (r *Runner) Run(ctx context.Context) error {
	if r == nil || r.delegate == nil {
		return nil
	}
	return r.delegate.Run(ctx)
}
