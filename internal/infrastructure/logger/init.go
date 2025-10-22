package logger

import "github.com/google/wire"

// ProviderSet wires logger provider for Wire.
var ProviderSet = wire.NewSet(NewLogger)
