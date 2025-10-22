// Package repositories hosts data access implementations for the template.
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

func (r *greeterRepo) Save(_ context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

func (r *greeterRepo) Update(_ context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

func (r *greeterRepo) FindByID(_ context.Context, _ int64) (*po.Greeter, error) {
	return nil, nil
}

func (r *greeterRepo) ListByHello(_ context.Context, _ string) ([]*po.Greeter, error) {
	return nil, nil
}

func (r *greeterRepo) ListAll(_ context.Context) ([]*po.Greeter, error) {
	return nil, nil
}
