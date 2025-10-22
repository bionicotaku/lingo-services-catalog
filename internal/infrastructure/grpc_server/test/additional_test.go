package grpcserver_test

import (
	"context"
	"io"
	"testing"
	"time"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"

	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/durationpb"
)

// TestNewGRPCServer_WithNetwork 验证 network 配置。
func TestNewGRPCServer_WithNetwork(t *testing.T) {
	svc := newTestController(t)
	cfg := &configpb.Server{
		Grpc: &configpb.Server_GRPC{
			Network: "tcp",
			Addr:    "127.0.0.1:0",
		},
	}
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: false}

	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, svc, logger)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	// 启动服务器验证配置生效
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = srv.Start(ctx)
	}()

	endpoint, _ := srv.Endpoint()
	if endpoint == nil {
		t.Fatal("expected non-nil endpoint")
	}

	cancel()
	_ = srv.Stop(context.Background())
}

// TestNewGRPCServer_WithTimeout 验证 timeout 配置。
func TestNewGRPCServer_WithTimeout(t *testing.T) {
	svc := newTestController(t)
	cfg := &configpb.Server{
		Grpc: &configpb.Server_GRPC{
			Addr:    "127.0.0.1:0",
			Timeout: durationpb.New(5 * time.Second),
		},
	}
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, svc, logger)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	// 不需要启动即可验证构造成功
	_ = srv.Stop(context.Background())
}

// TestNewGRPCServer_MetricsDisabled 验证禁用 metrics 时服务器仍能正常构造。
func TestNewGRPCServer_MetricsDisabled(t *testing.T) {
	svc := newTestController(t)
	cfg := &configpb.Server{
		Grpc: &configpb.Server_GRPC{
			Addr: "127.0.0.1:0",
		},
	}
	logger := log.NewStdLogger(io.Discard)
	// 显式禁用 gRPC metrics
	metricsCfg := &observability.MetricsConfig{
		GRPCEnabled:       false,
		GRPCIncludeHealth: false,
	}

	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, svc, logger)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	// 验证服务器构造成功（不实际启动以避免超时）
	_ = srv.Stop(context.Background())
}

// TestNewGRPCServer_NilMetricsConfig 验证 nil metricsCfg 时使用默认值。
func TestNewGRPCServer_NilMetricsConfig(t *testing.T) {
	svc := newTestController(t)
	cfg := &configpb.Server{
		Grpc: &configpb.Server_GRPC{
			Addr: "127.0.0.1:0",
		},
	}
	logger := log.NewStdLogger(io.Discard)

	// 传入 nil metricsCfg，应使用默认值（metrics enabled）
	srv := grpcserver.NewGRPCServer(cfg, nil, svc, logger)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	// 验证服务器构造成功
	_ = srv.Stop(context.Background())
}

// TestNewGRPCServer_MetricsIncludeHealth 验证 GRPCIncludeHealth=true 时服务器构造成功。
func TestNewGRPCServer_MetricsIncludeHealth(t *testing.T) {
	svc := newTestController(t)
	cfg := &configpb.Server{
		Grpc: &configpb.Server_GRPC{
			Addr: "127.0.0.1:0",
		},
	}
	logger := log.NewStdLogger(io.Discard)
	// 启用健康检查指标采集
	metricsCfg := &observability.MetricsConfig{
		GRPCEnabled:       true,
		GRPCIncludeHealth: true,
	}

	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, svc, logger)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	// 验证服务器构造成功
	_ = srv.Stop(context.Background())
}
