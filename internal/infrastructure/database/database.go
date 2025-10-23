// Package database 负责 PostgreSQL（Supabase）连接池的初始化与生命周期管理。
// 包括：连接池配置、健康检查、优雅关闭等功能。
package database

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPgxPool 创建并配置 pgxpool.Pool（PostgreSQL 连接池）。
//
// 职责：
//  1. 解析 DSN 并创建连接池配置
//  2. 应用连接池参数（最大/最小连接数、超时）
//  3. 集成 Kratos Logger（INFO/WARN/ERROR 级别日志）
//  4. 设置默认 Schema（search_path）
//  5. 启动时健康检查（Ping + 版本查询）
//  6. 返回清理函数（用于 Wire cleanup）
//
// 参数：
//   - ctx: 用于初始化和健康检查的上下文
//   - c: Data 配置（从 conf.proto 生成，包含 PostgreSQL 配置）
//   - logger: Kratos 日志实例
//
// 返回：
//   - *pgxpool.Pool: 可用的连接池实例
//   - func(): cleanup 函数，关闭连接池（Wire 会自动调用）
//   - error: 初始化或健康检查失败时返回错误
//
// 使用示例：
//
//	pool, cleanup, err := database.NewPgxPool(ctx, dataConfig, logger)
//	if err != nil {
//	    return nil, fmt.Errorf("failed to initialize database: %w", err)
//	}
//	defer cleanup()
func NewPgxPool(ctx context.Context, c *configpb.Data, logger log.Logger) (*pgxpool.Pool, func(), error) {
	helper := log.NewHelper(logger)

	pgCfg := c.GetPostgres()
	if pgCfg == nil {
		return nil, nil, fmt.Errorf("postgres configuration is required")
	}

	// 1. 解析 DSN
	dsn := pgCfg.GetDsn()
	if dsn == "" {
		return nil, nil, fmt.Errorf("postgres DSN is required (set DATABASE_URL)")
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse postgres DSN: %w", err)
	}

	// 2. 应用连接池参数
	if pgCfg.GetMaxOpenConns() > 0 {
		poolConfig.MaxConns = pgCfg.GetMaxOpenConns()
	}
	if pgCfg.GetMinOpenConns() >= 0 {
		poolConfig.MinConns = pgCfg.GetMinOpenConns()
	}
	if pgCfg.GetMaxConnLifetime() != nil {
		poolConfig.MaxConnLifetime = pgCfg.GetMaxConnLifetime().AsDuration()
	}
	if pgCfg.GetMaxConnIdleTime() != nil {
		poolConfig.MaxConnIdleTime = pgCfg.GetMaxConnIdleTime().AsDuration()
	}
	if pgCfg.GetHealthCheckPeriod() != nil {
		poolConfig.HealthCheckPeriod = pgCfg.GetHealthCheckPeriod().AsDuration()
	}

	// 3. 集成 Kratos Logger（映射 pgx 日志级别到 Kratos 日志）
	poolConfig.ConnConfig.Tracer = &pgxLogger{helper: helper}

	// 4. 设置默认 Schema（如果配置中指定）
	if schema := pgCfg.GetSchema(); schema != "" {
		// 确保 search_path 包含配置的 schema
		// 优先级：配置 schema > DSN 中的 search_path
		poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s, public", schema))
			if err != nil {
				return fmt.Errorf("failed to set search_path: %w", err)
			}
			return nil
		}
	}

	// 5. 禁用/启用 Prepared Statements（根据配置）
	if !pgCfg.GetEnablePreparedStatements() {
		// Supabase Pooler 模式必须禁用 prepared statements
		poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	// 6. 创建连接池
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}

	// 7. 启动时健康检查
	if err := healthCheck(ctx, pool, helper); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("postgres health check failed: %w", err)
	}

	// 8. 日志输出连接池摘要（脱敏 DSN）
	helper.Infof(
		"postgres pool created: dsn=%s max_conns=%d min_conns=%d schema=%s prepared_statements=%v",
		sanitizeDSN(dsn),
		poolConfig.MaxConns,
		poolConfig.MinConns,
		pgCfg.GetSchema(),
		pgCfg.GetEnablePreparedStatements(),
	)

	// 9. 返回 cleanup 函数（优雅关闭）
	cleanup := func() {
		helper.Info("closing postgres pool")
		pool.Close()
	}

	return pool, cleanup, nil
}

// healthCheck 执行数据库健康检查。
//
// 检查项：
//  1. Ping 连接（验证连接可达性）
//  2. 查询 PostgreSQL 版本（验证可执行 SQL）
//
// 失败时返回错误，不会 panic。
func healthCheck(ctx context.Context, pool *pgxpool.Pool, helper *log.Helper) error {
	// 超时上下文（避免健康检查长时间阻塞启动）
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 1. Ping 检查
	if err := pool.Ping(healthCtx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// 2. 版本查询检查
	var version string
	err := pool.QueryRow(healthCtx, "SELECT version()").Scan(&version)
	if err != nil {
		return fmt.Errorf("version query failed: %w", err)
	}

	// 记录成功日志
	helper.Infof(
		"database health check passed: version=%s",
		truncateVersion(version),
	)

	return nil
}

// sanitizeDSN 对 DSN 进行脱敏处理，隐藏密码。
//
// 示例：
//
//	输入: postgresql://user:secret@host:5432/db
//	输出: postgresql://user:***@host:5432/db
func sanitizeDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}

	if parsed.User != nil {
		username := parsed.User.Username()
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(username, "***")
		}
	}

	return parsed.String()
}

// truncateVersion 截断 PostgreSQL 版本字符串（避免日志过长）。
//
// 示例：
//
//	输入: PostgreSQL 15.1 (Ubuntu 15.1-1.pgdg22.04+1) on x86_64-pc-linux-gnu...
//	输出: PostgreSQL 15.1
func truncateVersion(version string) string {
	// 只保留第一个括号前的内容
	if idx := strings.Index(version, "("); idx != -1 {
		return strings.TrimSpace(version[:idx])
	}
	// 超过 100 字符则截断
	if len(version) > 100 {
		return version[:100] + "..."
	}
	return version
}

// pgxLogger 是 pgx.QueryTracer 的实现，用于将 pgx 日志转发到 Kratos Logger。
//
// 日志级别映射：
//   - pgx.LogLevelError → Kratos ERROR
//   - pgx.LogLevelWarn  → Kratos WARN
//   - pgx.LogLevelInfo  → Kratos INFO
//   - 其他级别不记录（避免日志噪音）
type pgxLogger struct {
	helper *log.Helper
}

// TraceQueryStart 实现 pgx.QueryTracer 接口（查询开始时调用）。
// 本项目暂不记录查询开始日志，避免过多噪音。
func (l *pgxLogger) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}

// TraceQueryEnd 实现 pgx.QueryTracer 接口（查询结束时调用）。
// 仅在查询失败时记录错误日志。
func (l *pgxLogger) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if data.Err != nil {
		// 记录查询错误（仅包含错误信息，不记录 SQL 以避免敏感数据泄露）
		l.helper.Errorf(
			"postgres query failed: error=%v command_tag=%s",
			data.Err,
			data.CommandTag.String(),
		)
	}
}
