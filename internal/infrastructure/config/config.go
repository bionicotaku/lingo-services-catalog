package configinfra

import (
	"github.com/bionicotaku/kratos-template/internal/conf"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
)

// NewBootstrap loads bootstrap configuration using the provided flag path.
func NewBootstrap(confPath string) (config.Config, conf.Bootstrap, func(), error) {
	c := config.New(config.WithSource(file.NewSource(confPath)))
	if err := c.Load(); err != nil {
		return nil, conf.Bootstrap{}, func() {}, err
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		c.Close()
		return nil, conf.Bootstrap{}, func() {}, err
	}
	cleanup := func() {
		_ = c.Close()
	}
	return c, bc, cleanup, nil
}
