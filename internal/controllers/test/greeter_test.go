// Package controllers_test 提供 controllers 层的黑盒测试。
package controllers_test

import (
	"context"
	"io"
	"testing"

	v1 "github.com/bionicotaku/kratos-template/api/helloworld/v1"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/go-kratos/kratos/v2/log"
	kratosmd "github.com/go-kratos/kratos/v2/metadata"
)

// stubGreeterRepo 是 GreeterRepo 接口的测试桩实现，用于隔离数据层依赖。
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

// stubGreeterRemote 是 GreeterRemote 接口的测试桩，用于验证远程调用逻辑。
type stubGreeterRemote struct {
	calls   int             // 记录被调用次数
	lastCtx context.Context // 记录最后一次调用时的上下文
	reply   string          // 模拟的远程响应
}

func (s *stubGreeterRemote) SayHello(ctx context.Context, _ string) (string, error) {
	s.calls++
	s.lastCtx = ctx
	return s.reply, nil
}

// newTestController 构造用于测试的 GreeterHandler 及其依赖的测试桩。
func newTestController(remoteReply string) (*controllers.GreeterHandler, *stubGreeterRemote) {
	repo := stubGreeterRepo{}
	remote := &stubGreeterRemote{reply: remoteReply}
	baseLogger := log.NewStdLogger(io.Discard)
	uc := services.NewGreeterUsecase(repo, remote, baseLogger)
	return controllers.NewGreeterHandler(uc), remote
}

const forwardedHeader = "x-template-forwarded"

// TestGreeterController_ForwardedOnce 验证在未标记为已转发的请求中，
// Controller 会调用远程服务一次，并将 forwardedHeader 传递到远程调用的 metadata 中。
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

// TestGreeterController_AvoidsRecursiveForward 验证当请求已被标记为 forwardedHeader 时，
// Controller 不会再次调用远程服务，从而防止递归调用导致的死循环。
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
