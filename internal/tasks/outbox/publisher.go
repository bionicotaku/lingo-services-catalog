package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/go-kratos/kratos/v2/log"
	"golang.org/x/sync/errgroup"
)

// Config 定义 Outbox Publisher 的运行参数。
type Config struct {
	BatchSize      int
	TickInterval   time.Duration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	MaxAttempts    int
	PublishTimeout time.Duration
	Workers        int
}

// PublisherTask 负责扫描 Outbox 并将事件发布到 Pub/Sub。
type PublisherTask struct {
	repo      *repositories.OutboxRepository
	publisher gcpubsub.Publisher
	cfg       Config
	clock     func() time.Time
	log       *log.Helper
}

// NewPublisherTask 构造发布器任务。
func NewPublisherTask(repo *repositories.OutboxRepository, pub gcpubsub.Publisher, cfg Config, logger log.Logger) *PublisherTask {
	return &PublisherTask{
		repo:      repo,
		publisher: pub,
		cfg:       sanitizeConfig(cfg),
		clock:     time.Now,
		log:       log.NewHelper(logger),
	}
}

// WithClock 允许在测试中注入自定义时钟。
func (t *PublisherTask) WithClock(clock func() time.Time) {
	if clock != nil {
		t.clock = clock
	}
}

// Run 启动发布循环，直到 ctx 被取消。
func (t *PublisherTask) Run(ctx context.Context) error {
	ticker := time.NewTicker(t.cfg.TickInterval)
	defer ticker.Stop()

	for {
		if err := t.drain(ctx); err != nil {
			t.log.WithContext(ctx).Errorf("outbox publish: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (t *PublisherTask) drain(ctx context.Context) error {
	now := t.clock()
	events, err := t.repo.ClaimPending(ctx, now, t.cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	t.log.WithContext(ctx).Infow("msg", "outbox batch", "count", len(events))

	var successCount, failureCount int32
	workers := t.cfg.Workers
	if workers <= 1 {
		for _, event := range events {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := t.publishOnce(ctx, event); err != nil {
				atomic.AddInt32(&failureCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}
	} else {
		sem := make(chan struct{}, workers)
		grp, grpCtx := errgroup.WithContext(ctx)
		for _, event := range events {
			event := event
			sem <- struct{}{}
			grp.Go(func() error {
				defer func() { <-sem }()
				if err := t.publishOnce(grpCtx, event); err != nil {
					atomic.AddInt32(&failureCount, 1)
				} else {
					atomic.AddInt32(&successCount, 1)
				}
				return nil
			})
		}
		if err := grp.Wait(); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	t.log.WithContext(ctx).Infow(
		"msg", "outbox batch finished",
		"count", len(events),
		"success", atomic.LoadInt32(&successCount),
		"failure", atomic.LoadInt32(&failureCount),
	)

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func (t *PublisherTask) publishOnce(ctx context.Context, event repositories.OutboxEvent) error {
	publishCtx := ctx
	var cancel context.CancelFunc
	if t.cfg.PublishTimeout > 0 {
		publishCtx, cancel = context.WithTimeout(ctx, t.cfg.PublishTimeout)
		defer cancel()
	}

	attributes := make(map[string]string)
	if len(event.Headers) > 0 {
		if err := json.Unmarshal(event.Headers, &attributes); err != nil {
			t.log.WithContext(ctx).Warnf("decode outbox headers: event_id=%s err=%v", event.EventID, err)
			attributes = map[string]string{}
		}
	}

	msg := gcpubsub.Message{
		Data:            event.Payload,
		Attributes:      attributes,
		OrderingKey:     event.AggregateID.String(),
		EventID:         event.EventID.String(),
		PublishTime:     event.OccurredAt,
		DeliveryAttempt: int(event.DeliveryAttempts) + 1,
	}

	start := t.clock()
	_, err := t.publisher.Publish(publishCtx, msg)
	latency := t.clock().Sub(start)

	if err != nil {
		t.log.WithContext(ctx).Warnw("msg", "outbox publish failed", "event_id", event.EventID, "aggregate_id", event.AggregateID, "attempt", event.DeliveryAttempts+1, "latency_ms", latency.Milliseconds(), "error", err)
		return t.handleFailure(ctx, event, err)
	}

	t.log.WithContext(ctx).Infow("msg", "outbox publish success", "event_id", event.EventID, "aggregate_id", event.AggregateID, "attempt", event.DeliveryAttempts+1, "latency_ms", latency.Milliseconds())
	return t.repo.MarkPublished(ctx, nil, event.EventID, t.clock())
}

func (t *PublisherTask) handleFailure(ctx context.Context, event repositories.OutboxEvent, publishErr error) error {
	now := t.clock()
	next := now.Add(t.backoffDuration(int(event.DeliveryAttempts)))
	lastErr := publishErr.Error()

	if err := t.repo.Reschedule(ctx, nil, event.EventID, next, lastErr); err != nil {
		return err
	}

	if t.cfg.MaxAttempts > 0 && int(event.DeliveryAttempts)+1 >= t.cfg.MaxAttempts {
		t.log.WithContext(ctx).Warnw("msg", "outbox retries exhausted", "event_id", event.EventID, "aggregate_id", event.AggregateID, "attempts", event.DeliveryAttempts+1)
	}
	return nil
}

func (t *PublisherTask) backoffDuration(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	backoff := t.cfg.InitialBackoff * time.Duration(1<<attempts)
	if backoff <= 0 {
		backoff = t.cfg.InitialBackoff
	}
	if t.cfg.MaxBackoff > 0 && backoff > t.cfg.MaxBackoff {
		backoff = t.cfg.MaxBackoff
	}
	return backoff
}

func sanitizeConfig(cfg Config) Config {
	result := cfg
	if result.BatchSize <= 0 {
		result.BatchSize = defaultBatchSize
	}
	if result.TickInterval <= 0 {
		result.TickInterval = defaultTickInterval
	}
	if result.InitialBackoff <= 0 {
		result.InitialBackoff = defaultInitialBackoff
	}
	if result.MaxBackoff <= 0 {
		result.MaxBackoff = defaultMaxBackoff
	}
	if result.MaxAttempts <= 0 {
		result.MaxAttempts = defaultMaxAttempts
	}
	if result.PublishTimeout <= 0 {
		result.PublishTimeout = defaultPublishTimeout
	}
	if result.Workers <= 0 {
		result.Workers = defaultWorkers
	}
	if result.Workers > result.BatchSize && result.BatchSize > 0 {
		result.Workers = result.BatchSize
	}
	return result
}

const (
	defaultBatchSize      = 100
	defaultTickInterval   = time.Second
	defaultInitialBackoff = 2 * time.Second
	defaultMaxBackoff     = 120 * time.Second
	defaultMaxAttempts    = 20
	defaultPublishTimeout = 10 * time.Second
	defaultWorkers        = 4
)
