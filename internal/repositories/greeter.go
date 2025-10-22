// Package repositories 提供数据访问层实现，负责与持久化存储交互。
// 该层实现 Service 层定义的 Repository 接口，隔离底层存储细节。
package repositories

import (
	"context"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/log"
)

// greeterRepo 是 services.GreeterRepo 接口的实现。
// 当前为测试桩实现，后续可替换为真实的数据库访问逻辑（如 sqlc、GORM 等）。
type greeterRepo struct {
	log *log.Helper // 结构化日志辅助器
	// TODO: 接入真实数据库时添加字段，如：
	// db *sql.DB 或 queries *sqlc.Queries
}

// NewGreeterRepo 构造 GreeterRepo 接口的实现实例。
// 通过 Wire 注入 logger，后续接入数据库时可注入 DB 连接池。
func NewGreeterRepo(logger log.Logger) services.GreeterRepo {
	return &greeterRepo{
		log: log.NewHelper(logger),
	}
}

// Save 保存 Greeter 实体到持久化存储。
// 当前为桩实现，直接返回传入的实体。
// TODO: 实现真实的数据库插入逻辑，如：
//   return r.queries.InsertGreeter(ctx, params)
func (r *greeterRepo) Save(_ context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

// Update 更新已有的 Greeter 实体。
// 当前为桩实现。
// TODO: 实现真实的数据库更新逻辑。
func (r *greeterRepo) Update(_ context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

// FindByID 根据 ID 查询 Greeter 实体。
// 当前为桩实现，返回 nil。
// TODO: 实现真实的查询逻辑，处理 NotFound 错误。
func (r *greeterRepo) FindByID(_ context.Context, _ int64) (*po.Greeter, error) {
	return nil, nil
}

// ListByHello 根据 Hello 字段查询匹配的 Greeter 实体列表。
// 当前为桩实现。
// TODO: 实现真实的条件查询逻辑。
func (r *greeterRepo) ListByHello(_ context.Context, _ string) ([]*po.Greeter, error) {
	return nil, nil
}

// ListAll 查询所有 Greeter 实体。
// 当前为桩实现。
// TODO: 实现分页查询逻辑，避免返回大数据集。
func (r *greeterRepo) ListAll(_ context.Context) ([]*po.Greeter, error) {
	return nil, nil
}
