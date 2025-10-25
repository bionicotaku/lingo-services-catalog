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
