package services

import (
	"context"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

// ErrUserNotFound 是当用户未找到时返回的哨兵错误。
// 使用 Kratos errors.NotFound 包装，便于上层统一错误处理。
var ErrUserNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "user not found")

// GreeterRepo 定义 Greeter 实体的持久化行为接口。
// 由 Repository 层实现，Service 层通过接口调用以保持解耦。
type GreeterRepo interface {
	Save(context.Context, *po.Greeter) (*po.Greeter, error)     // 保存新的 Greeter 实体
	Update(context.Context, *po.Greeter) (*po.Greeter, error)   // 更新已有实体
	FindByID(context.Context, int64) (*po.Greeter, error)       // 根据 ID 查询
	ListByHello(context.Context, string) ([]*po.Greeter, error) // 根据 Hello 字段查询列表
	ListAll(context.Context) ([]*po.Greeter, error)             // 查询所有实体
}

// GreeterRemote 抽象远程 Greeter 服务的交互接口。
// 由 Clients 层实现，用于跨服务调用。
type GreeterRemote interface {
	SayHello(ctx context.Context, name string) (string, error) // 调用远程 Greeter 服务
}

// GreeterUsecase 封装 Greeter 相关的业务用例逻辑。
// 组合了本地仓储（repo）和远程服务客户端（remote），实现核心业务流程。
type GreeterUsecase struct {
	repo   GreeterRepo   // 本地数据访问接口
	remote GreeterRemote // 远程服务调用接口（可选）
	log    *log.Helper   // 结构化日志辅助器
}

// NewGreeterUsecase 构造一个 Greeter 业务用例实例。
// 通过 Wire 注入 repo、remote 和 logger，实现依赖倒置。
func NewGreeterUsecase(repo GreeterRepo, remote GreeterRemote, logger log.Logger) *GreeterUsecase {
	return &GreeterUsecase{repo: repo, remote: remote, log: log.NewHelper(logger)}
}

// CreateGreeting 创建问候语并持久化到仓储。
//
// 业务流程：
// 1. 根据 name 构造 Greeter 实体
// 2. 通过 repo 保存到持久化存储
// 3. 生成问候语消息并记录日志
// 4. 返回视图对象（vo.Greeting）供上层使用
//
// 返回值使用 vo 而非 po，避免暴露内部数据结构。
func (uc *GreeterUsecase) CreateGreeting(ctx context.Context, name string) (*vo.Greeting, error) {
	entity := &po.Greeter{Hello: name}
	saved, err := uc.repo.Save(ctx, entity)
	if err != nil {
		return nil, err
	}

	message := "Hello " + saved.Hello
	uc.log.WithContext(ctx).Infof("CreateGreeting: %s", message)
	return &vo.Greeting{Message: message}, nil
}

// ForwardHello 调用远程 Greeter 服务获取问候语（如果可用）。
//
// 行为：
// - 如果 remote 未配置（nil），返回空字符串，不报错
// - 如果远程调用失败，记录错误日志并返回错误
//
// 这种设计允许服务在无远程依赖时优雅降级。
func (uc *GreeterUsecase) ForwardHello(ctx context.Context, name string) (string, error) {
	if uc.remote == nil {
		return "", nil
	}
	msg, err := uc.remote.SayHello(ctx, name)
	if err != nil {
		uc.log.WithContext(ctx).Errorf("forward hello remote call failed: %v", err)
		return "", err
	}
	return msg, nil
}
