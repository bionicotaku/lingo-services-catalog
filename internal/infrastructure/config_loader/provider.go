package loader

import (
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	"github.com/google/wire"
)

// ProviderSet exposes configuration-derived dependencies for Wire graphs.
var ProviderSet = wire.NewSet(
	ProvideServiceMetadata,
	ProvideBootstrap,
	ProvideServerConfig,
	ProvideDataConfig,
	ProvideObservabilityConfig,
	ProvideLoggerConfig,
	ProvideObservabilityInfo,
)

// ProvideServiceMetadata returns the resolved ServiceMetadata from the loader.
func ProvideServiceMetadata(l *Loader) ServiceMetadata {
	if l == nil {
		return ServiceMetadata{}
	}
	return l.Service
}

// ProvideBootstrap exposes the strongly typed bootstrap configuration.
func ProvideBootstrap(l *Loader) *configpb.Bootstrap {
	if l == nil {
		return nil
	}
	return l.Bootstrap
}

// ProvideServerConfig returns the server section of the bootstrap configuration.
func ProvideServerConfig(bc *configpb.Bootstrap) *configpb.Server {
	if bc == nil {
		return nil
	}
	return bc.GetServer()
}

// ProvideDataConfig returns the data section of the bootstrap configuration.
func ProvideDataConfig(bc *configpb.Bootstrap) *configpb.Data {
	if bc == nil {
		return nil
	}
	return bc.GetData()
}

// ProvideObservabilityConfig exposes the normalized observability configuration.
func ProvideObservabilityConfig(l *Loader) obswire.ObservabilityConfig {
	if l == nil {
		return obswire.ObservabilityConfig{}
	}
	return l.ObsConfig
}
