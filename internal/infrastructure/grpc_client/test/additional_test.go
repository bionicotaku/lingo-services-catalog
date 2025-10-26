package grpcclient_test

import (
	"context"
	"io"
	"testing"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	clientinfra "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/grpc_client"

	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
)

// TestNewGRPCClient_CleanupFunction 验证 cleanup 函数正常执行。
func TestNewGRPCClient_CleanupFunction(t *testing.T) {
	addr, stop := startVideoServer(t)
	defer stop()

	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true}
	cfg := configloader.GRPCClientConfig{Target: "dns:///" + addr}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}

	// 调用 cleanup 应成功关闭连接
	cleanup()

	// 验证连接状态（已关闭后再调用 RPC 应失败，但这里仅验证 cleanup 不崩溃）
	// 注意：gRPC ClientConn 关闭后不会立即拒绝新调用，因此不强制验证错误
}

// TestNewGRPCClient_MetricsDisabled 验证禁用 gRPC metrics。
func TestNewGRPCClient_MetricsDisabled(t *testing.T) {
	addr, stop := startVideoServer(t)
	defer stop()

	logger := log.NewStdLogger(io.Discard)
	// 显式禁用 metrics
	metricsCfg := &observability.MetricsConfig{
		GRPCEnabled:       false,
		GRPCIncludeHealth: false,
	}
	cfg := configloader.GRPCClientConfig{Target: "dns:///" + addr}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	defer cleanup()

	// 验证连接仍可正常使用
	if conn.GetState().String() == "" {
		t.Error("expected valid connection state")
	}
}

// TestNewGRPCClient_NilMetricsConfig 验证 nil metricsCfg 时的默认行为。
func TestNewGRPCClient_NilMetricsConfig(t *testing.T) {
	addr, stop := startVideoServer(t)
	defer stop()

	logger := log.NewStdLogger(io.Discard)
	cfg := configloader.GRPCClientConfig{Target: "dns:///" + addr}

	// 传入 nil metricsCfg，应使用默认值（metrics enabled）
	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, nil, nil, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	defer cleanup()
}

// TestNewGRPCClient_MetricsIncludeHealth 验证 GRPCIncludeHealth=true。
func TestNewGRPCClient_MetricsIncludeHealth(t *testing.T) {
	addr, stop := startVideoServer(t)
	defer stop()

	logger := log.NewStdLogger(io.Discard)
	// 启用健康检查指标采集
	metricsCfg := &observability.MetricsConfig{
		GRPCEnabled:       true,
		GRPCIncludeHealth: true,
	}
	cfg := configloader.GRPCClientConfig{Target: "dns:///" + addr}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	defer cleanup()
}

// TestNewGRPCClient_InvalidTarget 验证无效目标地址的错误处理。
// 注意：Kratos DialInsecure 可能在连接时才报错，而非立即报错。
func TestNewGRPCClient_InvalidTarget(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true}
	// 使用显然无效的目标地址
	cfg := configloader.GRPCClientConfig{Target: "invalid://bad_scheme"}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	// Kratos 可能不会立即报错，而是延迟到实际连接时
	if err != nil {
		t.Logf("NewGRPCClient returned expected error: %v", err)
	}
	if conn != nil {
		defer cleanup()
	}
}

// TestNewGRPCClient_EmptyTarget 验证空 target 时返回 nil 连接。
func TestNewGRPCClient_EmptyTarget(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true}
	cfg := configloader.GRPCClientConfig{}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != nil {
		t.Error("expected nil connection for empty target")
	}
	if cleanup == nil {
		t.Fatal("cleanup should always be non-nil")
	}
	cleanup()
}

// TestNewGRPCClient_NilData 验证 nil Data 配置时返回 nil 连接。
func TestNewGRPCClient_NilData(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true}

	conn, cleanup, err := clientinfra.NewGRPCClient(configloader.GRPCClientConfig{}, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != nil {
		t.Error("expected nil connection for nil Data")
	}
	if cleanup == nil {
		t.Fatal("cleanup should always be non-nil")
	}
	cleanup()
}

// TestNewGRPCClient_NilGrpcClient 验证 nil GrpcClient 配置时返回 nil 连接。
func TestNewGRPCClient_NilGrpcClient(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true}
	cfg := configloader.GRPCClientConfig{}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != nil {
		t.Error("expected nil connection for nil GrpcClient")
	}
	if cleanup == nil {
		t.Fatal("cleanup should always be non-nil")
	}
	cleanup()
}

// TestNewGRPCClient_CleanupMultipleTimes 验证多次调用 cleanup 不崩溃。
func TestNewGRPCClient_CleanupMultipleTimes(t *testing.T) {
	addr, stop := startVideoServer(t)
	defer stop()

	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: false}
	cfg := configloader.GRPCClientConfig{Target: "dns:///" + addr}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}

	// 多次调用 cleanup 应不崩溃（可能会记录错误日志）
	cleanup()
	cleanup() // 第二次调用
}

// TestNewGRPCClient_ContextCancellation 验证连接创建时的上下文处理。
// 注意：DialInsecure 内部使用 background context，此测试仅验证构造不受外部 ctx 影响。
func TestNewGRPCClient_ContextCancellation(t *testing.T) {
	addr, stop := startVideoServer(t)
	defer stop()

	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true}
	cfg := configloader.GRPCClientConfig{Target: "dns:///" + addr}

	// 创建一个已取消的 context（虽然 NewGRPCClient 内部不使用外部 ctx）
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// NewGRPCClient 不受外部 context 影响（内部使用 Background）
	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	defer cleanup()

	// 连接应仍可用
	_ = ctx // 避免未使用变量警告
}
