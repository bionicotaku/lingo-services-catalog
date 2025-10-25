package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/bionicotaku/kratos-template/internal/repositories/mappers"
	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxMessage 描述需要写入 outbox_events 的事件数据。
type OutboxMessage struct {
	EventID       uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       []byte
	Headers       []byte
	AvailableAt   time.Time
}

// OutboxEvent 表示从数据库读取的待发布事件。
type OutboxEvent struct {
	EventID          uuid.UUID
	AggregateType    string
	AggregateID      uuid.UUID
	EventType        string
	Payload          []byte
	Headers          []byte
	OccurredAt       time.Time
	AvailableAt      time.Time
	PublishedAt      *time.Time
	DeliveryAttempts int32
	LastError        *string
}

// OutboxRepository 提供写入 Outbox 表的能力，确保与 TxManager Session 协作。
type OutboxRepository struct {
	baseQueries *catalogsql.Queries
	log         *log.Helper
}

// NewOutboxRepository 构造 Repository。
func NewOutboxRepository(db *pgxpool.Pool, logger log.Logger) *OutboxRepository {
	// 预初始化 sqlc Queries 与日志 helper，减少每次调用的重复工作。
	return &OutboxRepository{
		baseQueries: catalogsql.New(db),
		log:         log.NewHelper(logger),
	}
}

// Enqueue 在指定事务内插入 Outbox 事件。
func (r *OutboxRepository) Enqueue(ctx context.Context, sess txmanager.Session, msg OutboxMessage) error {
	// 默认使用根查询对象；若处于事务，则切换到 tx 绑定的 sqlc Queries。
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	// 统一 AvailableAt 为 UTC，缺省时自动填当前时间，方便调度器排序。
	availableAt := msg.AvailableAt.UTC()
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}

	// 组装 sqlc 所需参数，包含事件头、载荷与聚合标识。
	params := mappers.BuildInsertOutboxEventParams(
		msg.EventID,
		msg.AggregateType,
		msg.AggregateID,
		msg.EventType,
		msg.Payload,
		msg.Headers,
		availableAt,
	)

	// 调用 sqlc 生成的 InsertOutboxEvent，确保失败时返回带上下文的错误。
	if _, err := queries.InsertOutboxEvent(ctx, params); err != nil {
		r.log.WithContext(ctx).Errorf("insert outbox event failed: event_id=%s err=%v", msg.EventID, err)
		return fmt.Errorf("insert outbox event: %w", err)
	}

	// Debug 日志记录成功写入，便于排障或验证幂等。
	r.log.WithContext(ctx).Debugf("outbox event enqueued: aggregate=%s id=%s", msg.AggregateType, msg.AggregateID)
	return nil
}

// ClaimPending 返回一批待发布的 Outbox 事件。
func (r *OutboxRepository) ClaimPending(ctx context.Context, availableBefore time.Time, limit int) ([]OutboxEvent, error) {
	params := catalogsql.ClaimPendingOutboxEventsParams{
		AvailableAt: timestamptzFromTime(availableBefore),
		Limit:       int32(limit),
	}
	records, err := r.baseQueries.ClaimPendingOutboxEvents(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	events := make([]OutboxEvent, 0, len(records))
	for _, rec := range records {
		events = append(events, outboxEventFromRecord(rec))
	}
	return events, nil
}

// MarkPublished 更新事件状态为已发布。
func (r *OutboxRepository) MarkPublished(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, publishedAt time.Time) error {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}
	params := catalogsql.MarkOutboxEventPublishedParams{
		EventID:     eventID,
		PublishedAt: timestamptzFromTime(publishedAt),
	}
	if err := queries.MarkOutboxEventPublished(ctx, params); err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}

// Reschedule 将事件重新安排在未来时间发布，并记录错误信息。
func (r *OutboxRepository) Reschedule(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, nextAvailable time.Time, lastErr string) error {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}
	params := catalogsql.RescheduleOutboxEventParams{
		EventID:     eventID,
		LastError:   textFromString(lastErr),
		AvailableAt: timestamptzFromTime(nextAvailable),
	}
	if err := queries.RescheduleOutboxEvent(ctx, params); err != nil {
		return fmt.Errorf("reschedule outbox event: %w", err)
	}
	return nil
}

func outboxEventFromRecord(rec catalogsql.CatalogOutboxEvent) OutboxEvent {
	var publishedAt *time.Time
	if rec.PublishedAt.Valid {
		value := rec.PublishedAt.Time
		publishedAt = &value
	}
	var lastErr *string
	if rec.LastError.Valid {
		value := rec.LastError.String
		lastErr = &value
	}
	return OutboxEvent{
		EventID:          rec.EventID,
		AggregateType:    rec.AggregateType,
		AggregateID:      rec.AggregateID,
		EventType:        rec.EventType,
		Payload:          rec.Payload,
		Headers:          rec.Headers,
		OccurredAt:       mustTimestamp(rec.OccurredAt),
		AvailableAt:      mustTimestamp(rec.AvailableAt),
		PublishedAt:      publishedAt,
		DeliveryAttempts: rec.DeliveryAttempts,
		LastError:        lastErr,
	}
}

func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func mustTimestamp(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

func textFromString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}
