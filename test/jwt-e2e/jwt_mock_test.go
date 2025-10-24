// Package e2e 提供端到端集成测试。
package e2e

import (
	"context"
	"io"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	clientinfra "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_client"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"
	"github.com/bionicotaku/kratos-template/test/jwt-e2e/testutils"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// TestE2E_JWT_MockToken_SkipValidate 测试完整的 Client → Server JWT 流程（Mock Token + Skip Validate）。
//
// 测试场景：
// 1. Client 使用 Mock TokenSource 注入假 Token
// 2. Server 配置 skip_validate=true，提取但不验证 Token
// 3. 验证 Token 正确传递和提取
func TestE2E_JWT_MockToken_SkipValidate(t *testing.T) {
	const testAudience = "https://test-service.run.app/"
	logger := log.NewStdLogger(io.Discard)

	// === 1. 启动 Server（配置 JWT Server middleware）===
	serverJWTCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: testAudience,
			SkipValidate:     true, // 跳过验证，仅提取 Claims
			Required:         true, // Token 必需
			HeaderKey:        "",   // 使用默认 "authorization"
		},
	}
	serverJWTComp, serverCleanup, err := gcjwt.NewComponent(serverJWTCfg, logger)
	if err != nil {
		t.Fatalf("server NewComponent: %v", err)
	}
	defer serverCleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(serverJWTComp)
	if err != nil {
		t.Fatalf("ProvideServerMiddleware: %v", err)
	}

	// 创建 Video 服务
	videoRepo := &mockVideoRepo{}
	videoUC := services.NewVideoUsecase(videoRepo, logger)
	videoHandler := controllers.NewVideoHandler(videoUC)

	// 启动 gRPC Server
	serverCfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: false}
	srv := grpcserver.NewGRPCServer(serverCfg, metricsCfg, serverMw, videoHandler, logger)

	endpointURL, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("srv.Endpoint: %v", err)
	}
	serverAddr := endpointURL.Host

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("server exited: %v", err)
		}
	}()

	waitForServer(t, serverAddr)
	defer func() {
		cancel()
		_ = srv.Stop(context.Background())
	}()

	// === 2. 配置 Client（生成自签名 JWT Token）===
	// 生成符合 Cloud Run 格式的 JWT Token
	testEmail := "test-service@test-project.iam.gserviceaccount.com"
	mockToken := testutils.GenerateValidCloudRunToken(t, testAudience, testEmail)

	gcjwt.SetTokenSourceFactory(func(_ context.Context, _ string) (oauth2.TokenSource, error) {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: mockToken}), nil
	})
	t.Cleanup(func() { gcjwt.SetTokenSourceFactory(nil) })

	clientJWTCfg := gcjwt.Config{
		Client: &gcjwt.ClientConfig{
			Audience: testAudience,
			Disabled: false,
		},
	}
	clientJWTComp, clientCleanup, err := gcjwt.NewComponent(clientJWTCfg, logger)
	if err != nil {
		t.Fatalf("client NewComponent: %v", err)
	}
	defer clientCleanup()

	clientMw, err := gcjwt.ProvideClientMiddleware(clientJWTComp)
	if err != nil {
		t.Fatalf("ProvideClientMiddleware: %v", err)
	}

	// === 3. 创建 gRPC Client 连接 ===
	clientCfg := &configpb.Data{
		GrpcClient: &configpb.Data_Client{
			Target: serverAddr,
		},
	}
	conn, connCleanup, err := clientinfra.NewGRPCClient(clientCfg, metricsCfg, clientMw, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	defer connCleanup()

	// === 4. 发起 RPC 调用 ===
	client := videov1.NewVideoQueryServiceClient(conn)
	testVideoID := uuid.New().String()

	// 调用 GetVideoDetail（预期返回 NotFound，因为 mock repo 总是返回 ErrVideoNotFound）
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{
		VideoId: testVideoID,
	})

	// === 5. 验证结果 ===
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}

	// 预期：Token 正确传递，Server 验证通过，业务逻辑返回 NotFound
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound (authenticated), got %v: %v", status.Code(err), err)
	}

	// 成功！这证明：
	// 1. Client middleware 成功注入 Token
	// 2. Server middleware 成功提取并验证（skip_validate 模式）
	// 3. 请求到达业务逻辑层
}

// TestE2E_JWT_NoToken_Required 测试 Client 未启用 JWT 时，Server required=true 拒绝请求。
func TestE2E_JWT_NoToken_Required(t *testing.T) {
	const testAudience = "https://test-service.run.app/"
	logger := log.NewStdLogger(io.Discard)

	// === 1. 启动 Server（required=true）===
	serverJWTCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: testAudience,
			SkipValidate:     false,
			Required:         true, // Token 必需
		},
	}
	serverJWTComp, serverCleanup, err := gcjwt.NewComponent(serverJWTCfg, logger)
	if err != nil {
		t.Fatalf("server NewComponent: %v", err)
	}
	defer serverCleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(serverJWTComp)
	if err != nil {
		t.Fatalf("ProvideServerMiddleware: %v", err)
	}

	videoRepo := &mockVideoRepo{}
	videoUC := services.NewVideoUsecase(videoRepo, logger)
	videoHandler := controllers.NewVideoHandler(videoUC)

	serverCfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: false}
	srv := grpcserver.NewGRPCServer(serverCfg, metricsCfg, serverMw, videoHandler, logger)

	endpointURL, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("srv.Endpoint: %v", err)
	}
	serverAddr := endpointURL.Host

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("server exited: %v", err)
		}
	}()

	waitForServer(t, serverAddr)
	defer func() {
		cancel()
		_ = srv.Stop(context.Background())
	}()

	// === 2. 创建 Client（不启用 JWT）===
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// === 3. 发起 RPC 调用（无 Token）===
	client := videov1.NewVideoQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{
		VideoId: uuid.New().String(),
	})

	// === 4. 验证结果 ===
	if err == nil {
		t.Fatal("expected Unauthenticated error")
	}

	// 预期：Server 拒绝无 Token 请求
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v: %v", status.Code(err), err)
	}
}

// TestE2E_JWT_NoToken_Optional 测试 Client 未启用 JWT 时，Server required=false 允许请求。
func TestE2E_JWT_NoToken_Optional(t *testing.T) {
	const testAudience = "https://test-service.run.app/"
	logger := log.NewStdLogger(io.Discard)

	// === 1. 启动 Server（required=false）===
	serverJWTCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: testAudience,
			SkipValidate:     false,
			Required:         false, // Token 可选
		},
	}
	serverJWTComp, serverCleanup, err := gcjwt.NewComponent(serverJWTCfg, logger)
	if err != nil {
		t.Fatalf("server NewComponent: %v", err)
	}
	defer serverCleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(serverJWTComp)
	if err != nil {
		t.Fatalf("ProvideServerMiddleware: %v", err)
	}

	videoRepo := &mockVideoRepo{}
	videoUC := services.NewVideoUsecase(videoRepo, logger)
	videoHandler := controllers.NewVideoHandler(videoUC)

	serverCfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: false}
	srv := grpcserver.NewGRPCServer(serverCfg, metricsCfg, serverMw, videoHandler, logger)

	endpointURL, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("srv.Endpoint: %v", err)
	}
	serverAddr := endpointURL.Host

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("server exited: %v", err)
		}
	}()

	waitForServer(t, serverAddr)
	defer func() {
		cancel()
		_ = srv.Stop(context.Background())
	}()

	// === 2. 创建 Client（不启用 JWT）===
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// === 3. 发起 RPC 调用（无 Token）===
	client := videov1.NewVideoQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{
		VideoId: uuid.New().String(),
	})

	// === 4. 验证结果 ===
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}

	// 预期：Server 允许无 Token 请求通过，返回业务错误
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound (no auth required), got %v: %v", status.Code(err), err)
	}
}

// mockVideoRepo 实现 services.VideoRepo 接口用于测试。
type mockVideoRepo struct{}

func (m *mockVideoRepo) Create(_ context.Context, _ repositories.CreateVideoInput) (*po.Video, error) {
	return nil, repositories.ErrVideoNotFound
}

func (m *mockVideoRepo) FindByID(_ context.Context, _ uuid.UUID) (*po.VideoReadyView, error) {
	return nil, repositories.ErrVideoNotFound
}

// waitForServer 等待 gRPC Server 启动。
func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not start listening on %s", addr)
}
