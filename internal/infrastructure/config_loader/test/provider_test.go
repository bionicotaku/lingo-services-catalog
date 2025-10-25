// Package loader_test 提供 config_loader 包 provider 函数的黑盒测试。
package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	loader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
)

// TestProvideBundle 验证 ProvideBundle 函数。
func TestProvideBundle(t *testing.T) {
	tmpDir := t.TempDir()
	writeMinimalConfig(t, tmpDir)

	t.Setenv("SERVICE_NAME", "test-svc")
	t.Setenv("SERVICE_VERSION", "v1.0")

	params := loader.Params{ConfPath: tmpDir}

	bundle, err := loader.ProvideBundle(params)
	if err != nil {
		t.Fatalf("ProvideBundle failed: %v", err)
	}
	if bundle == nil {
		t.Fatal("expected non-nil bundle")
	}
	if bundle.Service.Name != "test-svc" {
		t.Errorf("expected service name 'test-svc', got %s", bundle.Service.Name)
	}
}

// TestProvideServiceMetadata 验证从 Bundle 提取 ServiceMetadata。
func TestProvideServiceMetadata(t *testing.T) {
	bundle := &loader.Bundle{
		Service: loader.ServiceMetadata{
			Name:        "meta-svc",
			Version:     "v2.0",
			Environment: "staging",
			InstanceID:  "inst-123",
		},
	}

	meta := loader.ProvideServiceMetadata(bundle)
	if meta.Name != "meta-svc" {
		t.Errorf("expected Name 'meta-svc', got %s", meta.Name)
	}
	if meta.Version != "v2.0" {
		t.Errorf("expected Version 'v2.0', got %s", meta.Version)
	}
	if meta.Environment != "staging" {
		t.Errorf("expected Environment 'staging', got %s", meta.Environment)
	}
	if meta.InstanceID != "inst-123" {
		t.Errorf("expected InstanceID 'inst-123', got %s", meta.InstanceID)
	}
}

// TestProvideServiceMetadata_Nil 验证 nil Bundle 的安全处理。
func TestProvideServiceMetadata_Nil(t *testing.T) {
	meta := loader.ProvideServiceMetadata(nil)
	// 应返回零值结构体
	if meta.Name != "" || meta.Version != "" || meta.Environment != "" || meta.InstanceID != "" {
		t.Error("expected zero-value ServiceMetadata for nil Bundle")
	}
}

// TestProvideBootstrap 验证从 Bundle 提取 Bootstrap 配置。
func TestProvideBootstrap(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:8080
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	bootstrap := loader.ProvideBootstrap(bundle)
	if bootstrap == nil {
		t.Fatal("expected non-nil Bootstrap")
	}
	if addr := bootstrap.GetServer().GetGrpc().GetAddr(); addr != "0.0.0.0:8080" {
		t.Errorf("expected addr '0.0.0.0:8080', got %s", addr)
	}
}

// TestProvideBootstrap_Nil 验证 nil Bundle 的安全处理。
func TestProvideBootstrap_Nil(t *testing.T) {
	bootstrap := loader.ProvideBootstrap(nil)
	if bootstrap != nil {
		t.Error("expected nil Bootstrap for nil Bundle")
	}
}

// TestProvideServerConfig 验证提取 Server 配置段。
func TestProvideServerConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
    timeout: 2s
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	bootstrap := loader.ProvideBootstrap(bundle)
	serverCfg := loader.ProvideServerConfig(bootstrap)
	if serverCfg == nil {
		t.Fatal("expected non-nil Server config")
	}
	if addr := serverCfg.GetGrpc().GetAddr(); addr != "0.0.0.0:9000" {
		t.Errorf("expected addr '0.0.0.0:9000', got %s", addr)
	}
}

// TestProvideServerConfig_Nil 验证 nil Bootstrap 的安全处理。
func TestProvideServerConfig_Nil(t *testing.T) {
	serverCfg := loader.ProvideServerConfig(nil)
	if serverCfg != nil {
		t.Error("expected nil Server config for nil Bootstrap")
	}
}

// TestProvideDataConfig 验证提取 Data 配置段。
func TestProvideDataConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PORT", "")
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
data:
  postgres:
    dsn: "postgresql://postgres:postgres@localhost:5432/test?sslmode=disable"
    max_open_conns: 10
    min_open_conns: 2
    schema: "kratos_template"
    prepared_statements_enabled: false
    pool_metrics_enabled: true
  grpc_client:
    target: "dns:///remote:9000"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	bootstrap := loader.ProvideBootstrap(bundle)
	dataCfg := loader.ProvideDataConfig(bootstrap)
	if dataCfg == nil {
		t.Fatal("expected non-nil Data config")
	}
	if dsn := dataCfg.GetPostgres().GetDsn(); dsn != "postgresql://postgres:postgres@localhost:5432/test?sslmode=disable" {
		t.Errorf("expected dsn 'postgresql://postgres:postgres@localhost:5432/test?sslmode=disable', got %s", dsn)
	}
	if target := dataCfg.GetGrpcClient().GetTarget(); target != "dns:///remote:9000" {
		t.Errorf("expected target 'dns:///remote:9000', got %s", target)
	}
}

// TestProvideDataConfig_Nil 验证 nil Bootstrap 的安全处理。
func TestProvideDataConfig_Nil(t *testing.T) {
	dataCfg := loader.ProvideDataConfig(nil)
	if dataCfg != nil {
		t.Error("expected nil Data config for nil Bootstrap")
	}
}

// TestProvideObservabilityConfig 验证提取 Observability 配置。
func TestProvideObservabilityConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
observability:
  tracing:
    enabled: true
    exporter: stdout
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	obsCfg := loader.ProvideObservabilityConfig(bundle)
	if obsCfg.Tracing == nil {
		t.Fatal("expected non-nil Tracing config")
	}
	if !obsCfg.Tracing.Enabled {
		t.Error("expected Tracing.Enabled=true")
	}
}

// TestProvideObservabilityConfig_Nil 验证 nil Bundle 的安全处理。
func TestProvideObservabilityConfig_Nil(t *testing.T) {
	obsCfg := loader.ProvideObservabilityConfig(nil)
	// 应返回零值结构体
	if obsCfg.Tracing != nil || obsCfg.Metrics != nil {
		t.Error("expected zero-value ObservabilityConfig for nil Bundle")
	}
}

// TestProvideObservabilityInfo 验证转换为 ObservabilityInfo。
func TestProvideObservabilityInfo(t *testing.T) {
	meta := loader.ServiceMetadata{
		Name:        "obs-svc",
		Version:     "v3.0",
		Environment: "production",
		InstanceID:  "inst-456",
	}

	info := loader.ProvideObservabilityInfo(meta)
	if info.Name != "obs-svc" {
		t.Errorf("expected Name 'obs-svc', got %s", info.Name)
	}
	if info.Version != "v3.0" {
		t.Errorf("expected Version 'v3.0', got %s", info.Version)
	}
	if info.Environment != "production" {
		t.Errorf("expected Environment 'production', got %s", info.Environment)
	}
}

// TestProvideLoggerConfig 验证转换为 LoggerConfig。
func TestProvideLoggerConfig(t *testing.T) {
	meta := loader.ServiceMetadata{
		Name:        "log-svc",
		Version:     "v1.5",
		Environment: "staging",
		InstanceID:  "inst-789",
	}

	cfg := loader.ProvideLoggerConfig(meta)
	if cfg.Service != "log-svc" {
		t.Errorf("expected Service 'log-svc', got %s", cfg.Service)
	}
	if cfg.Version != "v1.5" {
		t.Errorf("expected Version 'v1.5', got %s", cfg.Version)
	}
	if cfg.Environment != "staging" {
		t.Errorf("expected Environment 'staging', got %s", cfg.Environment)
	}
	if cfg.InstanceID != "inst-789" {
		t.Errorf("expected InstanceID 'inst-789', got %s", cfg.InstanceID)
	}
	if !cfg.EnableSourceLocation {
		t.Error("expected EnableSourceLocation=true")
	}
}

// TestProvideJWTConfig 验证 JWT 配置提取完整性。
func TestProvideJWTConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
  jwt:
    expected_audience: "https://my-service.run.app/"
    skip_validate: false
    required: true
    header_key: "x-cloud-run-jwt"
data:
  postgres:
    dsn: "postgresql://postgres:postgres@localhost:5432/test?sslmode=disable"
    max_open_conns: 1
    min_open_conns: 0
    schema: "test"
    prepared_statements_enabled: false
  grpc_client:
    target: "dns:///downstream:9000"
    jwt:
      audience: "https://downstream.run.app/"
      disabled: false
      header_key: "x-custom-jwt"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	bootstrap := loader.ProvideBootstrap(bundle)
	serverCfg := loader.ProvideServerConfig(bootstrap)
	dataCfg := loader.ProvideDataConfig(bootstrap)

	jwtCfg := loader.ProvideJWTConfig(serverCfg, dataCfg)

	// 验证 Server JWT 配置
	if jwtCfg.Server == nil {
		t.Fatal("expected non-nil Server JWT config")
	}
	if jwtCfg.Server.ExpectedAudience != "https://my-service.run.app/" {
		t.Errorf("expected audience 'https://my-service.run.app/', got %s", jwtCfg.Server.ExpectedAudience)
	}
	if jwtCfg.Server.SkipValidate {
		t.Error("expected SkipValidate=false")
	}
	if !jwtCfg.Server.Required {
		t.Error("expected Required=true")
	}
	if jwtCfg.Server.HeaderKey != "x-cloud-run-jwt" {
		t.Errorf("expected header_key 'x-cloud-run-jwt', got %s", jwtCfg.Server.HeaderKey)
	}

	// 验证 Client JWT 配置
	if jwtCfg.Client == nil {
		t.Fatal("expected non-nil Client JWT config")
	}
	if jwtCfg.Client.Audience != "https://downstream.run.app/" {
		t.Errorf("expected audience 'https://downstream.run.app/', got %s", jwtCfg.Client.Audience)
	}
	if jwtCfg.Client.Disabled {
		t.Error("expected Disabled=false")
	}
	if jwtCfg.Client.HeaderKey != "x-custom-jwt" {
		t.Errorf("expected header_key 'x-custom-jwt', got %s", jwtCfg.Client.HeaderKey)
	}
}

// TestProvideJWTConfig_ServerOnly 验证仅配置服务端的情况。
func TestProvideJWTConfig_ServerOnly(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
  jwt:
    expected_audience: "https://my-service.run.app/"
    skip_validate: false
    required: true
data:
  postgres:
    dsn: "postgresql://postgres:postgres@localhost:5432/test?sslmode=disable"
    max_open_conns: 1
    min_open_conns: 0
    schema: "test"
    prepared_statements_enabled: false
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	bootstrap := loader.ProvideBootstrap(bundle)
	serverCfg := loader.ProvideServerConfig(bootstrap)
	dataCfg := loader.ProvideDataConfig(bootstrap)

	jwtCfg := loader.ProvideJWTConfig(serverCfg, dataCfg)

	// 验证 Server JWT 配置存在
	if jwtCfg.Server == nil {
		t.Fatal("expected non-nil Server JWT config")
	}
	if jwtCfg.Server.ExpectedAudience != "https://my-service.run.app/" {
		t.Errorf("expected audience 'https://my-service.run.app/', got %s", jwtCfg.Server.ExpectedAudience)
	}

	// 验证 Client JWT 配置为 nil（未配置）
	if jwtCfg.Client != nil {
		t.Errorf("expected nil Client JWT config, got %+v", jwtCfg.Client)
	}
}

// TestProvideJWTConfig_ClientOnly 验证仅配置客户端的情况。
func TestProvideJWTConfig_ClientOnly(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
data:
  postgres:
    dsn: "postgresql://postgres:postgres@localhost:5432/test?sslmode=disable"
    max_open_conns: 1
    min_open_conns: 0
    schema: "test"
    prepared_statements_enabled: false
  grpc_client:
    target: "dns:///downstream:9000"
    jwt:
      audience: "https://downstream.run.app/"
      disabled: false
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	bootstrap := loader.ProvideBootstrap(bundle)
	serverCfg := loader.ProvideServerConfig(bootstrap)
	dataCfg := loader.ProvideDataConfig(bootstrap)

	jwtCfg := loader.ProvideJWTConfig(serverCfg, dataCfg)

	// 验证 Server JWT 配置为 nil（未配置）
	if jwtCfg.Server != nil {
		t.Errorf("expected nil Server JWT config, got %+v", jwtCfg.Server)
	}

	// 验证 Client JWT 配置存在
	if jwtCfg.Client == nil {
		t.Fatal("expected non-nil Client JWT config")
	}
	if jwtCfg.Client.Audience != "https://downstream.run.app/" {
		t.Errorf("expected audience 'https://downstream.run.app/', got %s", jwtCfg.Client.Audience)
	}
}

// TestProvideJWTConfig_NoJWT 验证未配置 JWT 时的默认行为。
func TestProvideJWTConfig_NoJWT(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  grpc:
    addr: 0.0.0.0:9000
data:
  postgres:
    dsn: "postgresql://postgres:postgres@localhost:5432/test?sslmode=disable"
    max_open_conns: 1
    min_open_conns: 0
    schema: "test"
    prepared_statements_enabled: false
`
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	bootstrap := loader.ProvideBootstrap(bundle)
	serverCfg := loader.ProvideServerConfig(bootstrap)
	dataCfg := loader.ProvideDataConfig(bootstrap)

	jwtCfg := loader.ProvideJWTConfig(serverCfg, dataCfg)

	// 验证返回零值 Config
	if jwtCfg.Server != nil {
		t.Errorf("expected nil Server JWT config, got %+v", jwtCfg.Server)
	}
	if jwtCfg.Client != nil {
		t.Errorf("expected nil Client JWT config, got %+v", jwtCfg.Client)
	}
	if !jwtCfg.IsZero() {
		t.Error("expected IsZero()=true for empty JWT config")
	}
}

// TestProvideJWTConfig_NilInputs 验证 nil 输入的安全处理。
func TestProvideJWTConfig_NilInputs(t *testing.T) {
	// nil server, nil data
	jwtCfg := loader.ProvideJWTConfig(nil, nil)
	if jwtCfg.Server != nil || jwtCfg.Client != nil {
		t.Error("expected zero-value Config for nil inputs")
	}

	// nil server, valid data (without JWT)
	dataCfg := loader.ProvideDataConfig(nil)
	jwtCfg = loader.ProvideJWTConfig(nil, dataCfg)
	if jwtCfg.Server != nil || jwtCfg.Client != nil {
		t.Error("expected zero-value Config when server is nil")
	}
}

// writeConfigFile 是辅助函数，创建配置文件。
func writeConfigFile(t *testing.T, dir, content string) error {
	t.Helper()
	configFile := filepath.Join(dir, "config.yaml")
	return os.WriteFile(configFile, []byte(content), 0o644)
}
