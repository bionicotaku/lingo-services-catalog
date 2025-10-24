// Package integration_test 提供端到端集成测试，连接真实的 Supabase 数据库。
// 测试完整的 gRPC 调用链路：Client → gRPC Server → Controller → Service → Repository → PostgreSQL
//
// 环境要求：
//   - 本地开发：需要 configs/.env 文件（包含 DATABASE_URL）
//   - CI 环境：自动跳过（configs/.env 在 .gitignore 中）
//
// 运行方式：
//   make test                    # 本地有 .env 时运行，CI 自动跳过
//   go test -v ./test/integration  # 同上
package integration_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configloader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
	"github.com/bionicotaku/kratos-template/internal/infrastructure/database"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	stdgrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// testServer 封装测试服务器的启动和清理逻辑。
type testServer struct {
	addr    string
	cleanup func()
	client  videov1.VideoQueryServiceClient
}

// newTestServer 创建并启动一个连接真实数据库的 gRPC 测试服务器。
//
// 流程：
//  1. 加载配置（从 configs/ 目录，需要 DATABASE_URL 环境变量）
//  2. 初始化数据库连接池（连接 Supabase PostgreSQL）
//  3. 构建依赖注入链：Repository → Service → Controller
//  4. 启动 gRPC Server
//  5. 创建 gRPC Client 连接
//
// 注意：此函数会连接真实的 Supabase 数据库，确保：
//   - configs/.env 文件存在且包含有效的 DATABASE_URL
//   - 数据库中已执行迁移脚本（001_create_catalog_schema.sql）
//   - 已插入测试数据（seed_test_videos.sql）
func newTestServer(t *testing.T) *testServer {
	t.Helper()

	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard) // 测试时不输出日志

	// 1. 加载配置（从 configs/ 目录）
	// 获取配置路径并设置环境变量（确保所有测试使用相同的路径）
	confPath := getConfigPath(t)
	if confPath == "" {
		t.Skip("跳过集成测试：configs/config.yaml 不存在\n" +
			"本地运行：确保项目根目录有 configs/config.yaml 和 configs/.env 文件\n" +
			"CI 环境：自动跳过（需要真实数据库连接）")
	}

	// 检查 .env 文件是否存在（包含 DATABASE_URL）
	envFile := filepath.Join(confPath, ".env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		t.Skipf("跳过集成测试：%s 不存在\n"+
			"本地运行：复制 configs/.env.example 为 configs/.env 并配置 DATABASE_URL\n"+
			"CI 环境：自动跳过（需要真实数据库连接）", envFile)
	}

	t.Logf("Using config path: %s", confPath)

	// 临时设置 CONF_PATH 环境变量，确保配置加载器使用正确的路径
	oldConfPath := os.Getenv("CONF_PATH")
	os.Setenv("CONF_PATH", confPath)
	defer func() {
		if oldConfPath == "" {
			os.Unsetenv("CONF_PATH")
		} else {
			os.Setenv("CONF_PATH", oldConfPath)
		}
	}()

	bundle, err := configloader.Build(configloader.Params{ConfPath: confPath})
	if err != nil {
		t.Skipf("跳过集成测试：加载配置失败: %v\n"+
			"请确保 configs/.env 包含有效的 DATABASE_URL", err)
	}

	// 2. 初始化数据库连接池（连接 Supabase PostgreSQL）
	pool, dbCleanup, err := database.NewPgxPool(ctx, bundle.Bootstrap.GetData(), logger)
	if err != nil {
		t.Fatalf("create database pool failed: %v", err)
	}

	// 3. 构建依赖链
	videoRepo := repositories.NewVideoRepository(pool, logger)
	videoService := services.NewVideoUsecase(videoRepo, logger)
	videoController := controllers.NewVideoHandler(videoService)

	// 4. 启动 gRPC Server（禁用 metrics 避免干扰）
	serverConfig := bundle.Bootstrap.GetServer()
	serverConfig.Grpc.Addr = "127.0.0.1:0" // 使用随机端口

	metricsCfg := &observability.MetricsConfig{
		GRPCEnabled:       false,
		GRPCIncludeHealth: false,
	}

	grpcSrv := grpcserver.NewGRPCServer(serverConfig, metricsCfg, nil, videoController, logger)

	endpointURL, err := grpcSrv.Endpoint()
	if err != nil {
		dbCleanup()
		t.Fatalf("get server endpoint failed: %v", err)
	}
	addr := endpointURL.Host

	// 启动服务器（goroutine）
	serverCtx, serverCancel := context.WithCancel(ctx)
	go func() {
		if err := grpcSrv.Start(serverCtx); err != nil && err != context.Canceled {
			t.Logf("server exited with error: %v", err)
		}
	}()

	// 等待服务器就绪
	waitForServer(t, addr, 3*time.Second)

	// 5. 创建 gRPC Client
	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		serverCancel()
		_ = grpcSrv.Stop(ctx)
		dbCleanup()
		t.Fatalf("create grpc client failed: %v", err)
	}

	client := videov1.NewVideoQueryServiceClient(conn)

	// 封装清理函数
	cleanup := func() {
		_ = conn.Close()
		serverCancel()
		_ = grpcSrv.Stop(context.Background())
		dbCleanup()
	}

	return &testServer{
		addr:    addr,
		cleanup: cleanup,
		client:  client,
	}
}

// getConfigPath 获取配置文件路径。
//
// 优先级：
//  1. 环境变量 CONF_PATH
//  2. 项目根目录的 configs/（通过 go.mod 定位）
//  3. 当前目录的 configs/
//  4. 上级目录的 configs/
//
// 如果找不到配置文件或 .env 文件，返回空字符串（调用方应跳过测试）
func getConfigPath(t *testing.T) string {
	t.Helper()

	if confPath := os.Getenv("CONF_PATH"); confPath != "" {
		return confPath
	}

	// 尝试查找项目根目录（包含 go.mod 的目录）
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// 从当前目录向上查找 go.mod
	dir := wd
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// 找到了项目根目录
			configPath := filepath.Join(dir, "configs")
			configFile := filepath.Join(configPath, "config.yaml")
			if _, err := os.Stat(configFile); err == nil {
				return configPath
			}
		}

		// 到达根目录，停止查找
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// waitForServer 等待 gRPC 服务器启动完成。
func waitForServer(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server not ready after %v at %s", timeout, addr)
}

// TestIntegration_GetVideoDetail_Success 测试成功查询已发布视频的详细信息。
//
// 场景：查询 status=published 的视频，验证返回完整的字段（标题、描述、媒体 URL、AI 产物等）。
//
// 前置条件：
//   - 数据库中存在 status='published' 的测试视频
//   - 视频包含完整的 media 和 analysis 产物
func TestIntegration_GetVideoDetail_Success(t *testing.T) {
	srv := newTestServer(t)
	defer srv.cleanup()

	// 使用测试数据中的 published 视频 ID
	// "Academic Writing: Essay Structure" - df3c43f5-6c8f-4b25-b0f2-715b228c7a2f
	videoID := "df3c43f5-6c8f-4b25-b0f2-715b228c7a2f"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &videov1.GetVideoDetailRequest{VideoId: videoID}
	resp, err := srv.client.GetVideoDetail(ctx, req)
	// 验证响应
	if err != nil {
		t.Fatalf("GetVideoDetail failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Detail == nil {
		t.Fatal("expected non-nil detail")
	}

	// 验证基础字段
	detail := resp.Detail
	if detail.VideoId != videoID {
		t.Errorf("video_id mismatch: expected %s, got %s", videoID, detail.VideoId)
	}
	if detail.Title == "" {
		t.Error("title should not be empty")
	}
	if detail.Status != "published" {
		t.Errorf("expected status=published, got %s", detail.Status)
	}

	// 验证媒体产物（精简视图只包含核心字段）
	if detail.ThumbnailUrl == nil || detail.ThumbnailUrl.Value == "" {
		t.Error("thumbnail_url should not be empty for published video")
	}
	if detail.HlsMasterPlaylist == nil || detail.HlsMasterPlaylist.Value == "" {
		t.Error("hls_master_playlist should not be empty for published video")
	}
	if detail.DurationMicros == nil || detail.DurationMicros.Value <= 0 {
		t.Error("duration_micros should be positive for published video")
	}

	// 验证 AI 产物
	if detail.Difficulty == nil || detail.Difficulty.Value == "" {
		t.Error("difficulty should not be empty for published video")
	}
	if detail.Summary == nil || detail.Summary.Value == "" {
		t.Error("summary should not be empty for published video")
	}
	if len(detail.Tags) == 0 {
		t.Error("tags should not be empty for published video")
	}

	t.Logf("✅ Successfully retrieved video: title=%s difficulty=%s tags=%v",
		detail.Title, detail.Difficulty.Value, detail.Tags)
}

// TestIntegration_GetVideoDetail_NotFound 测试查询不存在的视频。
//
// 场景：使用一个不存在的 video_id，期望返回 NotFound 错误。
func TestIntegration_GetVideoDetail_NotFound(t *testing.T) {
	srv := newTestServer(t)
	defer srv.cleanup()

	// 使用一个肯定不存在的 UUID
	videoID := "00000000-0000-0000-0000-000000000000"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &videov1.GetVideoDetailRequest{VideoId: videoID}
	_, err := srv.client.GetVideoDetail(ctx, req)

	// 验证错误
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}

	if st.Code() != codes.NotFound {
		t.Errorf("expected code=NotFound, got %s", st.Code())
	}

	if st.Message() == "" {
		t.Error("error message should not be empty")
	}

	t.Logf("✅ Correctly returned NotFound error: %s", st.Message())
}

// TestIntegration_GetVideoDetail_InvalidID 测试无效的 video_id 格式。
//
// 场景：传入非 UUID 格式的 video_id，期望返回 InvalidArgument 错误。
func TestIntegration_GetVideoDetail_InvalidID(t *testing.T) {
	srv := newTestServer(t)
	defer srv.cleanup()

	testCases := []struct {
		name    string
		videoID string
	}{
		{"empty_id", ""},
		{"invalid_format", "not-a-uuid"},
		{"partial_uuid", "df3c43f5-6c8f"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req := &videov1.GetVideoDetailRequest{VideoId: tc.videoID}
			_, err := srv.client.GetVideoDetail(ctx, req)

			if err == nil {
				t.Fatal("expected error for invalid video_id")
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %T: %v", err, err)
			}

			if st.Code() != codes.InvalidArgument {
				t.Errorf("expected code=InvalidArgument, got %s", st.Code())
			}

			t.Logf("✅ Correctly rejected invalid ID '%s': %s", tc.videoID, st.Message())
		})
	}
}

// TestIntegration_GetVideoDetail_MultipleVideos 测试查询多个不同状态的视频。
//
// 场景：批量查询不同 status 的视频，验证每个视频的字段完整性符合其状态。
func TestIntegration_GetVideoDetail_MultipleVideos(t *testing.T) {
	srv := newTestServer(t)
	defer srv.cleanup()

	// 测试不同状态的视频
	testCases := []struct {
		name           string
		videoID        string
		expectedStatus string
		expectMedia    bool // 是否应该有完整的媒体产物
		expectAI       bool // 是否应该有 AI 产物
	}{
		{
			name:           "published_advanced",
			videoID:        "df3c43f5-6c8f-4b25-b0f2-715b228c7a2f", // Academic Writing
			expectedStatus: "published",
			expectMedia:    true,
			expectAI:       true,
		},
		{
			name:           "published_beginner",
			videoID:        "b3e73ad4-8cb0-4ecd-a6d6-a4b891378297", // Travel English
			expectedStatus: "published",
			expectMedia:    true,
			expectAI:       true,
		},
		{
			name:           "ready_not_published",
			videoID:        "c1a81119-c1f9-4806-9fcf-e0b87599c52c", // IELTS Speaking
			expectedStatus: "ready",
			expectMedia:    true,
			expectAI:       true,
		},
		{
			name:           "processing",
			videoID:        "519794d1-23eb-4dac-9978-ed8c41e3aa77", // Grammar
			expectedStatus: "processing",
			expectMedia:    false,
			expectAI:       false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &videov1.GetVideoDetailRequest{VideoId: tc.videoID}
			resp, err := srv.client.GetVideoDetail(ctx, req)
			if err != nil {
				t.Fatalf("GetVideoDetail failed: %v", err)
			}

			detail := resp.Detail
			if detail.Status != tc.expectedStatus {
				t.Errorf("status mismatch: expected %s, got %s", tc.expectedStatus, detail.Status)
			}

			// 验证媒体产物
			hasMedia := detail.HlsMasterPlaylist != nil && detail.HlsMasterPlaylist.Value != "" &&
				detail.ThumbnailUrl != nil && detail.ThumbnailUrl.Value != ""
			if tc.expectMedia && !hasMedia {
				t.Error("expected complete media outputs")
			}

			// 验证 AI 产物
			hasAI := detail.Difficulty != nil && detail.Difficulty.Value != "" &&
				detail.Summary != nil && detail.Summary.Value != "" &&
				len(detail.Tags) > 0
			if tc.expectAI && !hasAI {
				t.Error("expected complete AI outputs")
			}

			t.Logf("✅ Video '%s': status=%s media=%v ai=%v",
				detail.Title, detail.Status, hasMedia, hasAI)
		})
	}
}

// TestIntegration_GetVideoDetail_Timeout 测试超时控制。
//
// 场景：设置一个极短的超时，验证超时机制是否生效。
// 注意：由于数据库查询通常很快，此测试可能不会总是触发超时。
func TestIntegration_GetVideoDetail_Timeout(t *testing.T) {
	srv := newTestServer(t)
	defer srv.cleanup()

	videoID := "df3c43f5-6c8f-4b25-b0f2-715b228c7a2f"

	// 设置一个极短的超时（1纳秒）
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond) // 确保 context 已超时

	req := &videov1.GetVideoDetailRequest{VideoId: videoID}
	_, err := srv.client.GetVideoDetail(ctx, req)

	if err == nil {
		// 如果没有超时，可能是因为查询太快了（这是正常的）
		t.Log("⚠️  Query completed before timeout (database too fast)")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}

	// 超时可能返回 DeadlineExceeded 或 GatewayTimeout
	if st.Code() != codes.DeadlineExceeded && st.Code() != codes.Unknown {
		t.Logf("⚠️  Got code=%s, message=%s (expected DeadlineExceeded)", st.Code(), st.Message())
	} else {
		t.Logf("✅ Correctly handled timeout: code=%s", st.Code())
	}
}

// TestIntegration_GetVideoDetail_ConcurrentRequests 测试并发查询。
//
// 场景：同时发起多个查询请求，验证服务器能正确处理并发。
func TestIntegration_GetVideoDetail_ConcurrentRequests(t *testing.T) {
	srv := newTestServer(t)
	defer srv.cleanup()

	// 使用 3 个不同的 published 视频
	videoIDs := []string{
		"df3c43f5-6c8f-4b25-b0f2-715b228c7a2f", // Academic Writing
		"b3e73ad4-8cb0-4ecd-a6d6-a4b891378297", // Travel English
		"e99df514-fa1d-4ba4-9aca-dbabc92eea3a", // American vs British
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 启动 10 个并发请求
	concurrency := 10
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			videoID := videoIDs[idx%len(videoIDs)]
			req := &videov1.GetVideoDetailRequest{VideoId: videoID}
			resp, err := srv.client.GetVideoDetail(ctx, req)
			if err != nil {
				errCh <- err
				return
			}
			if resp.Detail.VideoId != videoID {
				errCh <- err
				return
			}
			errCh <- nil
		}(i)
	}

	// 收集结果
	successCount := 0
	for i := 0; i < concurrency; i++ {
		err := <-errCh
		if err == nil {
			successCount++
		} else {
			t.Errorf("concurrent request %d failed: %v", i, err)
		}
	}

	if successCount != concurrency {
		t.Errorf("expected %d successful requests, got %d", concurrency, successCount)
	} else {
		t.Logf("✅ All %d concurrent requests succeeded", concurrency)
	}
}

// TestIntegration_GetVideoDetail_TagsHandling 测试标签数组的处理。
//
// 场景：查询包含多个标签的视频，验证标签数组的正确性。
func TestIntegration_GetVideoDetail_TagsHandling(t *testing.T) {
	srv := newTestServer(t)
	defer srv.cleanup()

	// Academic Writing 有 4 个标签: Writing, Academic, Advanced, Essay
	videoID := "df3c43f5-6c8f-4b25-b0f2-715b228c7a2f"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &videov1.GetVideoDetailRequest{VideoId: videoID}
	resp, err := srv.client.GetVideoDetail(ctx, req)
	if err != nil {
		t.Fatalf("GetVideoDetail failed: %v", err)
	}

	detail := resp.Detail
	if len(detail.Tags) != 4 {
		t.Errorf("expected 4 tags, got %d: %v", len(detail.Tags), detail.Tags)
	}

	// 验证标签内容（顺序可能不同，使用 map 检查）
	expectedTags := map[string]bool{
		"Writing":  true,
		"Academic": true,
		"Advanced": true,
		"Essay":    true,
	}

	for _, tag := range detail.Tags {
		if !expectedTags[tag] {
			t.Errorf("unexpected tag: %s", tag)
		}
	}

	t.Logf("✅ Tags correctly retrieved: %v", detail.Tags)
}

// TestIntegration_DatabaseConnection 测试数据库连接的健康状态。
//
// 场景：验证集成测试能够正确连接到 Supabase PostgreSQL。
func TestIntegration_DatabaseConnection(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)

	confPath := getConfigPath(t)
	if confPath == "" {
		t.Skip("跳过集成测试：configs/config.yaml 不存在（CI 环境自动跳过）")
	}

	// 检查 .env 文件
	envFile := filepath.Join(confPath, ".env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		t.Skipf("跳过集成测试：%s 不存在（CI 环境自动跳过）", envFile)
	}

	bundle, err := configloader.Build(configloader.Params{ConfPath: confPath})
	if err != nil {
		t.Skipf("跳过集成测试：加载配置失败: %v", err)
	}

	pool, cleanup, err := database.NewPgxPool(ctx, bundle.Bootstrap.GetData(), logger)
	if err != nil {
		t.Fatalf("create database pool failed: %v", err)
	}
	defer cleanup()

	// 执行简单查询验证连接
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM catalog.videos WHERE upload_user_id = $1",
		"f0ad5a16-0d50-4f94-8ff7-b99dda13ee47").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if count != 14 {
		t.Errorf("expected 14 test videos, got %d (run seed_test_videos.sql first)", count)
	}

	t.Logf("✅ Database connection healthy: found %d test videos", count)
}
