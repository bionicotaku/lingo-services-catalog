package repositories

import (
	"github.com/bionicotaku/kratos-template/internal/infrastructure/data"
	"github.com/google/wire"
)

// ProviderSet bundles repository providers for Wire.
var ProviderSet = wire.NewSet(
	data.ProviderSet,
	NewGreeterRepo,
)
