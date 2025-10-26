// Package main 提供 Outbox Runner 独立进程入口，便于在后台单独运行发布器。
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	outboxpublisher "github.com/bionicotaku/lingo-utils/outbox/publisher"
	"github.com/go-kratos/kratos/v2/log"
)

type outboxTaskApp struct {
	Runner *outboxpublisher.Runner
	Logger log.Logger
}

func main() {
	ctx := context.Background()

	confFlag := flag.String("conf", "", "config path or directory, eg: -conf configs/config.yaml")
	flag.Parse()

	params := configloader.Params{ConfPath: *confFlag}
	app, cleanup, err := wireOutboxTask(ctx, params)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	logger := app.Logger
	if logger == nil {
		logger = log.NewStdLogger(os.Stdout)
	}
	helper := log.NewHelper(logger)

	if app.Runner == nil {
		helper.Warn("outbox runner disabled (missing messaging.pubsub configuration)")
		return
	}

	helper.Info("starting outbox publisher task")

	runCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Runner.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
		helper.Errorf("outbox runner stopped unexpectedly: %v", err)
		os.Exit(1)
	}

	helper.Info("outbox publisher stopped")
}
