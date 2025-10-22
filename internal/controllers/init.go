package controllers

import "github.com/google/wire"

// ProviderSet is controllers providers.
var ProviderSet = wire.NewSet(NewGreeterController)
