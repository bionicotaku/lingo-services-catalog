package repositories

import (
	"context"
	"time"

	catalogsql "github.com/bionicotaku/kratos-template/internal/repositories/sqlc"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InboxEvent 描述需要写入 inbox_events 的事件数据。
type InboxEvent struct {
	EventID       uuid.UUID
	SourceService string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       []byte
}

// InboxRepository 封装 inbox_events 表的访问逻辑。
type InboxRepository struct {
	baseQueries *catalogsql.Queries
	log         *log.Helper
}

// NewInboxRepository 构造 InboxRepository。
func NewInboxRepository(db *pgxpool.Pool, logger log.Logger) *InboxRepository {
	return &InboxRepository{
		baseQueries: catalogsql.New(db),
		log:         log.NewHelper(logger),
	}
}

// Insert 在指定事务内写入 Inbox 事件，冲突时静默忽略。
func (r *InboxRepository) Insert(ctx context.Context, sess txmanager.Session, event InboxEvent) error {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	params := catalogsql.InsertInboxEventParams{
		EventID:       event.EventID,
		SourceService: event.SourceService,
		EventType:     event.EventType,
		AggregateType: textFromString(event.AggregateType),
		AggregateID:   textFromString(event.AggregateID),
		Payload:       append([]byte(nil), event.Payload...),
	}

	if err := queries.InsertInboxEvent(ctx, params); err != nil {
		return err
	}
	return nil
}

// MarkProcessed 在事务内标记事件处理成功。
func (r *InboxRepository) MarkProcessed(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, processedAt time.Time) error {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	return queries.MarkInboxEventProcessed(ctx, catalogsql.MarkInboxEventProcessedParams{
		EventID:     eventID,
		ProcessedAt: timestamptzFromTime(processedAt),
	})
}

// RecordError 记录事件处理错误信息。
func (r *InboxRepository) RecordError(ctx context.Context, sess txmanager.Session, eventID uuid.UUID, lastErr string) error {
	queries := r.baseQueries
	if sess != nil {
		queries = queries.WithTx(sess.Tx())
	}

	return queries.RecordInboxEventError(ctx, catalogsql.RecordInboxEventErrorParams{
		EventID:   eventID,
		LastError: pgtype.Text{String: lastErr, Valid: lastErr != ""},
	})
}
