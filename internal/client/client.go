package client

import "github.com/google/wire"

// ProviderSet bundles client constructors for dependency injection.
var ProviderSet = wire.NewSet(NewGRPCClient)
