package repositories

import (
	"context"
	"fmt"
	"time"

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

// OutboxRepository 提供写入 Outbox 表的能力，确保与 TxManager Session 协作。
type OutboxRepository struct {
	baseQueries *catalogsql.Queries
	log         *log.Helper
}

// NewOutboxRepository 构造 Repository。
func NewOutboxRepository(db *pgxpool.Pool, logger log.Logger) *OutboxRepository {
	return &OutboxRepository{
		baseQueries: catalogsql.New(db),
		log:         log.NewHelper(logger),
	}
}

// Enqueue 在指定事务内插入 Outbox 事件。
func (r *OutboxRepository) Enqueue(ctx context.Context, sess txmanager.Session, msg OutboxMessage) error {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	availableAt := msg.AvailableAt.UTC()
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}

	params := catalogsql.InsertOutboxEventParams{
		EventID:       msg.EventID,
		AggregateType: msg.AggregateType,
		AggregateID:   msg.AggregateID,
		EventType:     msg.EventType,
		Payload:       msg.Payload,
		Headers:       msg.Headers,
		AvailableAt: pgtype.Timestamptz{
			Time:  availableAt,
			Valid: true,
		},
	}

	if _, err := queries.InsertOutboxEvent(ctx, params); err != nil {
		r.log.WithContext(ctx).Errorf("insert outbox event failed: event_id=%s err=%v", msg.EventID, err)
		return fmt.Errorf("insert outbox event: %w", err)
	}

	r.log.WithContext(ctx).Debugf("outbox event enqueued: aggregate=%s id=%s", msg.AggregateType, msg.AggregateID)
	return nil
}
