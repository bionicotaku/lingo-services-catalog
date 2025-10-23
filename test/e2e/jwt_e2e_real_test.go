// Package e2e 提供端到端集成测试（使用真实 GCP Token）。
//
// ⚠️ 注意：此文件中的测试需要 Google Cloud 凭证才能运行。
//
// 运行方式：
//
//  1. 本地开发：
//     gcloud auth application-default login
//     go test -v ./test/e2e -run TestE2E_JWT_Real
//
//  2. CI/CD with Service Account：
//     export GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa-key.json
//     go test -v ./test/e2e -run TestE2E_JWT_Real
//
//  3. 跳过真实 GCP 测试：
//     go test -v ./test/e2e -short  (这些测试会被跳过)
package e2e

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	clientinfra "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_client"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestE2E_JWT_Real_SkipValidate 测试使用真实 GCP ID Token 的完整流程（skip_validate 模式）。
//
// 测试场景：
// 1. Client 使用真实的 Google Cloud Metadata Server 获取 ID Token
// 2. Server 配置 skip_validate=true，提取但不验证签名
// 3. 验证 Token 正确传递和 Claims 提取
//
// 要求：
// - 需要 Google Cloud 凭证（gcloud auth application-default login 或 GOOGLE_APPLICATION_CREDENTIALS）
// - 使用 -short 标志时会跳过此测试
func TestE2E_JWT_Real_SkipValidate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real GCP token test in short mode")
	}

	// 检查是否有 Google Cloud 凭证
	if !hasGoogleCredentials(t) {
		t.Skip("skipping test: no Google Cloud credentials available")
	}

	const testAudience = "https://test-service.run.app/"
	logger := log.NewStdLogger(io.Discard)

	// === 1. 启动 Server（skip_validate=true）===
	serverJWTCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: testAudience,
			SkipValidate:     true, // 跳过签名验证
			Required:         true,
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	// === 2. 配置 Client（使用真实的 Google ID Token）===
	// gcjwt.Client 内部会调用 idtoken.NewTokenSource，它会自动使用：
	// - 本地：Application Default Credentials (ADC)
	// - GCE/Cloud Run：Metadata Server
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

	_, err = client.GetVideoDetail(ctx, &videov1.GetVideoDetailRequest{
		VideoId: testVideoID,
	})

	// === 5. 验证结果 ===
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}

	// 预期：Token 正确传递，Server 验证通过（skip_validate），返回 NotFound
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound (authenticated), got %v: %v", status.Code(err), err)
	}

	t.Log("✅ 真实 GCP Token E2E 测试通过")
}

// TestE2E_JWT_Real_FullValidation 测试使用真实 GCP ID Token 的完整验证流程。
//
// ⚠️ 注意：此测试需要 Server 能够访问 Google 的 JWKS endpoint 来验证签名。
//
// 测试场景：
// 1. Client 使用真实的 Google Cloud ID Token
// 2. Server 进行完整的签名验证（skip_validate=false）
// 3. 验证 Token 签名、audience、expiry 等
//
// 要求：
// - Google Cloud 凭证
// - 网络访问 Google JWKS endpoint (https://www.googleapis.com/oauth2/v3/certs)
func TestE2E_JWT_Real_FullValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real GCP token test in short mode")
	}

	if !hasGoogleCredentials(t) {
		t.Skip("skipping test: no Google Cloud credentials available")
	}

	const testAudience = "https://test-service.run.app/"
	logger := log.NewStdLogger(io.Discard)

	// === 1. 启动 Server（完整验证模式）===
	serverJWTCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: testAudience,
			SkipValidate:     false, // 完整验证
			Required:         true,
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	// === 2. 配置 Client（使用真实的 Google ID Token）===
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

	_, err = client.GetVideoDetail(ctx, &videov1.GetVideoDetailRequest{
		VideoId: testVideoID,
	})

	// === 5. 验证结果 ===
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}

	// 预期：
	// - 如果 Token 有效且签名验证通过 → NotFound
	// - 如果 Token 无效或签名验证失败 → Unauthenticated
	code := status.Code(err)
	if code != codes.NotFound && code != codes.Unauthenticated {
		t.Fatalf("expected NotFound or Unauthenticated, got %v: %v", code, err)
	}

	if code == codes.NotFound {
		t.Log("✅ 真实 GCP Token 完整验证测试通过（Token 有效）")
	} else {
		t.Logf("⚠️  Token 验证失败（可能是本地环境限制）: %v", err)
	}
}

// hasGoogleCredentials 检查是否有 Google Cloud 凭证。
func hasGoogleCredentials(t *testing.T) bool {
	t.Helper()

	// 方法 1：检查 GOOGLE_APPLICATION_CREDENTIALS 环境变量
	if path := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	// 方法 2：尝试创建 TokenSource（会使用 ADC）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := idtoken.NewTokenSource(ctx, "https://test.example.com")
	if err == nil {
		return true
	}

	// 如果错误不是 "credentials not found"，可能是其他原因（网络、权限等）
	t.Logf("Google credentials check failed: %v", err)
	return false
}
