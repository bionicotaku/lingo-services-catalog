package clients

import "github.com/google/wire"

// ProviderSet bundles business-level client providers for Wire.
var ProviderSet = wire.NewSet(NewGreeterRemote)
