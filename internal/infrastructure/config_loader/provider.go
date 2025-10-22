package loader

import (
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	"github.com/bionicotaku/lingo-utils/gclog"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	"github.com/google/wire"
)

// ProviderSet exposes configuration-derived dependencies for Wire graphs.
var ProviderSet = wire.NewSet(
	ProvideBundle,
	ProvideServiceMetadata,
	ProvideBootstrap,
	ProvideServerConfig,
	ProvideDataConfig,
	ProvideObservabilityConfig,
	ProvideObservabilityInfo,
	ProvideLoggerConfig,
)

// ProvideBundle constructs a Bundle from runtime parameters.
func ProvideBundle(p Params) (*Bundle, error) {
	return Build(p)
}

// ProvideServiceMetadata returns the resolved ServiceMetadata from the bundle.
func ProvideServiceMetadata(b *Bundle) ServiceMetadata {
	if b == nil {
		return ServiceMetadata{}
	}
	return b.Service
}

// ProvideBootstrap exposes the strongly typed bootstrap configuration.
func ProvideBootstrap(b *Bundle) *configpb.Bootstrap {
	if b == nil {
		return nil
	}
	return b.Bootstrap
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
func ProvideObservabilityConfig(b *Bundle) obswire.ObservabilityConfig {
	if b == nil {
		return obswire.ObservabilityConfig{}
	}
	return b.ObsConfig
}

// ProvideObservabilityInfo exposes service metadata to observability Provider.
func ProvideObservabilityInfo(meta ServiceMetadata) obswire.ServiceInfo {
	return meta.ObservabilityInfo()
}

// ProvideLoggerConfig exposes service metadata to logging Provider.
func ProvideLoggerConfig(meta ServiceMetadata) gclog.Config {
	return meta.LoggerConfig()
}
