package services

import "github.com/google/wire"

// ProviderSet is services providers.
var ProviderSet = wire.NewSet(NewGreeterUsecase)
