package grpcclient_test

import (
	"context"
	"io"
	"testing"
	"time"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	clientinfra "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_client"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type repoStub struct{}

func (repoStub) Save(_ context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

func (repoStub) Update(context.Context, *po.Greeter) (*po.Greeter, error)   { return nil, nil }
func (repoStub) FindByID(context.Context, int64) (*po.Greeter, error)       { return nil, nil }
func (repoStub) ListByHello(context.Context, string) ([]*po.Greeter, error) { return nil, nil }
func (repoStub) ListAll(context.Context) ([]*po.Greeter, error)             { return nil, nil }

type remoteStub struct{}

func (remoteStub) SayHello(context.Context, string) (string, error) { return "", nil }

func startGreeterServer(t *testing.T) (addr string, stop func()) {
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	t.Helper()
	logger := log.NewStdLogger(io.Discard)
	uc := services.NewGreeterUsecase(repoStub{}, remoteStub{}, logger)
	svc := controllers.NewGreeterHandler(uc)

	cfg := &configpb.Server{Grpc: &configpb.Server_GRPC{Addr: "127.0.0.1:0"}}
	grpcSrv := grpcserver.NewGRPCServer(cfg, metricsCfg, svc, logger)

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

func TestNewGRPCClient_CallGreeter(t *testing.T) {
	addr, stop := startGreeterServer(t)
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

	client := v1.NewGreeterClient(conn)
	resp, err := client.SayHello(context.Background(), &v1.HelloRequest{Name: "Client"})
	if err != nil {
		t.Fatalf("SayHello failed: %v", err)
	}
	if resp.GetMessage() != "Hello Client" {
		t.Fatalf("unexpected response: %s", resp.GetMessage())
	}
}
