//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

//go:generate go run github.com/google/wire/cmd/wire

package main

import (
	"context"

	"github.com/bionicotaku/lingo-services-catalog/internal/controllers"
	configloader "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/configloader"
	grpcserver "github.com/bionicotaku/lingo-services-catalog/internal/infrastructure/grpc_server"
	"github.com/bionicotaku/lingo-services-catalog/internal/repositories"
	"github.com/bionicotaku/lingo-services-catalog/internal/services"
	engagementtasks "github.com/bionicotaku/lingo-services-catalog/internal/tasks/engagement"
	outboxtasks "github.com/bionicotaku/lingo-services-catalog/internal/tasks/outbox"

	"github.com/bionicotaku/lingo-utils/gcjwt"
	"github.com/bionicotaku/lingo-utils/gclog"
	"github.com/bionicotaku/lingo-utils/gcpubsub"
	obswire "github.com/bionicotaku/lingo-utils/observability"
	"github.com/bionicotaku/lingo-utils/pgxpoolx"
	"github.com/bionicotaku/lingo-utils/txmanager"
	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
)

// wireApp 构建整个 Kratos 应用，分阶段装配依赖。
//
// Wire 会根据类型自动解析依赖关系并生成 wire_gen.go，详细的 Provider 列表见文件末尾注释。
//
// 依赖注入顺序:
//  1. 配置加载: configloader.ProviderSet 解析配置并派生组件配置
//  2. 基础设施: gclog → observability → gcjwt → pgxpoolx → txmanager
//  3. 业务层: repositories → services → controllers
//  4. 服务器: grpc_server.ProviderSet 组装 gRPC Server
//  5. 应用: newApp 创建 Kratos App
func wireApp(context.Context, configloader.Params) (*kratos.App, func(), error) {
	panic(wire.Build(
		configloader.ProviderSet, // 配置加载与解析
		gclog.ProviderSet,        // 结构化日志
		gcjwt.ProviderSet,        // JWT 认证中间件
		obswire.ProviderSet,      // OpenTelemetry 追踪和指标
		pgxpoolx.ProviderSet,     // PostgreSQL 连接池
		txmanager.ProviderSet,    // 事务管理器
		gcpubsub.ProviderSet,     // Pub/Sub 发布与订阅
		grpcserver.ProviderSet,   // gRPC Server
		// grpcclient.ProviderSet, // 暂时不使用, 未来需要调用外部 gRPC 服务时再启用
		// clients.ProviderSet,    // 暂时不使用, 未来需要调用外部服务时再启用
		repositories.ProviderSet, // 数据访问层（sqlc）
		wire.Bind(new(services.LifecycleRepo), new(*repositories.VideoRepository)),  // 写仓储绑定
		wire.Bind(new(services.VideoQueryRepo), new(*repositories.VideoRepository)), // 读仓储绑定
		wire.Bind(new(services.LifecycleOutboxWriter), new(*repositories.OutboxRepository)),
		services.ProviderSet,    // 业务逻辑层
		controllers.ProviderSet, // 控制器层（gRPC handlers）
		outboxtasks.ProvideRunner,
		engagementtasks.ProvideRunner,
		newApp, // 组装 Kratos 应用
	))
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 依赖注入详细文档
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// 以下列出所有 Provider 函数及其依赖关系，供理解完整依赖图使用。
// Wire 会自动按照类型依赖顺序调用这些 Provider，生成的代码见 wire_gen.go。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 1. 配置加载层 (configloader.ProviderSet)                                │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - configloader.ProvideBundle(configloader.Params) (*loader.Bundle, error)
//       基于传入的 Params 解析配置路径、执行 PGV 校验后返回 *loader.Bundle。
//
//   - configloader.ProvideServiceMetadata(*loader.Bundle) loader.ServiceMetadata
//       从 Bundle 提取服务元信息（名称/版本/环境/实例 ID）。
//
//   - configloader.ProvideBootstrap(*loader.Bundle) *configpb.Bootstrap
//       从 Bundle 提取 Server/Data 配置。
//
//   - configloader.ProvideServerConfig(*configpb.Bootstrap) *configpb.Server
//       提取 gRPC Server 配置。
//
//   - configloader.ProvideDataConfig(*configpb.Bootstrap) *configpb.Data
//       提取数据源配置（数据库/缓存等）。
//
//   - configloader.ProvideObservabilityConfig(*loader.Bundle) observability.ObservabilityConfig
//       提取标准化的可观测性配置。
//
//   - configloader.ProvideLoggerConfig(loader.ServiceMetadata) gclog.Config
//       由服务元信息生成 gclog 所需的 Config。
//
//   - configloader.ProvideObservabilityInfo(loader.ServiceMetadata) observability.ServiceInfo
//       将服务元信息转为 observability 使用的 ServiceInfo。
//
//   - configloader.ProvideJWTConfig(*configpb.Server, *configpb.Data) gcjwt.Config
//       提供 JWT 客户端/服务端配置。
//
//   - configloader.ProvideTxManagerConfig(*loader.Bundle) txmanager.Config
//       提取事务管理器配置。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 2. 日志层 (gclog.ProviderSet)                                           │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - gclog.NewComponent(gclog.Config) (*gclog.Component, func(), error)
//       初始化结构化日志组件，返回 cleanup 函数。
//
//   - gclog.ProvideLogger(*gclog.Component) log.Logger
//       从日志组件提取 trace-aware 的 log.Logger。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 3. 可观测性层 (observability.ProviderSet)                               │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - observability.NewComponent(context.Context, observability.ObservabilityConfig,
//                                  observability.ServiceInfo, log.Logger)
//                                  (*observability.Component, func(), error)
//       初始化 Tracer/Meter Provider，绑定 Service/Logger，并返回 cleanup。
//
//   - observability.ProvideMetricsConfig(observability.ObservabilityConfig)
//                                         *observability.MetricsConfig
//       提供 gRPC 指标配置（含默认值）。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 4. JWT 认证层 (gcjwt.ProviderSet)                                       │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - gcjwt.NewComponent(gcjwt.Config, log.Logger)
//                         (*gcjwt.Component, func(), error)
//       构建客户端/服务端 JWT 中间件组件。
//
//   - gcjwt.ProvideServerMiddleware(*gcjwt.Component)
//                                     (gcjwt.ServerMiddleware, error)
//       暴露服务端中间件供 gRPC Server 注入。
//
//   - gcjwt.ProvideClientMiddleware(*gcjwt.Component)
//                                     (gcjwt.ClientMiddleware, error)
//       暴露客户端中间件供 gRPC Client 注入。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 5. 数据库层 (pgxpoolx.ProviderSet)                                     │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - pgxpoolx.ProvideComponent(context.Context, pgxpoolx.Config, log.Logger)
//                             (*pgxpoolx.Component, func(), error)
//       构建 PostgreSQL 连接池组件。
//
//   - pgxpoolx.ProvidePool(*pgxpoolx.Component) *pgxpool.Pool
//       暴露 Pool 实例供仓储及事务管理器使用。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 6. 事务管理层 (txmanager.ProviderSet)                                   │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - txmanager.NewComponent(txmanager.Config, *pgxpool.Pool, log.Logger)
//                             (*txmanager.Component, func(), error)
//       组装事务管理器，复用日志与连接池。
//
//   - txmanager.ProvideManager(*txmanager.Component) txmanager.Manager
//       暴露事务管理接口供 Service 依赖。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 7. gRPC Server 层 (grpcserver.ProviderSet)                              │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - grpcserver.NewGRPCServer(*configpb.Server, *observability.MetricsConfig,
//                               gcjwt.ServerMiddleware, *controllers.VideoHandler,
//                               log.Logger) *grpc.Server
//       构建 gRPC Server，注入指标、日志、JWT 等中间件。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 8. 业务层 (repositories/services/controllers)                           │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - repositories.NewVideoRepository(*pgxpool.Pool, log.Logger)
//                                      *repositories.VideoRepository
//       构造视频仓储层，使用 sqlc 生成的查询方法。
//
//   - services.NewLifecycleWriter(services.LifecycleRepo, services.LifecycleOutboxWriter, txmanager.Manager, log.Logger)
//                               *services.LifecycleWriter
//   - services.NewRegisterUploadService(*services.LifecycleWriter) *services.RegisterUploadService
//   - services.NewOriginalMediaService(*services.LifecycleWriter, *repositories.VideoRepository)
//   - services.NewProcessingStatusService(*services.LifecycleWriter, *repositories.VideoRepository)
//   - services.NewMediaInfoService(*services.LifecycleWriter, *repositories.VideoRepository)
//   - services.NewAIAttributesService(*services.LifecycleWriter, *repositories.VideoRepository)
//   - services.NewVisibilityService(*services.LifecycleWriter, *repositories.VideoRepository)
//   - services.NewLifecycleService(*services.RegisterUploadService, *services.OriginalMediaService,
//       *services.ProcessingStatusService, *services.MediaInfoService, *services.AIAttributesService,
//       *services.VisibilityService) *services.LifecycleService
//   - services.NewVideoQueryService(services.VideoQueryRepo, txmanager.Manager, log.Logger)
//                               *services.VideoQueryService
//       组装视频业务用例，协调仓储访问及 Outbox 写入。
//       注: VideoRepo / OutboxRepo 接口通过 wire.Bind 绑定到对应 Repository 实现。
//
//   - controllers.NewLifecycleHandler(*services.LifecycleService, *controllers.BaseHandler) *controllers.LifecycleHandler
//   - controllers.NewVideoQueryHandler(*services.VideoQueryService, *controllers.BaseHandler) *controllers.VideoQueryHandler
//       构造视频控制层，为 gRPC handler 提供入口。
//
// ┌─────────────────────────────────────────────────────────────────────────┐
// │ 9. 应用层                                                                │
// └─────────────────────────────────────────────────────────────────────────┘
//
//   - newApp(*observability.Component, log.Logger, *grpc.Server,
//            configloader.ServiceMetadata) *kratos.App
//       将日志、观测组件、服务元信息和 gRPC Server 装配成 Kratos 应用。
