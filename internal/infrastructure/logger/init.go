package logger

import "github.com/google/wire"

// ProviderSet wires logger provider for dependency injection.
var ProviderSet = wire.NewSet(NewLogger)
