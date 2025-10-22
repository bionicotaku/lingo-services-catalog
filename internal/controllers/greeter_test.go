package controllers

import (
	"context"
	"io"
	"testing"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/log"
	kratosmd "github.com/go-kratos/kratos/v2/metadata"
)

type stubGreeterRepo struct{}

func (stubGreeterRepo) Save(_ context.Context, g *po.Greeter) (*po.Greeter, error) {
	return g, nil
}

func (stubGreeterRepo) Update(context.Context, *po.Greeter) (*po.Greeter, error) {
	return nil, nil
}

func (stubGreeterRepo) FindByID(context.Context, int64) (*po.Greeter, error) {
	return nil, nil
}

func (stubGreeterRepo) ListByHello(context.Context, string) ([]*po.Greeter, error) {
	return nil, nil
}

func (stubGreeterRepo) ListAll(context.Context) ([]*po.Greeter, error) {
	return nil, nil
}

type stubGreeterRemote struct {
	calls   int
	lastCtx context.Context
	reply   string
}

func (s *stubGreeterRemote) SayHello(ctx context.Context, name string) (string, error) {
	s.calls++
	s.lastCtx = ctx
	return s.reply, nil
}

func newTestController(remoteReply string) (*GreeterController, *stubGreeterRemote) {
	repo := stubGreeterRepo{}
	remote := &stubGreeterRemote{reply: remoteReply}
	baseLogger := log.NewStdLogger(io.Discard)
	uc := services.NewGreeterUsecase(repo, remote, baseLogger)
	return NewGreeterController(uc), remote
}

func TestGreeterController_ForwardedOnce(t *testing.T) {
	svc, remote := newTestController("Remote Hello")

	ctx := kratosmd.NewServerContext(context.Background(), kratosmd.New())
	resp, err := svc.SayHello(ctx, &v1.HelloRequest{Name: "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.GetMessage(); got != "Hello Alice | remote: Remote Hello" {
		t.Fatalf("unexpected message: %s", got)
	}
	if remote.calls != 1 {
		t.Fatalf("expected remote to be called once, got %d", remote.calls)
	}
	if md, ok := kratosmd.FromClientContext(remote.lastCtx); !ok || md.Get(forwardedHeader) != "true" {
		t.Fatalf("forwarded header not propagated: %+v", md)
	}
}

func TestGreeterController_AvoidsRecursiveForward(t *testing.T) {
	svc, remote := newTestController("Remote Hello")

	md := kratosmd.New(map[string][]string{forwardedHeader: {"true"}})
	ctx := kratosmd.NewServerContext(context.Background(), md)
	resp, err := svc.SayHello(ctx, &v1.HelloRequest{Name: "Bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.GetMessage(); got != "Hello Bob" {
		t.Fatalf("unexpected message: %s", got)
	}
	if remote.calls != 0 {
		t.Fatalf("expected remote not to be called, got %d", remote.calls)
	}
}
