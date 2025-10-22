package data

import (
	"github.com/bionicotaku/kratos-template/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

// Data wraps lower-level storage clients (database, cache, etc.).
type Data struct {
	// TODO: add concrete clients when ready.
}

// NewData constructs storage resources and returns a cleanup function.
func NewData(c *conf.Data, logger log.Logger) (*Data, func(), error) {
	helper := log.NewHelper(logger)
	cleanup := func() {
		helper.Info("closing the data resources")
	}
	return &Data{}, cleanup, nil
}
