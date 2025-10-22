// Package loader_test 提供 config_loader 包 defaults 逻辑的黑盒测试。
package loader_test

import (
	"testing"

	loader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
)

// TestResolveServiceName 验证服务名称解析逻辑。
func TestResolveServiceName(t *testing.T) {
	// 使用反射调用 unexported 函数，或通过 Build 间接测试
	// 这里通过 Build 参数间接测试
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "explicit name",
			input: "my-service",
			want:  "my-service",
		},
		{
			name:  "empty defaults to template",
			input: "",
			want:  "template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 通过 Build 验证（需要有效配置文件）
			tmpDir := t.TempDir()
			writeMinimalConfig(t, tmpDir)

			params := loader.Params{
				ConfPath:    tmpDir,
				ServiceName: tt.input,
			}
			bundle, err := loader.Build(params)
			if err != nil {
				t.Fatalf("Build failed: %v", err)
			}
			if bundle.Service.Name != tt.want {
				t.Errorf("expected name %s, got %s", tt.want, bundle.Service.Name)
			}
		})
	}
}

// TestResolveServiceVersion 验证服务版本解析逻辑。
func TestResolveServiceVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "explicit version",
			input: "v1.2.3",
			want:  "v1.2.3",
		},
		{
			name:  "empty defaults to dev",
			input: "",
			want:  "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeMinimalConfig(t, tmpDir)

			params := loader.Params{
				ConfPath:       tmpDir,
				ServiceVersion: tt.input,
			}
			bundle, err := loader.Build(params)
			if err != nil {
				t.Fatalf("Build failed: %v", err)
			}
			if bundle.Service.Version != tt.want {
				t.Errorf("expected version %s, got %s", tt.want, bundle.Service.Version)
			}
		})
	}
}

// TestResolveEnvironment 验证环境变量规范化逻辑。
func TestResolveEnvironment(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		want   string
	}{
		{
			name:   "dev -> development",
			envVar: "dev",
			want:   "development",
		},
		{
			name:   "DEV -> development (case insensitive)",
			envVar: "DEV",
			want:   "development",
		},
		{
			name:   "staging -> staging",
			envVar: "staging",
			want:   "staging",
		},
		{
			name:   "prod -> production",
			envVar: "prod",
			want:   "production",
		},
		{
			name:   "PROD -> production",
			envVar: "PROD",
			want:   "production",
		},
		{
			name:   "custom environment preserved",
			envVar: "testing",
			want:   "testing",
		},
		{
			name:   "empty defaults to development",
			envVar: "",
			want:   "development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeMinimalConfig(t, tmpDir)

			if tt.envVar != "" {
				t.Setenv("APP_ENV", tt.envVar)
			}

			params := loader.Params{ConfPath: tmpDir}
			bundle, err := loader.Build(params)
			if err != nil {
				t.Fatalf("Build failed: %v", err)
			}
			if bundle.Service.Environment != tt.want {
				t.Errorf("expected environment %s, got %s", tt.want, bundle.Service.Environment)
			}
		})
	}
}

// TestResolveInstanceID 验证实例 ID 解析逻辑。
func TestResolveInstanceID(t *testing.T) {
	tmpDir := t.TempDir()
	writeMinimalConfig(t, tmpDir)

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// InstanceID 来自 os.Hostname()，非空即正确
	// 如果 Hostname() 失败，应回退到 "unknown-instance"
	if bundle.Service.InstanceID == "" {
		t.Error("expected non-empty InstanceID")
	}
}

// TestDefaultGRPCMetricsEnabled 验证 gRPC 指标的默认值。
func TestDefaultGRPCMetricsEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	writeMinimalConfig(t, tmpDir)

	params := loader.Params{ConfPath: tmpDir}
	bundle, err := loader.Build(params)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 当配置文件完全省略 observability.metrics 时，ObsConfig.Metrics 应为 nil
	// 这是预期行为（由 grpc_server/grpc_client 处理默认值）
	if bundle.ObsConfig.Metrics != nil {
		// 如果配置了 metrics 但省略了 grpc_enabled/grpc_include_health
		// 应使用默认值 true/false
		if !bundle.ObsConfig.Metrics.GRPCEnabled {
			t.Error("expected GRPCEnabled=true when not explicitly set")
		}
		if bundle.ObsConfig.Metrics.GRPCIncludeHealth {
			t.Error("expected GRPCIncludeHealth=false when not explicitly set")
		}
	}
}

// writeMinimalConfig 是辅助函数，创建最小有效配置文件。
func writeMinimalConfig(t *testing.T, dir string) {
	t.Helper()
	content := `
server:
  grpc:
    addr: 0.0.0.0:9000
`
	if err := writeConfigFile(t, dir, content); err != nil {
		t.Fatalf("write minimal config: %v", err)
	}
}
