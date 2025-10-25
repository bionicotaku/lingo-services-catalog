package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/protobuf/proto"
)

// Task 负责消费 Pub/Sub 事件并更新投影表。
type Task struct {
	subscriber     gcpubsub.Subscriber
	inboxRepo      *repositories.InboxRepository
	projectionRepo *repositories.VideoProjectionRepository
	txManager      txmanager.Manager
	log            *log.Helper
	clock          func() time.Time
	sourceService  string
	metrics        *projectionMetrics
}

// NewTask 构造投影消费任务。
func NewTask(sub gcpubsub.Subscriber, inbox *repositories.InboxRepository, projection *repositories.VideoProjectionRepository, tx txmanager.Manager, logger log.Logger) *Task {
	helper := log.NewHelper(logger)
	meter := otel.GetMeterProvider().Meter("kratos-template.projection")
	return &Task{
		subscriber:     sub,
		inboxRepo:      inbox,
		projectionRepo: projection,
		txManager:      tx,
		log:            helper,
		clock:          time.Now,
		sourceService:  "catalog",
		metrics:        newProjectionMetrics(meter, helper),
	}
}

// WithClock 提供测试替换时钟的能力。
func (t *Task) WithClock(fn func() time.Time) {
	if fn != nil {
		t.clock = fn
	}
}

// Run 启动 StreamingPull 消费循环。
func (t *Task) Run(ctx context.Context) error {
	if t.subscriber == nil {
		return nil
	}
	return t.subscriber.Receive(ctx, t.handleMessage)
}

func (t *Task) handleMessage(ctx context.Context, msg *gcpubsub.Message) error {
	if msg == nil {
		return errors.New("projection: nil message")
	}

	event := &videov1.Event{}
	if err := proto.Unmarshal(msg.Data, event); err != nil {
		return fmt.Errorf("projection: decode event: %w", err)
	}

	eventID, err := uuid.Parse(event.GetEventId())
	if err != nil {
		return fmt.Errorf("projection: parse event_id: %w", err)
	}
	videoID, err := uuid.Parse(event.GetAggregateId())
	if err != nil {
		return fmt.Errorf("projection: parse aggregate_id: %w", err)
	}

	occurredAt, err := parseTime(event.GetOccurredAt())
	if err != nil {
		return fmt.Errorf("projection: parse occurred_at: %w", err)
	}
	if occurredAt.IsZero() {
		occurredAt = t.clock().UTC()
	}

	eventType := event.GetEventType().String()

	txErr := t.txManager.WithinTx(ctx, txmanager.TxOptions{}, func(txCtx context.Context, sess txmanager.Session) error {
		insertErr := t.inboxRepo.Insert(txCtx, sess, repositories.InboxEvent{
			EventID:       eventID,
			SourceService: t.sourceService,
			EventType:     event.GetEventType().String(),
			AggregateType: event.GetAggregateType(),
			AggregateID:   event.GetAggregateId(),
			Payload:       msg.Data,
		})
		if insertErr != nil {
			return fmt.Errorf("projection: insert inbox event: %w", insertErr)
		}

		if err := t.applyEvent(txCtx, sess, event, videoID, occurredAt); err != nil {
			return err
		}

		if err := t.inboxRepo.MarkProcessed(txCtx, sess, eventID, t.clock().UTC()); err != nil {
			return fmt.Errorf("projection: mark inbox processed: %w", err)
		}
		return nil
	})
	if txErr != nil {
		if t.metrics != nil {
			t.metrics.recordFailure(ctx, eventType, txErr)
		}
		return txErr
	}
	if t.metrics != nil {
		t.metrics.recordSuccess(ctx, eventType, occurredAt, t.clock())
	}
	return nil
}

func (t *Task) applyEvent(ctx context.Context, sess txmanager.Session, evt *videov1.Event, videoID uuid.UUID, occurredAt time.Time) error {
	switch evt.GetEventType() {
	case videov1.EventType_EVENT_TYPE_VIDEO_CREATED:
		payload := evt.GetCreated()
		if payload == nil {
			return errors.New("projection: created payload missing")
		}
		return t.handleCreated(ctx, sess, evt, payload, videoID, occurredAt)
	case videov1.EventType_EVENT_TYPE_VIDEO_UPDATED:
		payload := evt.GetUpdated()
		if payload == nil {
			return errors.New("projection: updated payload missing")
		}
		return t.handleUpdated(ctx, sess, evt, payload, videoID, occurredAt)
	case videov1.EventType_EVENT_TYPE_VIDEO_DELETED:
		payload := evt.GetDeleted()
		if payload == nil {
			return errors.New("projection: deleted payload missing")
		}
		return t.handleDeleted(ctx, sess, evt, payload, videoID)
	default:
		t.log.WithContext(ctx).Warnw("msg", "projection: skip unknown event type", "event_type", evt.GetEventType().String(), "event_id", evt.GetEventId())
		return nil
	}
}

func (t *Task) handleCreated(ctx context.Context, sess txmanager.Session, evt *videov1.Event, payload *videov1.Event_VideoCreated, videoID uuid.UUID, occurredAt time.Time) error {
	title := payload.GetTitle()
	if title == "" {
		title = "(untitled)"
	}
	status := po.VideoStatus(payload.GetStatus())
	mediaStatus := po.StageStatus(payload.GetMediaStatus())
	analysisStatus := po.StageStatus(payload.GetAnalysisStatus())

	createdAt, err := parseTime(payload.GetOccurredAt())
	if err != nil {
		return fmt.Errorf("projection: parse created.occurred_at: %w", err)
	}
	if createdAt.IsZero() {
		createdAt = occurredAt
	}

	record := repositories.VideoProjection{
		VideoID:        videoID,
		Title:          title,
		Status:         status,
		MediaStatus:    mediaStatus,
		AnalysisStatus: analysisStatus,
		CreatedAt:      createdAt.UTC(),
		UpdatedAt:      occurredAt.UTC(),
		Version:        evt.GetVersion(),
		OccurredAt:     occurredAt.UTC(),
	}

	if err := t.projectionRepo.Upsert(ctx, sess, record); err != nil {
		return fmt.Errorf("projection: upsert created: %w", err)
	}
	return nil
}

func (t *Task) handleUpdated(ctx context.Context, sess txmanager.Session, evt *videov1.Event, payload *videov1.Event_VideoUpdated, videoID uuid.UUID, occurredAt time.Time) error {
	current, err := t.projectionRepo.Get(ctx, sess, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			t.log.WithContext(ctx).Warnw("msg", "projection: skip update without existing projection", "video_id", videoID, "event_version", evt.GetVersion())
			return nil
		}
		return fmt.Errorf("projection: load projection: %w", err)
	}

	record := repositories.VideoProjection{
		VideoID:        current.VideoID,
		Title:          current.Title,
		Status:         current.Status,
		MediaStatus:    current.MediaStatus,
		AnalysisStatus: current.AnalysisStatus,
		CreatedAt:      mustTimestamp(current.CreatedAt),
		UpdatedAt:      occurredAt.UTC(),
		Version:        evt.GetVersion(),
		OccurredAt:     occurredAt.UTC(),
	}

	if payload.Title != nil {
		record.Title = payload.GetTitle()
	}
	if payload.Status != nil {
		record.Status = po.VideoStatus(payload.GetStatus())
	}
	if payload.MediaStatus != nil {
		record.MediaStatus = po.StageStatus(payload.GetMediaStatus())
	}
	if payload.AnalysisStatus != nil {
		record.AnalysisStatus = po.StageStatus(payload.GetAnalysisStatus())
	}

	if err := t.projectionRepo.Upsert(ctx, sess, record); err != nil {
		return fmt.Errorf("projection: upsert updated: %w", err)
	}
	return nil
}

func (t *Task) handleDeleted(ctx context.Context, sess txmanager.Session, evt *videov1.Event, payload *videov1.Event_VideoDeleted, videoID uuid.UUID) error {
	version := evt.GetVersion()
	if payload.GetVersion() > 0 {
		version = payload.GetVersion()
	}
	if version == 0 {
		version = time.Now().UnixNano()
	}

	if err := t.projectionRepo.Delete(ctx, sess, videoID, version); err != nil {
		return fmt.Errorf("projection: delete projection: %w", err)
	}
	return nil
}

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
