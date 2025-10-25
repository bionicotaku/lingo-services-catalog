// Package main 提供 Kratos gRPC 服务的启动入口。
// 负责加载配置、初始化依赖（通过 Wire）、启动 gRPC Server 并优雅关闭。
package main

import (
	"context"
	"errors"
	"flag"
	"sync"

	"github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
	"github.com/bionicotaku/kratos-template/internal/tasks/outbox"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"

	_ "go.uber.org/automaxprocs" // 自动设置 GOMAXPROCS 为容器 CPU 配额
)

// newApp 负责组装 Kratos 应用：注入观测组件、日志器、服务元信息以及 gRPC Server。
//
// 参数：
//   - obsCmp: 可观测性组件（Tracer/Meter Provider），Wire 自动管理生命周期
//   - logger: 结构化日志器（gclog），包含 trace_id/span_id 关联
//   - gs: 配置完整的 gRPC Server（已注册 Handler 和中间件）
//   - meta: 服务元信息（Name/Version/Environment/InstanceID）
//
// 返回 kratos.App 实例，调用 app.Run() 启动服务并阻塞直到收到停止信号。
func newApp(_ *obswire.Component, logger log.Logger, gs *grpc.Server, meta loader.ServiceMetadata, publisher *outbox.PublisherTask) *kratos.App {
	options := []kratos.Option{
		kratos.ID(meta.InstanceID),
		kratos.Name(meta.Name),
		kratos.Version(meta.Version),
		kratos.Metadata(map[string]string{"environment": meta.Environment}),
		kratos.Logger(logger),
		kratos.Server(gs),
	}

	if publisher != nil {
		var (
			wg     sync.WaitGroup
			cancel context.CancelFunc
		)

		helper := log.NewHelper(logger)
		options = append(options,
			kratos.BeforeStart(func(ctx context.Context) error {
				runCtx, c := context.WithCancel(ctx)
				cancel = c
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := publisher.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
						helper.Warnf("outbox publisher stopped: %v", err)
					}
				}()
				return nil
			}),
			kratos.AfterStop(func(ctx context.Context) error {
				if cancel != nil {
					cancel()
				}
				done := make(chan struct{})
				go func() {
					wg.Wait()
					close(done)
				}()
				select {
				case <-ctx.Done():
				case <-done:
				}
				return nil
			}),
		)
	}

	return kratos.New(options...)
}

func main() {
	ctx := context.Background()

	// 1. 解析命令行参数：-conf 指定配置文件路径或目录
	confFlag := flag.String("conf", "", "config path or directory, eg: -conf configs/config.yaml")
	flag.Parse()

	// 2. 构造配置加载参数
	params := loader.Params{
		ConfPath: *confFlag, // 配置路径（可为空，使用默认值或环境变量）
	}

	// 3. 通过 Wire 装配所有依赖（logger、servers、repositories 等）并创建 Kratos App
	// wireApp 由 wire_gen.go 自动生成，依赖注入顺序见 wire.go
	app, cleanupApp, err := wireApp(ctx, params)
	if err != nil {
		panic(err)
	}
	defer cleanupApp()

	// 4. 启动应用并阻塞，直到收到停止信号（SIGINT/SIGTERM）
	// Kratos 会优雅关闭所有注册的 Server
	if err := app.Run(); err != nil {
		panic(err)
	}
}
