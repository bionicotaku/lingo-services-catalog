// Package loader provides configuration loading utilities for the template service.
package loader

import (
	"flag"
	"os"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	loginfra "github.com/bionicotaku/kratos-template/internal/infrastructure/logger"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
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
	return &Loader{Config: c, Bootstrap: bc, LoggerCfg: loggerCfg}, cleanup, nil
}
