package controllers

import "github.com/google/wire"

// ProviderSet exposes controller/handler constructors for DI.
var ProviderSet = wire.NewSet(
	NewGreeterHandler,
	NewVideoHandler,
)
