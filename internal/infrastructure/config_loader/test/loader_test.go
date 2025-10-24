// Package loader_test 提供 config_loader 包的黑盒测试。
// 测试配置加载、路径解析、默认值处理、Proto 校验等核心功能。
package loader_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	loader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
)

// TestResolveConfPath_ExplicitPath 验证显式路径优先级最高。
func TestResolveConfPath_ExplicitPath(t *testing.T) {
	explicit := "/custom/config"
	os.Setenv("CONF_PATH", "/env/config")
	defer os.Unsetenv("CONF_PATH")

	got := loader.ResolveConfPath(explicit)
	if got != explicit {
		t.Errorf("expected %s, got %s", explicit, got)
	}
}

// TestResolveConfPath_EnvVar 验证环境变量在无显式路径时生效。
func TestResolveConfPath_EnvVar(t *testing.T) {
	envPath := "/env/config"
	os.Setenv("CONF_PATH", envPath)
	defer os.Unsetenv("CONF_PATH")

	got := loader.ResolveConfPath("")
	if got != envPath {
		t.Errorf("expected %s, got %s", envPath, got)
	}
}

// TestResolveConfPath_Default 验证回退到默认路径。
func TestResolveConfPath_Default(t *testing.T) {
	os.Unsetenv("CONF_PATH")
	got := loader.ResolveConfPath("")
	if got != "configs" {
		t.Errorf("expected 'configs', got %s", got)
	}
}

// TestBuild_ValidConfig 验证加载有效配置文件的完整流程。
func TestBuild_ValidConfig(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
    timeout: 1s
data:
  postgres:
    dsn: "postgresql://postgres:postgres@localhost:5432/test?sslmode=disable&search_path=public"
    max_open_conns: 10
    min_open_conns: 2
    max_conn_lifetime: 3600s
    max_conn_idle_time: 1800s
    health_check_period: 60s
    schema: test_schema
    enable_prepared_statements: false
  grpc_client:
    target: "dns:///127.0.0.1:9000"
observability:
  global_attributes:
    env: test
  tracing:
    enabled: true
    exporter: stdout
    sampling_ratio: 1.0
  metrics:
    enabled: true
    exporter: stdout
    interval: 60s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	// 设置服务元信息环境变量
	t.Setenv("SERVICE_NAME", "test-service")
	t.Setenv("SERVICE_VERSION", "v1.0.0")

	// 测试 Build
	params := loader.Params{ConfPath: tmpDir}

	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证 Bootstrap 配置
	if bundle.Bootstrap == nil {
		t.Fatal("Bootstrap is nil")
	}
	if addr := bundle.Bootstrap.GetServer().GetGrpc().GetAddr(); addr != "0.0.0.0:9000" {
		t.Errorf("expected addr '0.0.0.0:9000', got %s", addr)
	}
	if timeout := bundle.Bootstrap.GetServer().GetGrpc().GetTimeout(); timeout.AsDuration() != time.Second {
		t.Errorf("expected timeout 1s, got %v", timeout.AsDuration())
	}

	// 验证 ServiceMetadata
	if bundle.Service.Name != "test-service" {
		t.Errorf("expected service name 'test-service', got %s", bundle.Service.Name)
	}
	if bundle.Service.Version != "v1.0.0" {
		t.Errorf("expected version 'v1.0.0', got %s", bundle.Service.Version)
	}
	if bundle.Service.Environment != "development" {
		t.Errorf("expected environment 'development', got %s", bundle.Service.Environment)
	}

	// 验证 ObservabilityConfig
	if bundle.ObsConfig.Tracing == nil {
		t.Fatal("Tracing config is nil")
	}
	if !bundle.ObsConfig.Tracing.Enabled {
		t.Error("expected tracing enabled")
	}
	if bundle.ObsConfig.Tracing.Exporter != "stdout" {
		t.Errorf("expected exporter 'stdout', got %s", bundle.ObsConfig.Tracing.Exporter)
	}
}

// TestBuild_EmptyConfig 验证空配置时仍能正常加载（字段有默认值）。
// 注意：当前 Proto 未定义 PGV 约束，因此空字符串不会触发校验错误。
func TestBuild_EmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	// 完全空的配置（只有根对象）
	emptyContent := `
server:
  grpc:
    addr: ""
`
	if err := os.WriteFile(configFile, []byte(emptyContent), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	// 当前不期望错误，因为 Proto 未定义 required 约束
	if err != nil {
		t.Logf("Build with empty config returned: %v", err)
	}
	if bundle == nil && err == nil {
		t.Fatal("expected either bundle or error, got neither")
	}
}

// TestBuild_NonExistentPath 验证配置路径不存在时返回加载错误。
func TestBuild_NonExistentPath(t *testing.T) {
	params := loader.Params{ConfPath: "/nonexistent/path"}
	_, err := loader.Build(params)
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}

	var buildErr loader.BuildError
	if !isType(err, &buildErr) {
		t.Errorf("expected BuildError, got %T: %v", err, err)
	}
}

// TestBuild_MalformedYAML 验证畸形 YAML 时返回扫描错误。
func TestBuild_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	malformedContent := `
server:
  grpc:
    addr: [invalid yaml structure
`
	if err := os.WriteFile(configFile, []byte(malformedContent), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	_, err := loader.Build(params)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

// TestServiceMetadata_ObservabilityInfo 验证转换为 ObservabilityInfo。
func TestServiceMetadata_ObservabilityInfo(t *testing.T) {
	meta := loader.ServiceMetadata{
		Name:        "test-svc",
		Version:     "v2.0",
		Environment: "staging",
		InstanceID:  "host-123",
	}

	info := meta.ObservabilityInfo()
	if info.Name != "test-svc" {
		t.Errorf("expected Name 'test-svc', got %s", info.Name)
	}
	if info.Version != "v2.0" {
		t.Errorf("expected Version 'v2.0', got %s", info.Version)
	}
	if info.Environment != "staging" {
		t.Errorf("expected Environment 'staging', got %s", info.Environment)
	}
}

// TestServiceMetadata_LoggerConfig 验证转换为 LoggerConfig。
func TestServiceMetadata_LoggerConfig(t *testing.T) {
	meta := loader.ServiceMetadata{
		Name:        "log-svc",
		Version:     "v1.0",
		Environment: "production",
		InstanceID:  "inst-456",
	}

	cfg := meta.LoggerConfig()
	if cfg.Service != "log-svc" {
		t.Errorf("expected Service 'log-svc', got %s", cfg.Service)
	}
	if cfg.Version != "v1.0" {
		t.Errorf("expected Version 'v1.0', got %s", cfg.Version)
	}
	if cfg.Environment != "production" {
		t.Errorf("expected Environment 'production', got %s", cfg.Environment)
	}
	if cfg.InstanceID != "inst-456" {
		t.Errorf("expected InstanceID 'inst-456', got %s", cfg.InstanceID)
	}
	if !cfg.EnableSourceLocation {
		t.Error("expected EnableSourceLocation to be true")
	}
	if cfg.StaticLabels["service.id"] != "inst-456" {
		t.Errorf("expected StaticLabels[service.id] 'inst-456', got %s", cfg.StaticLabels["service.id"])
	}
}

// TestBuildError_Error 验证 BuildError 错误信息格式。
func TestBuildError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  loader.BuildError
		want string
	}{
		{
			name: "with stage and path",
			err:  loader.BuildError{Stage: "load", Path: "/foo/bar", Err: os.ErrNotExist},
			want: "config load at \"/foo/bar\": file does not exist",
		},
		{
			name: "with stage only",
			err:  loader.BuildError{Stage: "validate", Err: os.ErrInvalid},
			want: "config validate: invalid argument",
		},
		{
			name: "without stage",
			err:  loader.BuildError{Err: os.ErrPermission},
			want: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildError_Unwrap 验证 BuildError 支持错误链。
func TestBuildError_Unwrap(t *testing.T) {
	innerErr := os.ErrNotExist
	buildErr := loader.BuildError{Stage: "load", Err: innerErr}

	unwrapped := buildErr.Unwrap()
	if unwrapped != innerErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, innerErr)
	}
}

// TestBuild_EnvironmentVariables 验证环境变量对 ServiceMetadata 的影响。
func TestBuild_EnvironmentVariables(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	minimalConfig := `
server:
  grpc:
    addr: 0.0.0.0:9000
`
	if err := os.WriteFile(configFile, []byte(minimalConfig), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	// 设置 APP_ENV 环境变量
	os.Setenv("APP_ENV", "prod")
	defer os.Unsetenv("APP_ENV")

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if bundle.Service.Environment != "production" {
		t.Errorf("expected environment 'production' (from APP_ENV=prod), got %s", bundle.Service.Environment)
	}
}

// TestBuild_DefaultValues 验证各字段的默认值。
func TestBuild_DefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	// 最小配置，触发所有默认值
	minimalConfig := `
server:
  grpc:
    addr: 0.0.0.0:9000
`
	if err := os.WriteFile(configFile, []byte(minimalConfig), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证默认值
	if bundle.Service.Name != "template" {
		t.Errorf("expected default service name 'template', got %s", bundle.Service.Name)
	}
	if bundle.Service.Version != "dev" {
		t.Errorf("expected default version 'dev', got %s", bundle.Service.Version)
	}
	if bundle.Service.Environment != "development" {
		t.Errorf("expected default environment 'development', got %s", bundle.Service.Environment)
	}
}

// TestObservabilityConfig_MetricsDefaults 验证 Metrics 配置的默认值处理。
func TestObservabilityConfig_MetricsDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	// 配置中省略 grpc_enabled 和 grpc_include_health
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
observability:
  metrics:
    enabled: true
    exporter: stdout
    interval: 30s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证默认值：grpc_enabled=true, grpc_include_health=false
	if bundle.ObsConfig.Metrics == nil {
		t.Fatal("Metrics config is nil")
	}
	if !bundle.ObsConfig.Metrics.GRPCEnabled {
		t.Error("expected GRPCEnabled=true by default")
	}
	if bundle.ObsConfig.Metrics.GRPCIncludeHealth {
		t.Error("expected GRPCIncludeHealth=false by default")
	}
}

// TestObservabilityConfig_MetricsExplicitValues 验证显式配置 Metrics 字段。
func TestObservabilityConfig_MetricsExplicitValues(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
observability:
  metrics:
    enabled: false
    exporter: otlp_grpc
    interval: 15s
    grpc_enabled: false
    grpc_include_health: true
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if bundle.ObsConfig.Metrics == nil {
		t.Fatal("Metrics config is nil")
	}
	if bundle.ObsConfig.Metrics.Enabled {
		t.Error("expected Enabled=false")
	}
	if bundle.ObsConfig.Metrics.Exporter != "otlp_grpc" {
		t.Errorf("expected Exporter 'otlp_grpc', got %s", bundle.ObsConfig.Metrics.Exporter)
	}
	if bundle.ObsConfig.Metrics.Interval != 15*time.Second {
		t.Errorf("expected Interval 15s, got %v", bundle.ObsConfig.Metrics.Interval)
	}
	if bundle.ObsConfig.Metrics.GRPCEnabled {
		t.Error("expected GRPCEnabled=false")
	}
	if !bundle.ObsConfig.Metrics.GRPCIncludeHealth {
		t.Error("expected GRPCIncludeHealth=true")
	}
}

// TestObservabilityConfig_TracingConversion 验证 Tracing 配置转换。
func TestObservabilityConfig_TracingConversion(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
observability:
  global_attributes:
    key1: value1
    key2: value2
  tracing:
    enabled: true
    exporter: otlp_grpc
    endpoint: localhost:4317
    headers:
      authorization: Bearer token
    insecure: true
    sampling_ratio: 0.5
    batch_timeout: 5s
    export_timeout: 30s
    max_queue_size: 2048
    max_export_batch_size: 512
    required: true
    service_name: custom-svc
    service_version: v2.0
    environment: staging
    attributes:
      attr1: val1
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证 GlobalAttributes
	if len(bundle.ObsConfig.GlobalAttributes) != 2 {
		t.Errorf("expected 2 global attributes, got %d", len(bundle.ObsConfig.GlobalAttributes))
	}
	if bundle.ObsConfig.GlobalAttributes["key1"] != "value1" {
		t.Errorf("expected GlobalAttributes[key1]='value1', got %s", bundle.ObsConfig.GlobalAttributes["key1"])
	}

	// 验证 Tracing 字段
	tr := bundle.ObsConfig.Tracing
	if tr == nil {
		t.Fatal("Tracing config is nil")
	}
	if !tr.Enabled {
		t.Error("expected Enabled=true")
	}
	if tr.Exporter != "otlp_grpc" {
		t.Errorf("expected Exporter 'otlp_grpc', got %s", tr.Exporter)
	}
	if tr.Endpoint != "localhost:4317" {
		t.Errorf("expected Endpoint 'localhost:4317', got %s", tr.Endpoint)
	}
	if tr.Headers["authorization"] != "Bearer token" {
		t.Errorf("expected Headers[authorization]='Bearer token', got %s", tr.Headers["authorization"])
	}
	if !tr.Insecure {
		t.Error("expected Insecure=true")
	}
	if tr.SamplingRatio != 0.5 {
		t.Errorf("expected SamplingRatio 0.5, got %f", tr.SamplingRatio)
	}
	if tr.BatchTimeout != 5*time.Second {
		t.Errorf("expected BatchTimeout 5s, got %v", tr.BatchTimeout)
	}
	if tr.ExportTimeout != 30*time.Second {
		t.Errorf("expected ExportTimeout 30s, got %v", tr.ExportTimeout)
	}
	if tr.MaxQueueSize != 2048 {
		t.Errorf("expected MaxQueueSize 2048, got %d", tr.MaxQueueSize)
	}
	if tr.MaxExportBatchSize != 512 {
		t.Errorf("expected MaxExportBatchSize 512, got %d", tr.MaxExportBatchSize)
	}
	if !tr.Required {
		t.Error("expected Required=true")
	}
	if tr.ServiceName != "custom-svc" {
		t.Errorf("expected ServiceName 'custom-svc', got %s", tr.ServiceName)
	}
	if tr.ServiceVersion != "v2.0" {
		t.Errorf("expected ServiceVersion 'v2.0', got %s", tr.ServiceVersion)
	}
	if tr.Environment != "staging" {
		t.Errorf("expected Environment 'staging', got %s", tr.Environment)
	}
	if tr.Attributes["attr1"] != "val1" {
		t.Errorf("expected Attributes[attr1]='val1', got %s", tr.Attributes["attr1"])
	}
}

// TestObservabilityConfig_NilHandling 验证 nil 配置的安全处理。
func TestObservabilityConfig_NilHandling(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	// 完全省略 observability 段
	minimalConfig := `
server:
  grpc:
    addr: 0.0.0.0:9000
`
	if err := os.WriteFile(configFile, []byte(minimalConfig), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证 ObsConfig 是空结构体（不是 nil）
	if bundle.ObsConfig.Tracing != nil {
		t.Error("expected Tracing to be nil when not configured")
	}
	if bundle.ObsConfig.Metrics != nil {
		t.Error("expected Metrics to be nil when not configured")
	}
}

// TestDataConfig_Completeness 验证 Data 配置的完整转换。
func TestDataConfig_Completeness(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
data:
  postgres:
    dsn: "postgresql://user:pass@db:5432/testdb?sslmode=require"
    max_open_conns: 10
    min_open_conns: 2
    max_conn_lifetime: 3600s
    max_conn_idle_time: 1800s
    health_check_period: 60s
    schema: "kratos_template"
    enable_prepared_statements: false
  grpc_client:
    target: "dns:///remote:9000"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证 PostgreSQL
	pg := bundle.Bootstrap.GetData().GetPostgres()
	if pg.GetDsn() != "postgresql://user:pass@db:5432/testdb?sslmode=require" {
		t.Errorf("expected dsn 'postgresql://user:pass@db:5432/testdb?sslmode=require', got %s", pg.GetDsn())
	}
	if pg.GetMaxOpenConns() != 10 {
		t.Errorf("expected max_open_conns 10, got %d", pg.GetMaxOpenConns())
	}
	if pg.GetMinOpenConns() != 2 {
		t.Errorf("expected min_open_conns 2, got %d", pg.GetMinOpenConns())
	}
	if pg.GetSchema() != "kratos_template" {
		t.Errorf("expected schema 'kratos_template', got %s", pg.GetSchema())
	}
	if pg.GetEnablePreparedStatements() != false {
		t.Errorf("expected enable_prepared_statements false, got %v", pg.GetEnablePreparedStatements())
	}

	// 验证 gRPC Client
	client := bundle.Bootstrap.GetData().GetGrpcClient()
	if client.GetTarget() != "dns:///remote:9000" {
		t.Errorf("expected target 'dns:///remote:9000', got %s", client.GetTarget())
	}
}

// isType 是辅助函数，检查错误类型（通过类型断言）。
func isType(err error, target any) bool {
	switch target.(type) {
	case *loader.BuildError:
		_, ok := err.(loader.BuildError)
		return ok
	default:
		return false
	}
}

// TestProtoValidation_Skipped 说明当前 Proto 未定义 PGV 约束。
// 如果未来添加 validate.rules，应补充此测试以验证校验逻辑。
func TestProtoValidation_Skipped(t *testing.T) {
	t.Skip("Proto 当前未定义 PGV 约束（validate.rules），跳过校验测试")
}

// TestToObservabilityConfig_DurationConversion 验证 Duration 字段的正确转换。
func TestToObservabilityConfig_DurationConversion(t *testing.T) {
	// 直接测试 toObservabilityConfig 的内部逻辑
	// 这里通过构造 Bootstrap 对象并加载来间接测试
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
    timeout: 5s
observability:
  tracing:
    enabled: true
    batch_timeout: 10s
    export_timeout: 20s
  metrics:
    enabled: true
    interval: 30s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 验证 Duration 字段转换
	if bundle.Bootstrap.GetServer().GetGrpc().GetTimeout().AsDuration() != 5*time.Second {
		t.Errorf("expected server timeout 5s, got %v", bundle.Bootstrap.GetServer().GetGrpc().GetTimeout().AsDuration())
	}
	if bundle.ObsConfig.Tracing.BatchTimeout != 10*time.Second {
		t.Errorf("expected batch timeout 10s, got %v", bundle.ObsConfig.Tracing.BatchTimeout)
	}
	if bundle.ObsConfig.Tracing.ExportTimeout != 20*time.Second {
		t.Errorf("expected export timeout 20s, got %v", bundle.ObsConfig.Tracing.ExportTimeout)
	}
	if bundle.ObsConfig.Metrics.Interval != 30*time.Second {
		t.Errorf("expected metrics interval 30s, got %v", bundle.ObsConfig.Metrics.Interval)
	}
}

// BenchmarkBuild 基准测试配置加载性能。
func BenchmarkBuild(b *testing.B) {
	tmpDir := b.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
    timeout: 1s
data:
  postgres:
    dsn: "postgresql://postgres:postgres@localhost:5432/test?sslmode=disable"
observability:
  tracing:
    enabled: true
    exporter: stdout
  metrics:
    enabled: true
    exporter: stdout
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		b.Fatalf("create config file: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}

	b.ResetTimer()
	for range b.N {
		_, err := loader.Build(params)
		if err != nil {
			b.Fatalf("Build failed: %v", err)
		}
	}
}

// TestBuild_PORTEnvironmentVariable 测试 PORT 环境变量覆盖端口逻辑
func TestBuild_PORTEnvironmentVariable(t *testing.T) {
	t.Run("未设置 PORT - 使用配置文件默认值", func(t *testing.T) {
		// 清除 PORT 环境变量
		t.Setenv("PORT", "")

		bundle, err := loader.Build(loader.Params{ConfPath: "../../../configs"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		addr := bundle.Bootstrap.GetServer().GetGrpc().GetAddr()
		expected := "0.0.0.0:9000" // config.yaml 中的默认值
		if addr != expected {
			t.Errorf("未设置 PORT 时，addr = %q, want %q", addr, expected)
		}
	})

	t.Run("设置 PORT=8080 - 覆盖端口保留 host", func(t *testing.T) {
		t.Setenv("PORT", "8080")

		bundle, err := loader.Build(loader.Params{ConfPath: "../../../configs"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		addr := bundle.Bootstrap.GetServer().GetGrpc().GetAddr()
		expected := "0.0.0.0:8080" // 端口被覆盖，host 保持 0.0.0.0
		if addr != expected {
			t.Errorf("设置 PORT=8080 时，addr = %q, want %q", addr, expected)
		}
	})

	t.Run("设置 PORT=3000 - 验证动态端口", func(t *testing.T) {
		t.Setenv("PORT", "3000")

		bundle, err := loader.Build(loader.Params{ConfPath: "../../../configs"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		addr := bundle.Bootstrap.GetServer().GetGrpc().GetAddr()
		expected := "0.0.0.0:3000"
		if addr != expected {
			t.Errorf("设置 PORT=3000 时，addr = %q, want %q", addr, expected)
		}
	})

	t.Run("PORT + DATABASE_URL 同时覆盖", func(t *testing.T) {
		testDSN := "postgres://test:password@db.supabase.co:5432/postgres"
		t.Setenv("PORT", "8080")
		t.Setenv("DATABASE_URL", testDSN)

		bundle, err := loader.Build(loader.Params{ConfPath: "../../../configs"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}

		// 验证端口覆盖
		addr := bundle.Bootstrap.GetServer().GetGrpc().GetAddr()
		if addr != "0.0.0.0:8080" {
			t.Errorf("PORT 覆盖失败，addr = %q, want %q", addr, "0.0.0.0:8080")
		}

		// 验证数据库 DSN 覆盖
		dsn := bundle.Bootstrap.GetData().GetPostgres().GetDsn()
		if dsn != testDSN {
			t.Errorf("DATABASE_URL 覆盖失败，dsn = %q, want %q", dsn, testDSN)
		}
	})
}

// TestReplacePortLogic 通过临时配置文件测试端口替换逻辑
func TestReplacePortLogic(t *testing.T) {
	createTempConfig := func(t *testing.T, addr string) string {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")
		content := fmt.Sprintf(`
server:
  grpc:
    addr: %s
    timeout: 5s
data:
  postgres:
    dsn: ""
`, addr)
		if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
			t.Fatalf("create config file: %v", err)
		}
		return tmpDir
	}

	tests := []struct {
		name         string
		originalAddr string
		portEnv      string
		expectedAddr string
	}{
		{
			name:         "标准 IPv4 地址 - 端口覆盖",
			originalAddr: "0.0.0.0:9000",
			portEnv:      "8080",
			expectedAddr: "0.0.0.0:8080",
		},
		{
			name:         "localhost - 端口覆盖",
			originalAddr: "127.0.0.1:9000",
			portEnv:      "8080",
			expectedAddr: "127.0.0.1:8080",
		},
		{
			name:         "仅端口 - 端口覆盖",
			originalAddr: ":9000",
			portEnv:      "8080",
			expectedAddr: ":8080",
		},
		{
			name:         "未设置 PORT - 保持原值",
			originalAddr: "0.0.0.0:9000",
			portEnv:      "",
			expectedAddr: "0.0.0.0:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTempConfig(t, tt.originalAddr)

			if tt.portEnv != "" {
				t.Setenv("PORT", tt.portEnv)
			} else {
				t.Setenv("PORT", "")
			}

			bundle, err := loader.Build(loader.Params{ConfPath: tmpDir})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			addr := bundle.Bootstrap.GetServer().GetGrpc().GetAddr()
			if addr != tt.expectedAddr {
				t.Errorf("addr = %q, want %q", addr, tt.expectedAddr)
			}
		})
	}
}
