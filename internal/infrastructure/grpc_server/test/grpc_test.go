package grpcserver_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	videov1 "github.com/bionicotaku/kratos-template/api/video/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	kratosmd "github.com/go-kratos/kratos/v2/metadata"
	"github.com/google/uuid"
	stdgrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type testRepo struct{}

func (testRepo) Save(_ context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

func (testRepo) Update(context.Context, *po.Greeter) (*po.Greeter, error) {
	return nil, nil
}
func (testRepo) FindByID(context.Context, int64) (*po.Greeter, error)       { return nil, nil }
func (testRepo) ListByHello(context.Context, string) ([]*po.Greeter, error) { return nil, nil }
func (testRepo) ListAll(context.Context) ([]*po.Greeter, error)             { return nil, nil }

type noopRemote struct{}

func (noopRemote) SayHello(context.Context, string) (string, error) { return "", nil }

func newTestController(t *testing.T) *controllers.GreeterHandler {
	t.Helper()
	logger := log.NewStdLogger(io.Discard)
	uc := services.NewGreeterUsecase(testRepo{}, noopRemote{}, logger)
	return controllers.NewGreeterHandler(uc)
}

type videoRepoStub struct{}

func (videoRepoStub) FindByID(context.Context, uuid.UUID) (*po.Video, error) {
	return nil, repositories.ErrVideoNotFound
}

func newVideoController(t *testing.T) *controllers.VideoHandler {
	t.Helper()
	logger := log.NewStdLogger(io.Discard)
	repo := videoRepoStub{}
	uc := services.NewVideoUsecase(repo, logger)
	return controllers.NewVideoHandler(uc)
}

func startServer(t *testing.T) (string, func()) {
	t.Helper()
	svc := newTestController(t)
	videoSvc := newVideoController(t)
	cfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	logger := log.NewStdLogger(io.Discard)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}
	srv := grpcserver.NewGRPCServer(cfg, metricsCfg, svc, videoSvc, logger)

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

func TestNewGRPCServerServesGreeter(t *testing.T) {
	addr, cleanup := startServer(t)
	defer cleanup()

	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := v1.NewGreeterClient(conn)
	resp, err := client.SayHello(context.Background(), &v1.HelloRequest{Name: "Tester"})
	if err != nil {
		t.Fatalf("SayHello: %v", err)
	}
	if resp.GetMessage() != "Hello Tester" {
		t.Fatalf("unexpected response: %s", resp.GetMessage())
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

	client := v1.NewGreeterClient(conn)
	md := kratosmd.New(map[string][]string{"x-template-user": {"abc"}})
	ctx := kratosmd.NewClientContext(context.Background(), md)
	if _, err := client.SayHello(ctx, &v1.HelloRequest{Name: "Met"}); err != nil {
		t.Fatalf("SayHello with metadata: %v", err)
	}

	// Successful invocation is enough to confirm metadata with allowed prefix is accepted.
}

func TestNewGRPCServerValidationRejectsInvalid(t *testing.T) {
	addr, cleanup := startServer(t)
	defer cleanup()

	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := v1.NewGreeterClient(conn)
	_, err = client.SayHello(context.Background(), &v1.HelloRequest{Name: ""})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestVideoServiceRejectsInvalidID(t *testing.T) {
	addr, cleanup := startServer(t)
	defer cleanup()

	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewVideoQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: ""})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
	}
}
