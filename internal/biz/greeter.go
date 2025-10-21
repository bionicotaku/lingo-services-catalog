package biz

import (
	"context"

	v1 "github.com/go-kratos/kratos-layout/api/helloworld/v1"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

var (
	// ErrUserNotFound is user not found.
	ErrUserNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "user not found")
)

// Greeter is a Greeter model.
type Greeter struct {
	Hello string
}

// GreeterRepo is a Greater repo.
type GreeterRepo interface {
	Save(context.Context, *Greeter) (*Greeter, error)
	Update(context.Context, *Greeter) (*Greeter, error)
	FindByID(context.Context, int64) (*Greeter, error)
	ListByHello(context.Context, string) ([]*Greeter, error)
	ListAll(context.Context) ([]*Greeter, error)
}

// GreeterRemote abstracts remote Greeter interaction.
type GreeterRemote interface {
	SayHello(ctx context.Context, name string) (string, error)
}

// GreeterUsecase is a Greeter usecase.
type GreeterUsecase struct {
	repo   GreeterRepo
	remote GreeterRemote
	log    *log.Helper
}

// NewGreeterUsecase new a Greeter usecase.
func NewGreeterUsecase(repo GreeterRepo, remote GreeterRemote, logger log.Logger) *GreeterUsecase {
	return &GreeterUsecase{repo: repo, remote: remote, log: log.NewHelper(logger)}
}

// CreateGreeter creates a Greeter, and returns the new Greeter.
func (uc *GreeterUsecase) CreateGreeter(ctx context.Context, g *Greeter) (*Greeter, error) {
	uc.log.WithContext(ctx).Infof("CreateGreeter: %v", g.Hello)
	return uc.repo.Save(ctx, g)
}

// ForwardHello calls the remote Greeter service if available.
func (uc *GreeterUsecase) ForwardHello(ctx context.Context, name string) (string, error) {
	if uc.remote == nil {
		return "", nil
	}
	return uc.remote.SayHello(ctx, name)
}
