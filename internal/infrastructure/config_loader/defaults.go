package loader

const (
	// defaultConfPath is the fallback configuration directory when no overrides are provided.
	defaultConfPath = "configs"
	// envConfPath is the env var name that overrides configuration directory when flag is absent.
	envConfPath = "CONF_PATH"
	// defaultEnvironment is used when APP_ENV is missing.
	defaultEnvironment = "development"
	// defaultGRPCMetricsEnabled toggles otelgrpc instrumentation when config omits explicit values.
	defaultGRPCMetricsEnabled = true
	// defaultGRPCIncludeHealth controls whether health check RPCs are exported by default.
	defaultGRPCIncludeHealth = false
)
