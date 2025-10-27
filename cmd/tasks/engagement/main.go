// Package main 提供 Engagement Runner 独立进程入口。
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	"github.com/go-kratos/kratos/v2/log"
)

type engagementApp struct {
	Runner engagementRunner
	Logger log.Logger
}

type engagementRunner interface {
	Run(context.Context) error
}

func main() {
	ctx := context.Background()

	confFlag := flag.String("conf", "", "config path or directory, eg: -conf configs/config.yaml")
	flag.Parse()

	params := configloader.Params{ConfPath: *confFlag}
	app, cleanup, err := wireEngagementTask(ctx, params)
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
		helper.Warn("engagement runner disabled (missing messaging.engagement configuration)")
		return
	}

	helper.Info("starting engagement runner")

	runCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Runner.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
		helper.Errorf("engagement runner stopped unexpectedly: %v", err)
		os.Exit(1)
	}

	helper.Info("engagement runner stopped")
}
