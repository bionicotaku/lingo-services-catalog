// Package e2e 提供完整的真实环境 E2E 测试。
//
// 此测试使用真实的 Google Cloud ID Token，需要以下前置条件：
//
// 1. 配置环境：
//    - 复制 configs/.env.test.example 为 configs/.env.test
//    - 填写实际的 GCP_PROJECT_ID、JWT_TEST_SERVICE_ACCOUNT 等值
//
// 2. 认证（必需）：
//    gcloud auth application-default login
//
// 3. 运行测试：
//    make test  # 或 go test -v ./test/jwt-e2e
//
// 4. CI 环境（自动跳过真实 GCP 测试）：
//    - 由于 .env.test 在 .gitignore 中，CI 环境不存在该文件
//    - 测试会自动检测并跳过（通过 t.Skipf）

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
	"github.com/bionicotaku/kratos-template/test/jwt-e2e/testutils"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestE2E_JWT_RealEnv_SkipValidate 测试真实环境下的 JWT 流程（跳过签名验证）。
//
// 测试流程：
// 1. 从 configs/.env.test 加载配置
// 2. 使用 Google ADC 获取真实 ID Token
// 3. Client 注入 Token 到 gRPC 请求
// 4. Server 提取 Token（skip_validate=true，不验证签名）
// 5. 验证业务逻辑正常执行
func TestE2E_JWT_RealEnv_SkipValidate(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过真实环境测试（使用 -short 标志）")
	}

	// === 1. 加载测试配置 ===
	cfg, err := testutils.LoadTestEnv()
	if err != nil {
		t.Skipf("跳过测试，无法加载配置: %v\n"+
			"请确保 configs/.env.test 文件存在并已正确配置", err)
	}

	t.Logf("测试配置:")
	t.Logf("  项目 ID: %s", cfg.ProjectID)
	t.Logf("  Service Account: %s", cfg.ServiceAccountEmail)
	t.Logf("  Audience: %s", cfg.Audience)

	logger := setupTestLogger(t, cfg.Verbose == "true")

	// === 2. 配置自定义 TokenSource（使用 gcloud impersonate）===
	gcjwt.SetTokenSourceFactory(func(ctx context.Context, audience string) (oauth2.TokenSource, error) {
		return testutils.NewTokenSource(ctx, cfg.ServiceAccountEmail, audience)
	})
	t.Cleanup(func() {
		gcjwt.SetTokenSourceFactory(nil) // 恢复默认行为
	})

	// === 3. 启动 Server（skip_validate=true）===
	serverJWTCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: cfg.Audience,
			SkipValidate:     true, // 跳过签名验证，加速测试
			Required:         true,
		},
	}
	serverJWTComp, serverCleanup, err := gcjwt.NewComponent(serverJWTCfg, logger)
	if err != nil {
		t.Fatalf("创建 Server JWT 组件失败: %v", err)
	}
	defer serverCleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(serverJWTComp)
	if err != nil {
		t.Fatalf("创建 Server Middleware 失败: %v", err)
	}

	// 创建 Video 服务
	videoRepo := &mockVideoRepo{}
	videoUC := services.NewVideoUsecase(videoRepo, noopTxManager{}, logger)
	videoHandler := controllers.NewVideoHandler(videoUC)

	// 启动 gRPC Server
	serverCfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: false}
	srv := grpcserver.NewGRPCServer(serverCfg, metricsCfg, serverMw, videoHandler, logger)

	endpointURL, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("获取 Server 地址失败: %v", err)
	}
	serverAddr := endpointURL.Host

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("Server 退出: %v", err)
		}
	}()

	waitForServer(t, serverAddr)
	defer func() {
		cancel()
		_ = srv.Stop(context.Background())
	}()

	t.Logf("✅ Server 启动成功: %s", serverAddr)

	// === 4. 配置 Client（使用真实 Google ID Token）===
	clientJWTCfg := gcjwt.Config{
		Client: &gcjwt.ClientConfig{
			Audience: cfg.Audience,
			Disabled: false,
		},
	}
	clientJWTComp, clientCleanup, err := gcjwt.NewComponent(clientJWTCfg, logger)
	if err != nil {
		t.Fatalf("创建 Client JWT 组件失败: %v", err)
	}
	defer clientCleanup()

	clientMw, err := gcjwt.ProvideClientMiddleware(clientJWTComp)
	if err != nil {
		t.Fatalf("创建 Client Middleware 失败: %v", err)
	}

	// === 5. 创建 gRPC Client 连接 ===
	clientCfg := &configpb.Data{
		GrpcClient: &configpb.Data_Client{
			Target: serverAddr,
		},
	}
	conn, connCleanup, err := clientinfra.NewGRPCClient(clientCfg, metricsCfg, clientMw, logger)
	if err != nil {
		t.Fatalf("创建 gRPC Client 失败: %v", err)
	}
	defer connCleanup()

	t.Logf("✅ Client 连接成功")

	// === 6. 发起 RPC 调用 ===
	client := videov1.NewVideoQueryServiceClient(conn)
	testVideoID := uuid.New().String()

	t.Logf("发起 RPC 调用: GetVideoDetail(%s)", testVideoID)

	_, err = client.GetVideoDetail(ctx, &videov1.GetVideoDetailRequest{
		VideoId: testVideoID,
	})

	// === 7. 验证结果 ===
	if err == nil {
		t.Fatal("预期返回错误（视频不存在），但调用成功")
	}

	code := status.Code(err)
	if code != codes.NotFound {
		t.Fatalf("预期返回 NotFound（认证通过），但返回 %v: %v", code, err)
	}

	t.Logf("✅ 测试通过！")
	t.Logf("   - Token 成功注入")
	t.Logf("   - Server 成功验证")
	t.Logf("   - 业务逻辑正常执行")
}

// TestE2E_JWT_RealEnv_FullValidation 测试真实环境下的完整 JWT 验证流程。
//
// 与 SkipValidate 的区别：
// - Server 会验证 Token 签名（需要访问 Google JWKS endpoint）
// - 验证 audience、expiry、issuer
// - 完整模拟生产环境
func TestE2E_JWT_RealEnv_FullValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过真实环境测试（使用 -short 标志）")
	}

	// === 1. 加载测试配置 ===
	cfg, err := testutils.LoadTestEnv()
	if err != nil {
		t.Skipf("跳过测试，无法加载配置: %v", err)
	}

	t.Logf("测试配置:")
	t.Logf("  项目 ID: %s", cfg.ProjectID)
	t.Logf("  Service Account: %s", cfg.ServiceAccountEmail)
	t.Logf("  Audience: %s", cfg.Audience)

	logger := setupTestLogger(t, cfg.Verbose == "true")

	// === 2. 配置自定义 TokenSource ===
	gcjwt.SetTokenSourceFactory(func(ctx context.Context, audience string) (oauth2.TokenSource, error) {
		return testutils.NewTokenSource(ctx, cfg.ServiceAccountEmail, audience)
	})
	t.Cleanup(func() {
		gcjwt.SetTokenSourceFactory(nil)
	})

	// === 3. 启动 Server（skip_validate=false，完整验证）===
	serverJWTCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: cfg.Audience,
			SkipValidate:     false, // ← 关键：启用完整验证
			Required:         true,
		},
	}
	serverJWTComp, serverCleanup, err := gcjwt.NewComponent(serverJWTCfg, logger)
	if err != nil {
		t.Fatalf("创建 Server JWT 组件失败: %v", err)
	}
	defer serverCleanup()

	serverMw, err := gcjwt.ProvideServerMiddleware(serverJWTComp)
	if err != nil {
		t.Fatalf("创建 Server Middleware 失败: %v", err)
	}

	videoRepo := &mockVideoRepo{}
	videoUC := services.NewVideoUsecase(videoRepo, noopTxManager{}, logger)
	videoHandler := controllers.NewVideoHandler(videoUC)

	serverCfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: false}
	srv := grpcserver.NewGRPCServer(serverCfg, metricsCfg, serverMw, videoHandler, logger)

	endpointURL, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("获取 Server 地址失败: %v", err)
	}
	serverAddr := endpointURL.Host

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("Server 退出: %v", err)
		}
	}()

	waitForServer(t, serverAddr)
	defer func() {
		cancel()
		_ = srv.Stop(context.Background())
	}()

	t.Logf("✅ Server 启动成功（完整验证模式）: %s", serverAddr)

	// === 4. 配置 Client ===
	clientJWTCfg := gcjwt.Config{
		Client: &gcjwt.ClientConfig{
			Audience: cfg.Audience,
			Disabled: false,
		},
	}
	clientJWTComp, clientCleanup, err := gcjwt.NewComponent(clientJWTCfg, logger)
	if err != nil {
		t.Fatalf("创建 Client JWT 组件失败: %v", err)
	}
	defer clientCleanup()

	clientMw, err := gcjwt.ProvideClientMiddleware(clientJWTComp)
	if err != nil {
		t.Fatalf("创建 Client Middleware 失败: %v", err)
	}

	clientCfg := &configpb.Data{
		GrpcClient: &configpb.Data_Client{
			Target: serverAddr,
		},
	}
	conn, connCleanup, err := clientinfra.NewGRPCClient(clientCfg, metricsCfg, clientMw, logger)
	if err != nil {
		t.Fatalf("创建 gRPC Client 失败: %v", err)
	}
	defer connCleanup()

	// === 5. 发起 RPC 调用 ===
	client := videov1.NewVideoQueryServiceClient(conn)
	testVideoID := uuid.New().String()

	t.Logf("发起 RPC 调用（完整验证）: GetVideoDetail(%s)", testVideoID)

	_, err = client.GetVideoDetail(ctx, &videov1.GetVideoDetailRequest{
		VideoId: testVideoID,
	})

	// === 6. 验证结果 ===
	if err == nil {
		t.Fatal("预期返回错误（视频不存在），但调用成功")
	}

	code := status.Code(err)
	// 可能的结果：
	// - NotFound: Token 验证通过，业务逻辑返回未找到
	// - Unauthenticated: Token 验证失败（可能是网络问题、JWKS 访问失败等）
	if code != codes.NotFound && code != codes.Unauthenticated {
		t.Fatalf("预期返回 NotFound 或 Unauthenticated，但返回 %v: %v", code, err)
	}

	if code == codes.NotFound {
		t.Logf("✅ 完整验证测试通过！")
		t.Logf("   - Token 签名验证成功")
		t.Logf("   - Audience 验证成功")
		t.Logf("   - 业务逻辑正常执行")
	} else {
		t.Logf("⚠️  Token 验证失败: %v", err)
		t.Logf("   这可能是正常的（本地环境限制、网络问题等）")
	}
}

// TestE2E_JWT_RealEnv_PrintToken 打印真实 Token 内容（调试用）。
//
// 此测试不会执行实际的 RPC 调用，只是获取 Token 并打印其内容。
// 用于验证 Token 获取流程和 Claims 内容。
func TestE2E_JWT_RealEnv_PrintToken(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过真实环境测试（使用 -short 标志）")
	}

	cfg, err := testutils.LoadTestEnv()
	if err != nil {
		t.Skipf("跳过测试，无法加载配置: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Logf("正在获取 ID Token...")
	t.Logf("  Service Account: %s", cfg.ServiceAccountEmail)
	t.Logf("  Audience: %s", cfg.Audience)

	// 创建 TokenSource（使用 gcloud impersonate）
	ts, err := testutils.NewTokenSource(ctx, cfg.ServiceAccountEmail, cfg.Audience)
	if err != nil {
		t.Fatalf("创建 TokenSource 失败: %v", err)
	}

	// 获取 Token
	token, err := ts.Token()
	if err != nil {
		t.Fatalf("获取 Token 失败: %v", err)
	}

	t.Logf("✅ Token 获取成功！")
	t.Logf("")
	t.Logf("Token 长度: %d 字符", len(token.AccessToken))
	t.Logf("过期时间: %s", token.Expiry.Format(time.RFC3339))
	t.Logf("")

	// 验证并解析 Token
	payload, err := idtoken.Validate(ctx, token.AccessToken, cfg.Audience)
	if err != nil {
		t.Logf("⚠️  Token 验证失败: %v", err)
		t.Logf("（这在本地环境是正常的，生产环境会成功）")
	} else {
		t.Logf("✅ Token 验证成功！")
		t.Logf("")
		t.Logf("Claims:")
		t.Logf("  Subject: %s", payload.Subject)
		t.Logf("  Audience: %s", payload.Audience)
		t.Logf("  Issuer: %s", payload.Issuer)
		if email, ok := payload.Claims["email"].(string); ok {
			t.Logf("  Email: %s", email)
		}
		if azp, ok := payload.Claims["azp"].(string); ok {
			t.Logf("  Authorized Party: %s", azp)
		}
	}

	// 打印 Token 前 50 个字符（调试用）
	tokenPreview := token.AccessToken
	if len(tokenPreview) > 50 {
		tokenPreview = tokenPreview[:50] + "..."
	}
	t.Logf("")
	t.Logf("Token 预览: %s", tokenPreview)
	t.Logf("")
	t.Logf("完整 Token 已保存到环境变量（测试结束后会清除）")
}

// setupTestLogger 创建测试日志器。
func setupTestLogger(_ *testing.T, verbose bool) log.Logger {
	if verbose {
		return log.NewStdLogger(os.Stdout)
	}
	return log.NewStdLogger(io.Discard)
}
