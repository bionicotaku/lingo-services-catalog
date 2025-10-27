package grpcserver_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	videov1 "github.com/bionicotaku/lingo-services-catalog/api/video/v1"
	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	grpcserver "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/grpc_server"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	stdgrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// TestJWTServer_SkipValidate_OptionalToken 确认 skip_validate 允许匿名请求。
func TestJWTServer_SkipValidate_OptionalToken(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	commandHandler, queryHandler := newVideoHandlers(t)
	metricsCfg := &observability.MetricsConfig{GRPCEnabled: true, GRPCIncludeHealth: false}

	jwtCfg := gcjwt.Config{
		Server: &gcjwt.ServerConfig{
			ExpectedAudience: "https://example.run.app/",
			SkipValidate:     true,
			Required:         false,
		},
	}
	component, cleanup, err := gcjwt.NewComponent(jwtCfg, logger)
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	defer cleanup()

	serverMW, err := gcjwt.ProvideServerMiddleware(component)
	if err != nil {
		t.Fatalf("ProvideServerMiddleware: %v", err)
	}

	srvCfg := configloader.ServerConfig{Address: "127.0.0.1:0"}
	server := grpcserver.NewGRPCServer(srvCfg, metricsCfg, serverMW, commandHandler, queryHandler, logger)

	addr, stop := startKratosServer(t, server)
	defer stop()

	conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := videov1.NewCatalogQueryServiceClient(conn)
	_, err = client.GetVideoDetail(context.Background(), &videov1.GetVideoDetailRequest{VideoId: uuid.New().String()})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", status.Code(err))
	}
}

func startKratosServer(t *testing.T, srv *kgrpc.Server) (string, func()) {
	t.Helper()

	endpointURL, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	addr := endpointURL.Host

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := srv.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("server exited: %v", err)
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := stdgrpc.NewClient(addr, stdgrpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	stop := func() {
		cancel()
		_ = srv.Stop(context.Background())
	}
	return addr, stop
}
