// Package grpcserver_test 提供 gRPC Server JWT 中间件集成测试。
package grpcserver_test

import (
	"context"
	"io"
	"net/url"
	"testing"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	kratosmd "github.com/go-kratos/kratos/v2/metadata"
	"github.com/google/uuid"
	stdgrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// TestJWTServerMiddleware_SkipValidate 验证 skip_validate 模式允许无 Token 请求。
func TestJWTServerMiddleware_SkipValidate(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	videoSvc := newVideoController(t)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	// 配置：skip_validate=true, required=false
	jwtCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: "https://my-service.run.app/",
			SkipValidate:     true,  // 跳过验证
			Required:         false, // Token 非必需
		},
	}
	jwtComp, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer cleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideServerMiddleware error: %v", err)
	}

	cfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, serverMw, videoSvc, logger)

	addr, stop := startKratosServer(t, srv)
	defer stop()

	// 发起无 Token 的请求
	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewVideoQueryServiceClient(conn)
	// 预期：skip_validate 模式下，无 Token 请求应该通过（返回 NotFound 而非 Unauthenticated）
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: uuid.New().String()})
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound (not Unauthenticated), got %v", status.Code(err))
	}
}

// TestJWTServerMiddleware_RequiredToken 验证 required=true 时拒绝无 Token 请求。
func TestJWTServerMiddleware_RequiredToken(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	videoSvc := newVideoController(t)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	// 配置：skip_validate=false, required=true
	jwtCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: "https://my-service.run.app/",
			SkipValidate:     false,
			Required:         true, // Token 必需
		},
	}
	jwtComp, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer cleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideServerMiddleware error: %v", err)
	}

	cfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, serverMw, videoSvc, logger)

	addr, stop := startKratosServer(t, srv)
	defer stop()

	// 发起无 Token 的请求
	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewVideoQueryServiceClient(conn)
	// 预期：required=true 时，无 Token 请求应该被拒绝（Unauthenticated）
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: uuid.New().String()})
	if err == nil {
		t.Fatal("expected Unauthenticated error")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
	}
}

// TestJWTServerMiddleware_OptionalToken 验证 required=false 时允许无 Token 请求。
func TestJWTServerMiddleware_OptionalToken(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	videoSvc := newVideoController(t)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	// 配置：skip_validate=false, required=false
	jwtCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: "https://my-service.run.app/",
			SkipValidate:     false,
			Required:         false, // Token 可选
		},
	}
	jwtComp, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer cleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideServerMiddleware error: %v", err)
	}

	cfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, serverMw, videoSvc, logger)

	addr, stop := startKratosServer(t, srv)
	defer stop()

	// 发起无 Token 的请求
	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewVideoQueryServiceClient(conn)
	// 预期：required=false 时，无 Token 请求应该通过（返回 NotFound）
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: uuid.New().String()})
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", status.Code(err))
	}
}

// TestJWTServerMiddleware_WithValidToken 验证携带有效 Token 的请求。
func TestJWTServerMiddleware_WithValidToken(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	videoSvc := newVideoController(t)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	const testAudience = "https://my-service.run.app/"

	// 配置服务端：skip_validate=true（测试环境），required=true
	jwtCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: testAudience,
			SkipValidate:     true, // 跳过签名验证（仅测试 Claims 提取）
			Required:         true,
		},
	}
	jwtComp, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent error: %v", err)
	}
	defer cleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(jwtComp)
	if err != nil {
		t.Fatalf("ProvideServerMiddleware error: %v", err)
	}

	cfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, serverMw, videoSvc, logger)

	addr, stop := startKratosServer(t, srv)
	defer stop()

	// 创建标准 gRPC 客户端连接
	conn, err := stdgrpc.NewClient(addr,
		stdgrpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// 手动设置 Token 到 metadata（模拟 JWT Client 中间件行为）
	md := kratosmd.New(map[string][]string{
		"authorization": {"Bearer fake-jwt-token"},
	})
	ctx := kratosmd.NewClientContext(context.Background(), md)

	client := videov1.NewVideoQueryServiceClient(conn)
	// 预期：携带 Token 的请求应该通过认证（返回 NotFound 而非 Unauthenticated）
	_, err = client.GetVideoDetail(ctx, &videov1.GetVideoDetailRequest{VideoId: uuid.New().String()})
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}
	// 注意：由于我们使用了 skip_validate=true，即使 Token 格式不正确也会通过
	// 只要 Token 存在且 required=true 的条件被满足
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v: %v", status.Code(err), err)
	}
}

// TestJWTServerMiddleware_NilMiddleware 验证 nil middleware 的兼容性。
func TestJWTServerMiddleware_NilMiddleware(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	videoSvc := newVideoController(t)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	cfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	// 传入 nil middleware
	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, nil, videoSvc, logger)

	addr, stop := startKratosServer(t, srv)
	defer stop()

	// 发起无 Token 的请求
	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewVideoQueryServiceClient(conn)
	// 预期：nil middleware 时，服务器应该正常工作（没有 JWT 验证）
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: uuid.New().String()})
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", status.Code(err))
	}
}

// startKratosServer 启动 Kratos gRPC Server 并返回地址和清理函数。
// 这个函数重用了现有测试中的 startServer 逻辑。
func startKratosServer(t *testing.T, srv interface {
	Start(context.Context) error
	Stop(context.Context) error
	Endpoint() (*url.URL, error)
}) (addr string, stop func()) {
	t.Helper()
	// Force endpoint initialization to retrieve the bound address.
	endpointURL, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	addr = endpointURL.Host

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("server start returned: %v", err)
		}
	}()

	waitForServing(t, addr)

	stop = func() {
		cancel()
		_ = srv.Stop(context.Background())
	}
	return addr, stop
}
