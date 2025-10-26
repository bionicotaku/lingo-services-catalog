package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-utils/outbox/store"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type eventHandler struct {
	projections *repositories.VideoProjectionRepository
	log         *log.Helper
	metrics     *projectionMetrics
	clock       func() time.Time
}

func newEventHandler(repo *repositories.VideoProjectionRepository, logger log.Logger, metrics *projectionMetrics) *eventHandler {
	return &eventHandler{
		projections: repo,
		log:         log.NewHelper(logger),
		metrics:     metrics,
		clock:       time.Now,
	}
}

func (h *eventHandler) Handle(ctx context.Context, sess txmanager.Session, evt *videov1.Event, inboxEvt *store.InboxEvent) error {
	aggID := evt.GetAggregateId()
	if aggID == "" && inboxEvt.AggregateID != nil {
		aggID = *inboxEvt.AggregateID
	}
	videoID, err := uuid.Parse(aggID)
	if err != nil {
		return fmt.Errorf("projection: parse aggregate_id: %w", err)
	}

	eventType := evt.GetEventType().String()
	if eventType == "" && inboxEvt.EventType != "" {
		eventType = inboxEvt.EventType
	}

	occurredAt, err := parseTime(evt.GetOccurredAt())
	if err != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx, eventType, err)
		}
		return fmt.Errorf("projection: parse occurred_at: %w", err)
	}
	if occurredAt.IsZero() {
		occurredAt = h.clock().UTC()
	}

	var handleErr error
	switch evt.GetEventType() {
	case videov1.EventType_EVENT_TYPE_VIDEO_CREATED:
		payload := evt.GetCreated()
		if payload == nil {
			handleErr = errors.New("projection: created payload missing")
			break
		}
		handleErr = h.handleCreated(ctx, sess, evt, payload, videoID, occurredAt)
	case videov1.EventType_EVENT_TYPE_VIDEO_UPDATED:
		payload := evt.GetUpdated()
		if payload == nil {
			handleErr = errors.New("projection: updated payload missing")
			break
		}
		handleErr = h.handleUpdated(ctx, sess, evt, payload, videoID, occurredAt)
	case videov1.EventType_EVENT_TYPE_VIDEO_DELETED:
		payload := evt.GetDeleted()
		if payload == nil {
			handleErr = errors.New("projection: deleted payload missing")
			break
		}
		handleErr = h.handleDeleted(ctx, sess, evt, payload, videoID)
	default:
		h.log.WithContext(ctx).Warnw("msg", "projection: skip unknown event type", "event_type", evt.GetEventType().String(), "event_id", evt.GetEventId())
		return nil
	}

	if handleErr != nil {
		if h.metrics != nil {
			h.metrics.recordFailure(ctx, eventType, handleErr)
		}
		return handleErr
	}

	if h.metrics != nil {
		h.metrics.recordSuccess(ctx, eventType, occurredAt, h.clock())
	}
	return nil
}

func (h *eventHandler) handleCreated(ctx context.Context, sess txmanager.Session, evt *videov1.Event, payload *videov1.Event_VideoCreated, videoID uuid.UUID, occurredAt time.Time) error {
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

	if err := h.projections.Upsert(ctx, sess, record); err != nil {
		return fmt.Errorf("projection: upsert created: %w", err)
	}
	return nil
}

func (h *eventHandler) handleUpdated(ctx context.Context, sess txmanager.Session, evt *videov1.Event, payload *videov1.Event_VideoUpdated, videoID uuid.UUID, occurredAt time.Time) error {
	current, err := h.projections.Get(ctx, sess, videoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.log.WithContext(ctx).Warnw("msg", "projection: skip update without existing projection", "video_id", videoID, "event_version", evt.GetVersion())
			return nil
		}
		return fmt.Errorf("projection: load current projection: %w", err)
	}

	record := repositories.VideoProjection{
		VideoID:        videoID,
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

	if err := h.projections.Upsert(ctx, sess, record); err != nil {
		return fmt.Errorf("projection: upsert updated: %w", err)
	}
	return nil
}

func (h *eventHandler) handleDeleted(ctx context.Context, sess txmanager.Session, evt *videov1.Event, payload *videov1.Event_VideoDeleted, videoID uuid.UUID) error {
	version := evt.GetVersion()
	if payload.GetVersion() > 0 {
		version = payload.GetVersion()
	}
	if version == 0 {
		version = h.clock().UnixNano()
	}

	if err := h.projections.Delete(ctx, sess, videoID, version); err != nil {
		return fmt.Errorf("projection: delete projection: %w", err)
	}
	return nil
}
