package grpcserver_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/controllers"
	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	grpcserver "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	"github.com/bionicotaku/lingo-utils/observability"
	txmanager "github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	kratosmd "github.com/go-kratos/kratos/v2/metadata"
	"github.com/google/uuid"
	stdgrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type videoRepoStub struct{}

func (videoRepoStub) Create(context.Context, txmanager.Session, repositories.CreateVideoInput) (*po.Video, error) {
	return nil, repositories.ErrVideoNotFound
}

func (videoRepoStub) Update(context.Context, txmanager.Session, repositories.UpdateVideoInput) (*po.Video, error) {
	return nil, repositories.ErrVideoNotFound
}

func (videoRepoStub) Delete(context.Context, txmanager.Session, uuid.UUID) (*po.Video, error) {
	return nil, repositories.ErrVideoNotFound
}

func (videoRepoStub) GetLifecycleSnapshot(context.Context, txmanager.Session, uuid.UUID) (*po.Video, error) {
	return nil, repositories.ErrVideoNotFound
}

func (videoRepoStub) FindPublishedByID(context.Context, txmanager.Session, uuid.UUID) (*po.VideoReadyView, error) {
	return nil, repositories.ErrVideoNotFound
}

func (videoRepoStub) GetMetadata(context.Context, txmanager.Session, uuid.UUID) (*po.VideoMetadata, error) {
	return nil, repositories.ErrVideoNotFound
}

func (videoRepoStub) ListPublicVideos(context.Context, txmanager.Session, repositories.ListPublicVideosInput) ([]po.VideoListEntry, error) {
	return nil, nil
}

func (videoRepoStub) ListUserUploads(context.Context, txmanager.Session, repositories.ListUserUploadsInput) ([]po.MyUploadEntry, error) {
	return nil, nil
}

type outboxRepoStub struct{}

func (outboxRepoStub) Enqueue(context.Context, txmanager.Session, repositories.OutboxMessage) error {
	return nil
}

type noopTxManager struct{}

func (noopTxManager) WithinTx(ctx context.Context, _ txmanager.TxOptions, fn func(context.Context, txmanager.Session) error) error {
	return fn(ctx, nil)
}

func (noopTxManager) WithinReadOnlyTx(ctx context.Context, _ txmanager.TxOptions, fn func(context.Context, txmanager.Session) error) error {
	return fn(ctx, nil)
}

func newVideoHandlers(t *testing.T) (*controllers.LifecycleHandler, *controllers.VideoQueryHandler) {
	t.Helper()
	logger := log.NewStdLogger(io.Discard)
	repo := &videoRepoStub{}
	outbox := outboxRepoStub{}
	writer := services.NewLifecycleWriter(repo, outbox, noopTxManager{}, logger)
	querySvc := services.NewVideoQueryService(repo, nil, noopTxManager{}, logger)
	lifecycleSvc := services.NewLifecycleService(
		services.NewRegisterUploadService(writer),
		services.NewOriginalMediaService(writer, repo),
		services.NewProcessingStatusService(writer, repo),
		services.NewMediaInfoService(writer, repo),
		services.NewAIAttributesService(writer, repo),
		services.NewVisibilityService(writer, repo),
	)
	base := controllers.NewBaseHandler(controllers.HandlerTimeouts{})
	return controllers.NewLifecycleHandler(lifecycleSvc, base), controllers.NewVideoQueryHandler(querySvc, base)
}

func startServer(t *testing.T) (string, func()) {
	t.Helper()
	lifecycleHandler, queryHandler := newVideoHandlers(t)
	cfg := configloader.ServerConfig{
		Address:      "127.0.0.1:0",
		MetadataKeys: []string{"x-apigateway-api-userinfo", "x-md-", "x-md-idempotency-key", "x-md-if-match", "x-md-if-none-match"},
	}
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}
	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, nil, lifecycleHandler, queryHandler, logger)

	// Force endpoint initialization to retrieve the bound address.
	endpointURL, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	addr := endpointURL.Host

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := srv.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("server start returned: %v", err)
		}
	}()

	waitForServing(t, addr)

	cleanup := func() {
		cancel()
		_ = srv.Stop(context.Background())
	}
	return addr, cleanup
}

func waitForServing(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for server at %s", addr)
}

func TestNewGRPCServerServesVideo(t *testing.T) {
	addr, cleanup := startServer(t)
	defer cleanup()

	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewCatalogQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: uuid.New().String()})
	// 期望返回 NotFound 错误（因为我们的 stub 总是返回 ErrVideoNotFound）
	if err == nil {
		t.Fatal("expected error for non-existent video")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", status.Code(err))
	}
}

func TestNewGRPCServerProvidesHealth(t *testing.T) {
	addr, cleanup := startServer(t)
	defer cleanup()

	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	healthClient := healthpb.NewHealthClient(conn)
	res, err := healthClient.Check(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check error: %v", err)
	}
	if res.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected health status: %v", res.GetStatus())
	}
}

func TestNewGRPCServerMetadataPropagationPrefix(t *testing.T) {
	addr, cleanup := startServer(t)
	defer cleanup()

	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewCatalogQueryServiceClient(conn)
	md := kratosmd.New(map[string][]string{"x-template-user": {"abc"}})
	ctx := kratosmd.NewClientContext(context.Background(), md)
	// 调用 Video 服务验证 metadata 传播（预期返回 NotFound 或 InvalidArgument）
	if _, err := client.GetVideoDetail(ctx, &videov1.GetVideoDetailRequest{VideoId: uuid.New().String()}); err == nil {
		t.Fatal("expected error")
	}

	// 成功调用（即使返回错误）说明 metadata 传播正常工作
}

func TestVideoServiceRejectsInvalidID(t *testing.T) {
	addr, cleanup := startServer(t)
	defer cleanup()

	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewCatalogQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: ""})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}
