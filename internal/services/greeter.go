package services

import (
	"context"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/models/vo"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

// ErrUserNotFound is user not found.
var ErrUserNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "user not found")

// GreeterRepo describes persistence behavior for greeter entities.
type GreeterRepo interface {
	Save(context.Context, *po.Greeter) (*po.Greeter, error)
	Update(context.Context, *po.Greeter) (*po.Greeter, error)
	FindByID(context.Context, int64) (*po.Greeter, error)
	ListByHello(context.Context, string) ([]*po.Greeter, error)
	ListAll(context.Context) ([]*po.Greeter, error)
}

// GreeterRemote abstracts remote Greeter interaction.
type GreeterRemote interface {
	SayHello(ctx context.Context, name string) (string, error)
}

// GreeterUsecase encapsulates greeter business logic.
type GreeterUsecase struct {
	repo   GreeterRepo
	remote GreeterRemote
	log    *log.Helper
}

// NewGreeterUsecase constructs a Greeter usecase.
func NewGreeterUsecase(repo GreeterRepo, remote GreeterRemote, logger log.Logger) *GreeterUsecase {
	return &GreeterUsecase{repo: repo, remote: remote, log: log.NewHelper(logger)}
}

// CreateGreeting persists a greeter entity and returns an aggregated greeting message.
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

// ForwardHello calls the remote Greeter service if available.
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
