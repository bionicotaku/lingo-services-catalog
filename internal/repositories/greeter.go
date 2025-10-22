package repositories

import (
	"context"

	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/log"
)

type greeterRepo struct {
	log *log.Helper
}

// NewGreeterRepo constructs repository implementation.
func NewGreeterRepo(logger log.Logger) services.GreeterRepo {
	return &greeterRepo{
		log: log.NewHelper(logger),
	}
}

func (r *greeterRepo) Save(ctx context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

func (r *greeterRepo) Update(ctx context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

func (r *greeterRepo) FindByID(context.Context, int64) (*po.Greeter, error) {
	return nil, nil
}

func (r *greeterRepo) ListByHello(context.Context, string) ([]*po.Greeter, error) {
	return nil, nil
}

func (r *greeterRepo) ListAll(context.Context) ([]*po.Greeter, error) {
	return nil, nil
}
