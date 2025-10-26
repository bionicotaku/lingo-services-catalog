// Package grpcclient_test 提供 gRPC Client JWT 中间件集成测试。
package grpcclient_test

import (
	"context"
	"io"
	"testing"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	clientinfra "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/grpc_client"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	"golang.org/x/oauth2"
)

// TestJWTClientMiddleware_NilMiddleware 验证 nil middleware 不影响客户端连接。
func TestJWTClientMiddleware_NilMiddleware(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	// 配置无下游目标（target 为空）
	cfg := configloader.GRPCClientConfig{}

	// 传入 nil JWT middleware
	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != nil {
		t.Fatal("expected nil connection when target is empty")
	}
	if cleanup == nil {
		t.Fatal("cleanup should always be non-nil")
	}
	cleanup()
}

// TestJWTClientMiddleware_DisabledMode 验证 JWT disabled 模式的客户端连接。
func TestJWTClientMiddleware_DisabledMode(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)

	// 配置：JWT disabled
	jwtCfg := gcjwt.Config{
		Client: &gcjwt.ClientConfig{
			Audience: "https://downstream.run.app/",
			Disabled: true, // 禁用
		},
	}
	jwtComp, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer cleanup()

	clientMw, err := gcjwt.ProvideClientMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideClientMiddleware error: %v", err)
	}

	// disabled 模式下，middleware 应该为 no-op
	// 我们无法直接测试 no-op 行为，但可以验证它不会导致错误
	if clientMw == nil {
		t.Fatal("expected non-nil middleware even in disabled mode")
	}
}

// TestJWTClientMiddleware_EnabledMode 验证 JWT enabled 模式的中间件创建。
func TestJWTClientMiddleware_EnabledMode(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)

	// 配置 mock TokenSource
	mockTokenSource := &mockTokenSource{token: "fake-jwt-token"}
	gcjwt.SetTokenSourceFactory(func(_ context.Context, _ string) (oauth2.TokenSource, error) {
		return mockTokenSource, nil
	})
	t.Cleanup(func() { gcjwt.SetTokenSourceFactory(nil) })

	// 配置：JWT enabled
	jwtCfg := gcjwt.Config{
		Client: &gcjwt.ClientConfig{
			Audience: "https://downstream.run.app/",
			Disabled: false,
		},
	}
	jwtComp, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer cleanup()

	clientMw, err := gcjwt.ProvideClientMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideClientMiddleware error: %v", err)
	}

	if clientMw == nil {
		t.Fatal("expected non-nil middleware in enabled mode")
	}
}

// TestJWTClientMiddleware_NoJWTConfig 验证未配置 JWT 时返回 nil middleware。
func TestJWTClientMiddleware_NoJWTConfig(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)

	// 配置：无 JWT 配置
	jwtCfg := gcjwt.Config{
		Client: nil, // 无客户端配置
	}
	jwtComp, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer cleanup()

	clientMw, err := gcjwt.ProvideClientMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideClientMiddleware error: %v", err)
	}

	// 无配置时应返回 nil middleware
	if clientMw != nil {
		t.Fatalf("expected nil middleware when no JWT config, got %T", clientMw)
	}
}

// TestJWTClientMiddleware_CustomHeaderKey 验证自定义 header key 配置。
func TestJWTClientMiddleware_CustomHeaderKey(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)

	// 配置 mock TokenSource
	mockTokenSource := &mockTokenSource{token: "fake-jwt-token"}
	gcjwt.SetTokenSourceFactory(func(_ context.Context, _ string) (oauth2.TokenSource, error) {
		return mockTokenSource, nil
	})
	t.Cleanup(func() { gcjwt.SetTokenSourceFactory(nil) })

	// 配置：自定义 header key
	jwtCfg := gcjwt.Config{
		Client: &gcjwt.ClientConfig{
			Audience:  "https://downstream.run.app/",
			Disabled:  false,
			HeaderKey: "x-custom-auth", // 自定义 header
		},
	}
	jwtComp, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer cleanup()

	clientMw, err := gcjwt.ProvideClientMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideClientMiddleware error: %v", err)
	}

	if clientMw == nil {
		t.Fatal("expected non-nil middleware with custom header key")
	}
}

// TestJWTClientIntegration_WithNilMiddleware 验证 NewGRPCClient 对 nil middleware 的兼容性。
func TestJWTClientIntegration_WithNilMiddleware(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	// 配置无下游目标
	cfg := configloader.GRPCClientConfig{}

	// 传入 nil JWT middleware
	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != nil {
		t.Fatal("expected nil connection when no target")
	}
	if cleanup == nil {
		t.Fatal("cleanup should always be non-nil")
	}
	cleanup()
}

// TestJWTClientIntegration_WithJWTMiddleware 验证 NewGRPCClient 正确接收 JWT middleware。
func TestJWTClientIntegration_WithJWTMiddleware(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	// 配置 mock TokenSource
	mockTokenSource := &mockTokenSource{token: "fake-jwt-token"}
	gcjwt.SetTokenSourceFactory(func(_ context.Context, _ string) (oauth2.TokenSource, error) {
		return mockTokenSource, nil
	})
	t.Cleanup(func() { gcjwt.SetTokenSourceFactory(nil) })

	// 配置 JWT middleware
	jwtCfg := gcjwt.Config{
		Client: &gcjwt.ClientConfig{
			Audience: "https://downstream.run.app/",
			Disabled: false,
		},
	}
	jwtComp, jwtCleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer jwtCleanup()

	clientMw, err := gcjwt.ProvideClientMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideClientMiddleware error: %v", err)
	}

	// 配置无下游目标（仅测试中间件注入不会导致错误）
	cfg := configloader.GRPCClientConfig{}

	// 传入 JWT middleware
	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, clientMw, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != nil {
		t.Fatal("expected nil connection when no target")
	}
	cleanup()
}

// mockTokenSource 实现 oauth2.TokenSource 接口用于测试。
type mockTokenSource struct {
	token string
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: m.token}, nil
}
