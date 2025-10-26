package projection

import (
	"context"
	"time"

	outboxcfg "github.com/bionicotaku/lingo-utils/outbox/config"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/outbox/inbox"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Task 负责消费 Pub/Sub 事件并更新投影表。
type Task struct {
	runner  *inbox.Runner[videov1.Event]
	handler *eventHandler
	metrics *projectionMetrics
	clock   func() time.Time
}

// NewTask 构造投影消费任务。
func NewTask(sub gcpubsub.Subscriber, inboxRepo *repositories.InboxRepository, projection *repositories.VideoProjectionRepository, tx txmanager.Manager, logger log.Logger, inboxCfg outboxcfg.InboxConfig) *Task {
	helper := log.NewHelper(logger)
	meter := otel.GetMeterProvider().Meter("kratos-template.projection")

	metrics := newProjectionMetrics(meter, helper)

	dec := newEventDecoder()
	h := newEventHandler(projection, logger, metrics)

	runner, err := inbox.NewRunner[videov1.Event](inbox.RunnerParams[videov1.Event]{
		Store:      inboxRepo.Shared(),
		Subscriber: sub,
		TxManager:  tx,
		Decoder:    dec,
		Handler:    h,
		Config:     inboxCfg,
		Logger:     logger,
	})
	if err != nil {
		helper.Errorw("msg", "init inbox runner failed", "error", err)
		return nil
	}

	return &Task{
		runner:  runner,
		handler: h,
		metrics: metrics,
		clock:   time.Now,
	}
}

// WithClock 提供测试替换时钟的能力。
func (t *Task) WithClock(fn func() time.Time) {
	if fn != nil {
		t.clock = fn
		if t.runner != nil {
			t.runner.WithClock(fn)
		}
		if t.handler != nil {
			t.handler.clock = fn
		}
	}
}

// Run 启动 StreamingPull 消费循环。
func (t *Task) Run(ctx context.Context) error {
	if t.runner == nil {
		return nil
	}
	return t.runner.Run(ctx)
}

// 以下函数保留供 eventHandler 调用。
func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return ts, nil
}

func mustTimestamp(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

type projectionMetrics struct {
	success metric.Int64Counter
	failure metric.Int64Counter
	lag     metric.Float64Histogram
	helper  *log.Helper
	enabled bool
}

const (
	metricNameProjectionSuccess = "projection_apply_success_total"
	metricNameProjectionFailure = "projection_apply_failure_total"
	metricNameProjectionLag     = "projection_event_lag_ms"
)

func newProjectionMetrics(meter metric.Meter, helper *log.Helper) *projectionMetrics {
	m := &projectionMetrics{helper: helper}
	if meter == nil {
		return m
	}

	var err error
	if m.success, err = meter.Int64Counter(metricNameProjectionSuccess,
		metric.WithDescription("Number of projection events applied successfully")); err != nil {
		helper.Warnf("projection metrics: register success counter: %v", err)
		return m
	}
	if m.failure, err = meter.Int64Counter(metricNameProjectionFailure,
		metric.WithDescription("Number of projection events failed to apply")); err != nil {
		helper.Warnf("projection metrics: register failure counter: %v", err)
	}
	if m.lag, err = meter.Float64Histogram(metricNameProjectionLag,
		metric.WithDescription("Event lag between occurred_at and processing time"), metric.WithUnit("ms")); err != nil {
		helper.Warnf("projection metrics: register lag histogram: %v", err)
	}
	m.enabled = true
	return m
}

func (m *projectionMetrics) recordSuccess(ctx context.Context, eventType string, occurredAt time.Time, now time.Time) {
	if m == nil || !m.enabled {
		return
	}
	attrs := metric.WithAttributes(attribute.String("event_type", eventType))
	if m.success != nil {
		m.success.Add(ctx, 1, attrs)
	}
	if m.lag != nil {
		lag := now.Sub(occurredAt).Milliseconds()
		if lag < 0 {
			lag = 0
		}
		m.lag.Record(ctx, float64(lag), attrs)
	}
}

func (m *projectionMetrics) recordFailure(ctx context.Context, eventType string, err error) {
	if m == nil || !m.enabled {
		return
	}
	attrs := metric.WithAttributes(attribute.String("event_type", eventType))
	if m.failure != nil {
		m.failure.Add(ctx, 1, attrs)
	}
	if m.helper != nil {
		m.helper.WithContext(ctx).Warnw("msg", "projection apply failed", "event_type", eventType, "error", err)
	}
}
