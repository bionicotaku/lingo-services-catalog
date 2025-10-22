package grpcclient

import "github.com/google/wire"

// ProviderSet bundles gRPC client connection providers for Wire.
var ProviderSet = wire.NewSet(NewGRPCClient)
