package data

import "github.com/google/wire"

// ProviderSet bundles data-layer providers for Wire.
var ProviderSet = wire.NewSet(NewData)
