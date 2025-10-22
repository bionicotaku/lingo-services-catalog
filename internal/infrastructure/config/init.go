package configinfra

import "github.com/google/wire"

// ProviderSet registers configuration loader for Wire.
var ProviderSet = wire.NewSet(NewBootstrap)
