package grpcserver

import "github.com/google/wire"

// ProviderSet bundles the gRPC server provider for Wire.
var ProviderSet = wire.NewSet(NewGRPCServer)
