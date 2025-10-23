package grpcclient_test

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

	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type videoRepoStub struct{}

func (videoRepoStub) FindByID(context.Context, uuid.UUID) (*po.Video, error) {
	return nil, repositories.ErrVideoNotFound
}

func startVideoServer(t *testing.T) (addr string, stop func()) {
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	t.Helper()
	logger := log.NewStdLogger(io.Discard)
	videoSvc := controllers.NewVideoHandler(services.NewVideoUsecase(videoRepoStub{}, logger))

	cfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	grpcSrv := grpcserver.NewGRPCServer(cfg, metricsCfg, videoSvc, logger)

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
	conn, cleanup, err := clientinfra.NewGRPCClient(&configpb.Data{}, metricsCfg, logger)
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
	cfg := &configpb.Data{GrpcClient: &configpb.Data_Client{Target: "dns:///" + addr}}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatalf("expected connection")
	}
	defer cleanup()

	client := videov1.NewVideoQueryServiceClient(conn)
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
	cfg := &configpb.Data{GrpcClient: &configpb.Data_Client{Target: "dns:///" + addr}}

	conn, cleanup, err := clientinfra.NewGRPCClient(cfg, metricsCfg, logger)
	if err != nil {
		t.Fatalf("NewGRPCClient error: %v", err)
	}
	if conn == nil {
		t.Fatalf("expected connection")
	}
	defer cleanup()

	client := videov1.NewVideoQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: ""})
	// 期望返回 InvalidArgument（空 video_id）
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}
