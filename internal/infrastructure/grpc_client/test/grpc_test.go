package grpcclient_test

import (
	"context"
	"io"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	"github.com/bionicotaku/lingo-services-catalog/internal/controllers"
	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	clientinfra "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/grpc_client"
	grpcserver "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/lingo-services-catalog/internal/models/po"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"

	"github.com/bionicotaku/lingo-utils/observability"
	txmanager "github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
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

func startVideoServer(t *testing.T) (addr string, stop func()) {
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	t.Helper()
	logger := log.NewStdLogger(io.Discard)
	repo := &videoRepoStub{}
	writer := services.NewLifecycleWriter(repo, outboxRepoStub{}, noopTxManager{}, logger)
	querySvc := services.NewVideoQueryService(repo, nil, noopTxManager{}, logger)
	base := controllers.NewBaseHandler(controllers.HandlerTimeouts{})
	lifecycleSvc := services.NewLifecycleService(
		services.NewRegisterUploadService(writer),
		services.NewOriginalMediaService(writer, repo),
		services.NewProcessingStatusService(writer, repo),
		services.NewMediaInfoService(writer, repo),
		services.NewAIAttributesService(writer, repo),
		services.NewVisibilityService(writer, repo),
	)
	lifecycleHandler := controllers.NewLifecycleHandler(lifecycleSvc, base)
	queryHandler := controllers.NewVideoQueryHandler(querySvc, base)

	cfg := configloader.ServerConfig{
		Address:      "127.0.0.1:0",
		MetadataKeys: []string{"x-apigateway-api-userinfo", "x-md-", "x-md-idempotency-key", "x-md-if-match", "x-md-if-none-match"},
	}
	grpcSrv := grpcserver.NewGRPCServer(cfg, metricsCfg, nil, lifecycleHandler, queryHandler, logger)

	endpointURL, err := grpcSrv.Endpoint()
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	addr = endpointURL.Host

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := grpcSrv.Start(ctx); err != nil && err != context.Canceled {
			t.Logf("server exited: %v", err)
		}
	}()

	waitForServer(t, addr)

	stop = func() {
		cancel()
		_ = grpcSrv.Stop(context.Background())
	}
	return addr, stop
}

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

func TestNewGRPCClient_NoTarget(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}
	conn, cleanup, err := clientinfra.NewGRPCClient(configloader.GRPCClientConfig{}, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != nil {
		t.Fatalf("expected nil connection when target missing")
	}
	if cleanup == nil {
		t.Fatalf("cleanup should always be non-nil")
	}
	cleanup()
}

func TestNewGRPCClient_CallVideo(t *testing.T) {
	addr, stop := startVideoServer(t)
	defer stop()

	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}
	cfg := configloader.GRPCClientConfig{
		Target:       "dns:///" + addr,
		MetadataKeys: []string{"x-apigateway-api-userinfo", "x-md-", "x-md-idempotency-key", "x-md-if-match", "x-md-if-none-match"},
	}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatalf("expected connection")
	}
	defer cleanup()

	client := videov1.NewCatalogQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: uuid.NewString()})
	// 期望返回 NotFound（因为 stub 总是返回 ErrVideoNotFound）
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", status.Code(err))
	}
}

func TestNewGRPCClient_VideoInvalidID(t *testing.T) {
	addr, stop := startVideoServer(t)
	defer stop()

	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}
	cfg := configloader.GRPCClientConfig{Target: "dns:///" + addr}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, nil, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatalf("expected connection")
	}
	defer cleanup()

	client := videov1.NewCatalogQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: ""})
	// 期望返回 InvalidArgument（空 video_id）
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}
