//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"

	"github.com/bionicotaku/kratos-template/internal/clients"
	"github.com/bionicotaku/kratos-template/internal/controllers"
	configloader "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	grpcclient "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_client"
	grpcserver "github.com/bionicotaku/kratos-template/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/bionicotaku/kratos-template/internal/services"

	"github.com/bionicotaku/lingo-utils/gclog"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
)

// wireApp 构建整个 Kratos 应用，包括：
// 1. 读取配置（Server/Data/ServiceMetadata/Observability）。
// 2. 初始化 gclog 日志组件，暴露 trace 关联的 log.Logger。
// 3. 初始化观测组件（Tracing/Metrics Provider）。
// 4. 构造 gRPC Server/Client、仓储、业务用例、控制层。
// 5. 返回带统一 cleanup 的 kratos.App。
func wireApp(context.Context, *configpb.Server, *configpb.Data, obswire.ObservabilityConfig, configloader.ServiceMetadata) (*kratos.App, func(), error) {
	// Providers and their dependencies:
	//   - configloader.ProvideLoggerConfig(configloader.ServiceMetadata) gclog.Config
	//       由服务元信息（名称/版本/环境/实例 ID）生成 gclog 所需的 Config。
	//   - gclog.NewComponent(gclog.Config) (*gclog.Component, func(), error)
	//       初始化结构化日志组件，返回可选的 cleanup。
	//   - gclog.ProvideLogger(*gclog.Component) log.Logger
	//       从日志组件提取 trace-aware 的 log.Logger。
	//   - configloader.ProvideObservabilityInfo(configloader.ServiceMetadata) observability.ServiceInfo
	//       将服务元信息转为观测使用的 ServiceInfo。
	//   - observability.NewComponent(context.Context, observability.ObservabilityConfig, observability.ServiceInfo, log.Logger) (*observability.Component, func(), error)
	//       初始化 Tracer/Meter Provider，绑定 Service/Logger，并返回 cleanup。
	//   - observability.ProvideMetricsConfig(observability.ObservabilityConfig) *observability.MetricsConfig
	//       提供 gRPC 指标配置（含默认值）。
	//   - grpc_server.NewGRPCServer(*configpb.Server, *observability.MetricsConfig, *controllers.GreeterHandler, log.Logger) *grpc.Server
	//       构建 gRPC Server，注入指标、日志等中间件。
	//   - grpc_client.NewGRPCClient(*configpb.Data, *observability.MetricsConfig, log.Logger) (*grpc.ClientConn, func(), error)
	//       构建 gRPC Client 连接（用于跨服务调用），并返回 cleanup。
	//   - clients.NewGreeterRemote(*grpc.ClientConn, log.Logger) services.GreeterRemote
	//       基于 clientConn 封装远程 Greeter 客户端。
	//   - repositories.NewGreeterRepo(*data.Data, log.Logger) services.GreeterRepo
	//       构造仓储层，处理数据访问。
	//   - services.NewGreeterUsecase(repositories.GreeterRepo, services.GreeterRemote, log.Logger) *services.GreeterUsecase
	//       组装业务用例，协调仓储与远程客户端。
	//   - controllers.NewGreeterHandler(*services.GreeterUsecase) *controllers.GreeterHandler
	//       构造控制层，为 gRPC handler 提供入口。
	//   - newApp(log.Logger, *grpc.Server) *kratos.App
	//       将日志、观测组件和 gRPC Server 装配成 Kratos 应用。
	panic(wire.Build(
		configloader.ProvideLoggerConfig,
		gclog.ProviderSet,
		configloader.ProvideObservabilityInfo,
		obswire.ProviderSet,
		grpcserver.ProviderSet,
		grpcclient.ProviderSet,
		clients.ProviderSet,
		repositories.ProviderSet,
		services.ProviderSet,
		controllers.ProviderSet,
		newApp,
	))
}
