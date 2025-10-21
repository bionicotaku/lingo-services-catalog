package server

import (
	stdhttp "net/http"

	v1 "github.com/go-kratos/kratos-layout/api/helloworld/v1"
	"github.com/go-kratos/kratos-layout/internal/conf"
	"github.com/go-kratos/kratos-layout/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	kmetrics "github.com/go-kratos/kratos/v2/middleware/metrics"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, greeter *service.GreeterService, logger log.Logger, telemetry *Telemetry) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
			kmetrics.Server(
				kmetrics.WithRequests(telemetry.RequestCounter),
				kmetrics.WithSeconds(telemetry.SecondsHistogram),
			),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}

	srv := http.NewServer(opts...)

	srv.Handle("/healthz", stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
	}))

	srv.Handle("/readyz", stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		// 预留 readiness 校验钩子：若未来需要检查数据库等依赖，可在此处扩展。
		w.WriteHeader(stdhttp.StatusOK)
	}))

	srv.Handle("/metrics", promhttp.HandlerFor(telemetry.PrometheusRegistry, promhttp.HandlerOpts{}))

	v1.RegisterGreeterHTTPServer(srv, greeter)
	return srv
}
