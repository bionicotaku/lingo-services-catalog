// Package loader provides configuration loading utilities for the template service.
package loader

import (
	"flag"
	"os"
	"time"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	loginfra "github.com/bionicotaku/kratos-template/internal/infrastructure/logger"

	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	// defaultConfPath is the fallback configuration directory when no overrides are provided.
	defaultConfPath = "../../configs"
	// envConfPath is the env var name that overrides configuration directory when flag is absent.
	envConfPath = "CONF_PATH"
)

// Loader bundles configuration objects used by the application.
type Loader struct {
	Config    config.Config
	Bootstrap configpb.Bootstrap
	LoggerCfg loginfra.Config
	ObsConfig observability.ObservabilityConfig
}

// ParseConfPath reads the configuration path from flags/environment, returning the resolved value.
// Priority: explicit flag override > CONF_PATH environment variable > default path.
func ParseConfPath(fs *flag.FlagSet, args []string) (string, error) {
	var confPath string
	fs.StringVar(&confPath, "conf", "", "config path, eg: -conf config.yaml")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if confPath != "" {
		return confPath, nil
	}
	if env := os.Getenv(envConfPath); env != "" {
		return env, nil
	}
	return defaultConfPath, nil
}

// LoadBootstrap loads bootstrap configuration from the provided path and derives the logger settings.
func LoadBootstrap(confPath, service, version string) (*Loader, func(), error) {
	// Build Kratos config loader backed by file source (supports YAML/TOML/JSON under the conf path).
	c := config.New(config.WithSource(file.NewSource(confPath)))
	if err := c.Load(); err != nil {
		return nil, func() {}, err
	}
	var bc configpb.Bootstrap
	if err := c.Scan(&bc); err != nil {
		c.Close()
		return nil, func() {}, err
	}
	cleanup := func() {
		_ = c.Close()
	}
	loggerCfg := loginfra.DefaultConfig(service, version)
	return &Loader{
		Config:    c,
		Bootstrap: bc,
		LoggerCfg: loggerCfg,
		ObsConfig: toObservabilityConfig(bc.Observability),
	}, cleanup, nil
}

func toObservabilityConfig(src *configpb.Observability) observability.ObservabilityConfig {
	if src == nil {
		return observability.ObservabilityConfig{}
	}
	cfg := observability.ObservabilityConfig{
		GlobalAttributes: cloneStringMap(src.GetGlobalAttributes()),
	}
	if tr := src.GetTracing(); tr != nil {
		cfg.Tracing = &observability.TracingConfig{
			Enabled:            tr.GetEnabled(),
			Exporter:           tr.GetExporter(),
			Endpoint:           tr.GetEndpoint(),
			Headers:            cloneStringMap(tr.GetHeaders()),
			Insecure:           tr.GetInsecure(),
			SamplingRatio:      tr.GetSamplingRatio(),
			BatchTimeout:       durationValue(tr.GetBatchTimeout()),
			ExportTimeout:      durationValue(tr.GetExportTimeout()),
			MaxQueueSize:       int(tr.GetMaxQueueSize()),
			MaxExportBatchSize: int(tr.GetMaxExportBatchSize()),
			Required:           tr.GetRequired(),
			ServiceName:        tr.GetServiceName(),
			ServiceVersion:     tr.GetServiceVersion(),
			Environment:        tr.GetEnvironment(),
			Attributes:         cloneStringMap(tr.GetAttributes()),
		}
	}
	if mt := src.GetMetrics(); mt != nil {
		cfg.Metrics = &observability.MetricsConfig{
			Enabled:             mt.GetEnabled(),
			Exporter:            mt.GetExporter(),
			Endpoint:            mt.GetEndpoint(),
			Headers:             cloneStringMap(mt.GetHeaders()),
			Insecure:            mt.GetInsecure(),
			Interval:            durationValue(mt.GetInterval()),
			DisableRuntimeStats: mt.GetDisableRuntimeStats(),
			Required:            mt.GetRequired(),
			ResourceAttributes:  cloneStringMap(mt.GetResourceAttributes()),
		}
	}
	return cfg
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func durationValue(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}
