// Package main 提供上传回调 Runner 的独立进程入口，便于后台单独运行。
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	uploadrunner "github.com/bionicotaku/lingo-services-catalog/internal/tasks/uploads"
	"github.com/go-kratos/kratos/v2/log"
)

type uploadsTaskApp struct {
	Runner *uploadrunner.Runner
	Logger log.Logger
}

func main() {
	ctx := context.Background()

	confFlag := flag.String("conf", "", "config path or directory, eg: -conf configs/config.yaml")
	flag.Parse()

	params := configloader.Params{ConfPath: *confFlag}
	app, cleanup, err := wireUploadsTask(ctx, params)
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
		helper.Warn("uploads runner disabled (missing messaging.topics[\"uploads\"] configuration)")
		return
	}

	helper.Info("starting uploads callback runner")

	runCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Runner.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
		helper.Errorf("uploads runner stopped unexpectedly: %v", err)
		os.Exit(1)
	}

	helper.Info("uploads runner stopped")
}
