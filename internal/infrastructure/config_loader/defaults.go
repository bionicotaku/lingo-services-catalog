package loader

import (
	"strings"
)

const (
	// defaultConfPath is the fallback configuration directory when no overrides are provided.
	defaultConfPath = "configs"
	// envConfPath is the env var name that overrides configuration directory when flag is absent.
	envConfPath = "CONF_PATH"
	// defaultEnvironment is used when APP_ENV is missing.
	defaultEnvironment = "development"
	// defaultServiceName is applied when build inputs omit service name.
	defaultServiceName = "template"
	// defaultServiceVersion is applied when build inputs omit version.
	defaultServiceVersion = "dev"
	// defaultGRPCMetricsEnabled toggles otelgrpc instrumentation when config omits explicit values.
	defaultGRPCMetricsEnabled = true
	// defaultGRPCIncludeHealth controls whether health check RPCs are exported by default.
	defaultGRPCIncludeHealth = false
	// defaultInstanceID is used when os.Hostname returns empty.
	defaultInstanceID = "unknown-instance"
)

var canonicalEnvironment = map[string]string{
	"dev":     defaultEnvironment,
	"staging": "staging",
	"prod":    "production",
}

func resolveServiceName(name string) string {
	if name != "" {
		return name
	}
	return defaultServiceName
}

func resolveServiceVersion(version string) string {
	if version != "" {
		return version
	}
	return defaultServiceVersion
}

func resolveEnvironment(env string) string {
	if env == "" {
		return defaultEnvironment
	}
	key := strings.ToLower(env)
	if canonical, ok := canonicalEnvironment[key]; ok {
		return canonical
	}
	return key
}

func resolveInstanceID(host string) string {
	if host != "" {
		return host
	}
	return defaultInstanceID
}
