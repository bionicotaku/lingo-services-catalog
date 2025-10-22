package repositories

import "github.com/google/wire"

// ProviderSet bundles repository providers for Wire.
var ProviderSet = wire.NewSet(
	NewGreeterRepo,
)
