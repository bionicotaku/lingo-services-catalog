# Kratos Template - Supabase PostgreSQL 对接实施 TODO

> **目标:** 将 kratos-template 从模板代码完全替换为真实的 Supabase PostgreSQL 数据库访问，基于 pgx/v5 驱动。
>
> **预计时间:** 4-6 小时（分 3 个阶段）
>
> **当前状态:** 🟡 准备阶段

---

## 📋 总体任务清单

### 阶段 1：基础设施层（数据库连接）- 优先级 P0

- [ ] **1.1** 添加 pgx/v5 依赖
- [ ] **1.2** 清理无用配置并重构数据库配置结构
- [ ] **1.3** 实现 `infrastructure/database` 组件
- [ ] **1.4** 更新 Wire 依赖注入配置
- [ ] **1.5** 验证数据库连接与健康检查

### 阶段 2：数据访问层（Repository 实现）- 优先级 P0

- [ ] **2.1** 设计 Supabase 表结构与 Schema
- [ ] **2.2** 编写数据库迁移脚本
- [ ] **2.3** 更新 PO 模型（添加审计字段）
- [ ] **2.4** 实现 Repository 层（基于 pgx/v5）
- [ ] **2.5** 更新 Service 层业务逻辑
- [ ] **2.6** 验证完整的 CRUD 操作

### 阶段 3：测试与优化 - 优先级 P1

- [ ] **3.1** 编写集成测试（真实数据库）
- [ ] **3.2** 性能测试与连接池调优
- [ ] **3.3** 集成 OpenTelemetry（pgx tracing + 连接池指标）
- [ ] **3.4** 文档更新（仅在既有文档需要同步时）

---

## 🔐 环境配置与安全

### ⚠️ 敏感数据管理重要提示

**禁止在配置文件中硬编码密码！**

本项目使用 **环境变量** 管理敏感数据（数据库密码、API 密钥等）：

1. **配置文件（`configs/*.yaml`）** - 提交到 Git，包含占位符
   ```yaml
   dsn: ${DATABASE_URL:-postgresql://postgres:postgres@localhost:54322/postgres}
   ```

2. **环境变量文件（`.env`）** - 不提交到 Git，包含真实密钥
   ```bash
   DATABASE_URL=postgresql://postgres.xxxxx:RealPassword@...
   ```

3. **模板文件（`.env.example`）** - 提交到 Git，供团队参考
   ```bash
   DATABASE_URL=postgresql://postgres.xxxxx:[YOUR_PASSWORD]@...
   ```

### 📋 环境配置步骤

#### 1. 复制环境变量模板

```bash
cp configs/.env.example .env
```

#### 2. 编辑 .env 填入真实值

```bash
# 从 Supabase 控制台获取连接串
# Settings → Database → Connection string → Transaction pooler

vim .env
# 填入真实的 DATABASE_URL
```

#### 3. 加载环境变量

```bash
# 方式 1：手动导出
source .env

# 方式 2：运行时自动加载（如使用 dotenv）
export $(cat .env | xargs)

# 验证
echo $DATABASE_URL
```

**验收标准:**
- ✅ `.env` 文件存在且不被 Git 追踪
- ✅ `.env.example` 已提交到 Git
- ✅ `.gitignore` 包含 `.env` 规则
- ✅ 配置文件使用 `${DATABASE_URL}` 占位符

---

## 🎯 阶段 1：基础设施层（数据库连接）

### 任务 1.1：添加 pgx/v5 依赖

**执行命令:**
```bash
cd /Users/evan/Code/learning-app/back-end/kratos-template

# 添加 pgx/v5（含连接池子包）
go get github.com/jackc/pgx/v5@latest

# 清理依赖
go mod tidy

# 验证
go list -m all | grep jackc/pgx
```

**预期输出:**
```
github.com/jackc/pgx/v5 v5.x.x
```

**验收标准:**
- ✅ `go.mod` 包含 `github.com/jackc/pgx/v5`
- ✅ `go.sum` 已更新
- ✅ `go mod verify` 无错误

---

### 任务 1.2：清理无用配置（Redis、MySQL driver）

**文件修改清单:**

#### 1.2.1 更新 `conf.proto`

**文件:** `internal/infrastructure/config_loader/pb/conf.proto`

**操作:** 删除 `Data.Redis` 消息，简化 `Data.Database`

**修改前:**
```protobuf
message Data {
  message Database {
    string driver = 1;
    string source = 2;
  }
  message Redis {
    string network = 1;
    string addr = 2;
    google.protobuf.Duration read_timeout = 3;
    google.protobuf.Duration write_timeout = 4;
  }
  Database database = 1;
  Redis redis = 2;
  Client grpc_client = 3;
}
```

- ⚠️ 修改 proto 后务必同步更新 `loader.ProvideDataConfig` 返回类型、默认值解析逻辑（`defaults.go`）以及相关 PGV 校验，确保生成的 `conf.pb.go` 与 Wire Provider 使用的结构保持一致。

**修改后:**
```protobuf
message Data {
  // PostgreSQL 数据库配置（Supabase 专用）
  message PostgreSQL {
    // DSN 连接串（必填）
    string dsn = 1 [(validate.rules).string = {
      min_len: 1,
      pattern: "^postgres(ql)?://.*"
    }];

    // 连接池配置
    int32 max_open_conns = 2 [(validate.rules).int32 = {gte: 1, lte: 100}];
    int32 min_open_conns = 3 [(validate.rules).int32 = {gte: 0, lte: 50}];
    google.protobuf.Duration max_conn_lifetime = 4;
    google.protobuf.Duration max_conn_idle_time = 5;
    google.protobuf.Duration health_check_period = 6;

    // Supabase 特定配置
    string schema = 7;
    bool enable_prepared_statements = 8;
  }

  // gRPC Client 配置（可选）
  message Client {
    string target = 1;
  }

  PostgreSQL postgres = 1 [(validate.rules).message.required = true];
  Client grpc_client = 2;
}
```

**执行命令:**
```bash
# 重新生成 Proto 代码
make config

# 验证生成文件
ls -la internal/infrastructure/config_loader/pb/conf.pb.go
```

**验收标准:**
- ✅ `conf.pb.go` 包含 `PostgreSQL` 结构体
- ✅ `conf.pb.go` 不包含 `Redis` 结构体
- ✅ PGV 校验代码已生成（`conf.pb.validate.go`）

#### 1.2.2 更新 `config.yaml`

**文件:** `configs/config.yaml`

**修改前:**
```yaml
data:
  database:
    driver: mysql
    source: root:root@tcp(127.0.0.1:3306)/test?parseTime=True&loc=Local
  redis:
    addr: 127.0.0.1:6379
    read_timeout: 0.2s
    write_timeout: 0.2s
  grpc_client:
    target: dns:///127.0.0.1:9000
```

**修改后:**
```yaml
data:
  postgres:
    # Supabase DSN（使用环境变量）
    dsn: ${DATABASE_URL:-postgresql://postgres:postgres@localhost:54322/postgres?sslmode=disable&search_path=kratos_template}

    # 连接池配置
    max_open_conns: 10
    min_open_conns: 2
    max_conn_lifetime: 1h
    max_conn_idle_time: 30m
    health_check_period: 1m

    # Supabase 配置
    schema: kratos_template
    enable_prepared_statements: false

  # gRPC Client 配置（暂时留空，不启用）
  grpc_client:
    target: ""
```

- ℹ️ 如果使用 Supabase Pooler（默认 6543 端口），需要保持 `enable_prepared_statements: false`；直连 5432 端口时可按需开启。
- ⚠️ 同步更新 `config.instance-a.yaml`、`config.instance-b.yaml` 等示例文件，避免遗留旧字段。

**同时删除:** `config.instance-a.yaml` 和 `config.instance-b.yaml` 中的 Redis 配置

**验收标准:**
- ✅ 所有配置文件不包含 `redis` 字段
- ✅ `postgres.dsn` 符合 PostgreSQL 连接串格式
- ✅ 配置文件通过 YAML 语法检查

---

### 任务 1.3：实现 `infrastructure/database` 组件

**目录结构:**
```
internal/infrastructure/database/
├── database.go       # 连接池初始化 + 健康检查
├── init.go           # Wire ProviderSet
└── test/
    └── database_test.go  # 单元测试（可选）
```

> OpenTelemetry Tracer 将在阶段 3（任务 3.3）中补充，当前阶段只需保证连接池和健康检查稳定。

#### 1.3.1 创建 `database.go`

**文件:** `internal/infrastructure/database/database.go`

**内容:** （见下方完整代码）

**关键功能:**
1. 解析 DSN 并创建连接池
2. 应用连接池参数（max/min conns, timeouts）
3. 集成 Kratos Logger
4. 设置默认 Schema
5. 启动时健康检查（Ping + version 查询）
6. 可选的定期健康检查
7. 优雅关闭机制

#### 1.3.2 （预留）OpenTelemetry Tracer

- 在阶段 3 任务 3.3 中实现 `tracer.go`，当前阶段可仅创建空文件或跳过。

#### 1.3.3 创建 `init.go`

**文件:** `internal/infrastructure/database/init.go`

```go
package database

import "github.com/google/wire"

// ProviderSet 暴露数据库连接池构造器供 Wire 依赖注入。
var ProviderSet = wire.NewSet(
	NewPgxPool,
)
```

**执行命令:**
```bash
# 创建目录
mkdir -p internal/infrastructure/database/test

# 创建文件（使用编辑器或下方提供的完整代码）
# touch internal/infrastructure/database/{database.go,tracer.go,init.go}

# 验证编译
cd internal/infrastructure/database
go build .
```

**验收标准:**
- ✅ `database` 包可以独立编译
- ✅ 导出 `NewPgxPool` 函数
- ✅ 导出 `ProviderSet` 变量
- ✅ 静态检查通过

---

### 任务 1.4：更新 Wire 依赖注入配置

#### 1.4.1 更新 `wire.go`

**文件:** `cmd/grpc/wire.go`

**修改:** 在 `wire.Build` 中添加 `database.ProviderSet`

**修改前:**
```go
panic(wire.Build(
	configloader.ProviderSet,
	gclog.ProviderSet,
	obswire.ProviderSet,
	grpcserver.ProviderSet,
	grpcclient.ProviderSet,
	clients.ProviderSet,
	repositories.ProviderSet,
	services.ProviderSet,
	controllers.ProviderSet,
	newApp,
))
```

**修改后:**
```go
import (
	// ... 现有 import
	"github.com/bionicotaku/kratos-template/internal/infrastructure/database"
)

panic(wire.Build(
	configloader.ProviderSet,
	gclog.ProviderSet,
	obswire.ProviderSet,
	database.ProviderSet,        // ← 新增：数据库连接池
	grpcserver.ProviderSet,
	grpcclient.ProviderSet,
	clients.ProviderSet,
	repositories.ProviderSet,
	services.ProviderSet,
	controllers.ProviderSet,
	newApp,
))
```

#### 1.4.2 生成 Wire 代码

**执行命令:**
```bash
cd /Users/evan/Code/learning-app/back-end/kratos-template

# 生成 Wire 代码
wire ./cmd/grpc/...

# 验证生成结果
grep -A 10 "NewPgxPool" cmd/grpc/wire_gen.go
```

**预期输出:**
```go
pool, cleanup4, err := database.NewPgxPool(ctx, data, logger)
if err != nil {
	// ... cleanup
}
```

**验收标准:**
- ✅ `wire_gen.go` 包含 `database.NewPgxPool` 调用
- ✅ cleanup 顺序正确（数据库在 logger 之前关闭）
- ✅ 无 Wire 编译错误

---

### 任务 1.5：验证数据库连接

#### 1.5.1 准备 Supabase 环境

**选项 A: 使用真实 Supabase 项目**
```bash
# 在 Supabase 控制台创建项目，获取连接串
export DATABASE_URL="postgresql://postgres.xxxxx:[PASSWORD]@aws-0-us-west-1.pooler.supabase.com:6543/postgres?sslmode=require"
```

**选项 B: 使用本地 Supabase（Docker）**
```bash
# 安装 Supabase CLI
brew install supabase/tap/supabase

# 初始化本地项目
cd /Users/evan/Code/learning-app/back-end/kratos-template
supabase init

# 启动本地 Supabase
supabase start

# 获取本地 DSN
export DATABASE_URL=$(supabase status -o env | grep DATABASE_URL | cut -d'=' -f2)
```

#### 1.5.2 编译并运行

**执行命令:**
```bash
# 编译
make build

# 运行（使用环境变量）
./bin/grpc -conf configs/config.yaml
```

**预期日志输出:**
```json
{"level":"INFO","ts":"2025-01-22T10:00:00Z","msg":"database health check passed: PostgreSQL 15.1..."}
{"level":"INFO","ts":"2025-01-22T10:00:00Z","msg":"postgres pool created: dsn=postgresql://***..., max_conns=10, min_conns=2, schema=kratos_template"}
```

#### 1.5.3 验证连接池状态

**在服务运行时，另开终端执行:**
```bash
# 查询 Supabase 活跃连接
psql $DATABASE_URL -c "
SELECT
  COUNT(*) as total_connections,
  COUNT(*) FILTER (WHERE state = 'active') as active,
  COUNT(*) FILTER (WHERE state = 'idle') as idle
FROM pg_stat_activity
WHERE datname = 'postgres'
  AND application_name LIKE '%pgx%';
"
```

**预期输出:**
```
 total_connections | active | idle
-------------------+--------+------
                 2 |      0 |    2
```

**验收标准:**
- ✅ 服务成功启动，无连接错误
- ✅ 日志包含 "database health check passed"
- ✅ 数据库 `pg_stat_activity` 中至少存在一条来自本服务的连接，状态符合预期
- ✅ 服务关闭时日志包含 "postgres pool closed"

---

## 🎯 阶段 2：数据访问层（Repository 实现）

### 任务 2.1：设计 Supabase 表结构

#### 2.1.1 创建 Schema

**文件:** `migrations/001_create_schema.sql`

```sql
-- 创建服务专属 schema（数据主权）
CREATE SCHEMA IF NOT EXISTS kratos_template;

-- 设置默认搜索路径
ALTER DATABASE postgres SET search_path TO kratos_template, public;
```

**执行命令:**
```bash
# 方式 1：直接执行
psql $DATABASE_URL -f migrations/001_create_schema.sql

# 方式 2：使用 Supabase CLI
supabase db push --file migrations/001_create_schema.sql
```

#### 2.1.2 创建表结构

**文件:** `migrations/002_create_greetings_table.sql`

```sql
SET search_path TO kratos_template, public;

-- Greetings 表（替换原 Greeter 实体）
CREATE TABLE IF NOT EXISTS greetings (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    message       TEXT NOT NULL,

    -- 审计字段（符合可演进原则）
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ,  -- 软删除

    -- 约束
    CONSTRAINT name_length_check CHECK (char_length(name) BETWEEN 1 AND 64),
    CONSTRAINT message_not_empty CHECK (char_length(message) > 0)
);

-- 索引
CREATE INDEX idx_greetings_name ON greetings(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_greetings_created_at ON greetings(created_at DESC);

-- 自动更新 updated_at 触发器
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_greetings_updated_at
    BEFORE UPDATE ON greetings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- RLS 策略（行级安全）
ALTER TABLE greetings ENABLE ROW LEVEL SECURITY;

-- 允许服务角色全量访问
CREATE POLICY service_role_all_access ON greetings
    FOR ALL
    TO service_role
    USING (true)
    WITH CHECK (true);

-- 允许匿名用户只读（如果需要）
CREATE POLICY anon_read_only ON greetings
    FOR SELECT
    TO anon
    USING (deleted_at IS NULL);
```

**执行命令:**
```bash
psql $DATABASE_URL -f migrations/002_create_greetings_table.sql

# 验证表结构
psql $DATABASE_URL -c "\d kratos_template.greetings"
```

**预期输出:**
```
                                      Table "kratos_template.greetings"
   Column   |           Type           | Collation | Nullable |                Default
------------+--------------------------+-----------+----------+---------------------------------------
 id         | bigint                   |           | not null | nextval('greetings_id_seq'::regclass)
 name       | text                     |           | not null |
 message    | text                     |           | not null |
 created_at | timestamp with time zone |           | not null | now()
 updated_at | timestamp with time zone |           | not null | now()
 deleted_at | timestamp with time zone |           |          |
```

**验收标准:**
- ✅ Schema `kratos_template` 已创建
- ✅ Table `greetings` 已创建，包含所有字段
- ✅ 索引已创建
- ✅ 触发器已创建
- ✅ RLS 策略已启用

---

### 任务 2.2：更新 PO 模型

**文件:** `internal/models/po/greeter.go`

**修改前:**
```go
package po

type Greeter struct {
	Hello string
}
```

**修改后:**
```go
package po

import "time"

// Greeting 表示 kratos_template.greetings 表的数据库实体。
// 映射字段：id, name, message, created_at, updated_at, deleted_at
type Greeting struct {
	ID        int64      `db:"id"`
	Name      string     `db:"name"`
	Message   string     `db:"message"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt time.Time  `db:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at"`  // 指针类型，支持 NULL
}
```

**验收标准:**
- ✅ 结构体字段与数据库表列完全对应
- ✅ 使用 `db` tag 标注字段映射
- ✅ `DeletedAt` 为指针类型（支持 NULL）

---

### 任务 2.3：实现 Repository 层

**文件:** `internal/repositories/greeter.go`

**完整实现见下方代码块。**

**关键功能:**
1. 注入 `*pgxpool.Pool` 连接池
2. 实现 `services.GreeterRepo` 接口的所有方法
3. 所有方法接收 `context.Context`
4. 错误包装保留根因 `fmt.Errorf("...: %w", err)`
5. `FindByID` 查询不到时返回 `services.ErrUserNotFound`
6. 软删除支持（`WHERE deleted_at IS NULL`）
7. 分页限制（`LIMIT 100`）

**执行命令:**
```bash
# 编译验证
cd internal/repositories
go build .

# 静态检查
cd /Users/evan/Code/learning-app/back-end/kratos-template
make lint
```

**验收标准:**
- ✅ `NewGreeterRepo` 接收 `*pgxpool.Pool` 参数
- ✅ 实现 `Save/Update/FindByID/ListByHello/ListAll` 方法
- ✅ 所有方法正确处理错误
- ✅ SQL 语句正确（使用参数化查询）

---

### 任务 2.4：更新 Service 层

**文件:** `internal/services/greeter.go`

**修改点:**

**修改前（第 48-66 行）:**
```go
func (uc *GreeterUsecase) CreateGreeting(ctx context.Context, name string) (*vo.Greeting, error) {
	entity := &po.Greeter{Hello: name}
	saved, err := uc.repo.Save(ctx, entity)
	if err != nil {
		return nil, err
	}

	message := "Hello " + saved.Hello
	uc.log.WithContext(ctx).Infof("CreateGreeting: %s", message)
	return &vo.Greeting{Message: message}, nil
}
```

**修改后:**
```go
func (uc *GreeterUsecase) CreateGreeting(ctx context.Context, name string) (*vo.Greeting, error) {
	// 构造 Greeting 实体
	entity := &po.Greeting{
		Name:    name,
		Message: "Hello " + name,
	}

	// 保存到数据库
	saved, err := uc.repo.Save(ctx, entity)
	if err != nil {
		return nil, fmt.Errorf("save greeting: %w", err)
	}

	uc.log.WithContext(ctx).Infof("CreateGreeting: id=%d, name=%s", saved.ID, saved.Name)
	return &vo.Greeting{Message: saved.Message}, nil
}
```

**验收标准:**
- ✅ 使用 `po.Greeting` 而非 `po.Greeter`
- ✅ 设置 `Name` 和 `Message` 字段
- ✅ 日志输出包含数据库 ID

---

### 任务 2.5：端到端验证

#### 2.5.1 重新编译

```bash
cd /Users/evan/Code/learning-app/back-end/kratos-template

# 重新生成 Wire 代码（如果 Repository 签名变化）
wire ./cmd/grpc/...

# 编译
make build
```

#### 2.5.2 运行服务

```bash
./bin/grpc -conf configs/config.yaml
```

#### 2.5.3 调用 gRPC 方法

**使用 grpcurl:**
```bash
# 安装 grpcurl（如果未安装）
brew install grpcurl

# 调用 SayHello（会触发数据库 INSERT）
grpcurl -plaintext -d '{"name": "Alice"}' localhost:9000 helloworld.v1.Greeter/SayHello
```

**预期响应:**
```json
{
  "message": "Hello Alice"
}
```

#### 2.5.4 验证数据库

```bash
# 查询数据库，确认数据已写入
psql $DATABASE_URL -c "
SELECT id, name, message, created_at
FROM kratos_template.greetings
ORDER BY created_at DESC
LIMIT 5;
"
```

**预期输出:**
```
 id | name  |   message   |         created_at
----+-------+-------------+----------------------------
  1 | Alice | Hello Alice | 2025-01-22 10:30:00.123+00
```

#### 2.5.5 （预留）验证追踪

- OpenTelemetry 追踪将在阶段 3 任务 3.3 中完成，本阶段可先跳过该步骤。
- 仍需确认 gRPC 调用成功、数据库存在新记录、响应消息正确。

---

## 🎯 阶段 3：测试与优化

### 任务 3.1：编写集成测试

**文件:** `internal/repositories/test/greeter_integration_test.go`

```go
//go:build integration

package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/bionicotaku/kratos-template/internal/infrastructure/database"
	configpb "github.com/bionicotaku/kratos-template/internal/infrastructure/config_loader/pb"
	"github.com/bionicotaku/kratos-template/internal/models/po"
	"github.com/bionicotaku/kratos-template/internal/repositories"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestGreeterRepo_Save(t *testing.T) {
	// 1. 创建测试数据库连接
	cfg := &configpb.Data{
		Postgres: &configpb.Data_PostgreSQL{
			Dsn:              "postgresql://postgres:postgres@localhost:54322/postgres?sslmode=disable&search_path=kratos_template",
			MaxOpenConns:     5,
			MinOpenConns:     1,
			MaxConnLifetime:  durationpb.New(time.Hour),
			MaxConnIdleTime:  durationpb.New(30 * time.Minute),
			Schema:           "kratos_template",
			EnablePreparedStatements: true,
		},
	}

	pool, cleanup, err := database.NewPgxPool(context.Background(), cfg, log.DefaultLogger)
	require.NoError(t, err)
	defer cleanup()

	// 2. 创建 Repository
	repo := repositories.NewGreeterRepo(pool, log.DefaultLogger)

	// 3. 测试保存
	greeting := &po.Greeting{
		Name:    "test_user",
		Message: "Hello test_user",
	}

	saved, err := repo.Save(context.Background(), greeting)
	require.NoError(t, err)
	assert.NotZero(t, saved.ID)
	assert.Equal(t, "test_user", saved.Name)
	assert.NotZero(t, saved.CreatedAt)

	// 4. 测试查询
	found, err := repo.FindByID(context.Background(), saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, found.ID)
	assert.Equal(t, "test_user", found.Name)
}
```

**运行测试:**
```bash
# 启动本地 Supabase
supabase start

# 运行集成测试
go test -tags=integration ./internal/repositories/test/... -v
```

**验收标准:**
- ✅ 测试通过
- ✅ 数据正确写入和读取
- ✅ 无连接泄漏

---

### 任务 3.2：性能测试与调优

**创建基准测试:**

**文件:** `internal/repositories/test/greeter_bench_test.go`

```go
package repositories_test

import (
	"context"
	"testing"

	// ... imports
)

func BenchmarkGreeterRepo_Save(b *testing.B) {
	// ... 初始化连接池

	repo := repositories.NewGreeterRepo(pool, log.DefaultLogger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		greeting := &po.Greeting{
			Name:    "bench_user",
			Message: "Hello bench_user",
		}
		_, err := repo.Save(context.Background(), greeting)
		if err != nil {
			b.Fatal(err)
		}
	}
}
```

**运行基准测试:**
```bash
go test -bench=. -benchmem ./internal/repositories/test/... -run=^$
```

**预期输出:**
```
BenchmarkGreeterRepo_Save-8   	    5000	    250000 ns/op	    1024 B/op	      20 allocs/op
```

**调优参考:**
- 如果 QPS < 1000，考虑增加 `max_open_conns`
- 如果内存占用过高，考虑减小 `min_open_conns`
- 如果延迟 > 100ms，检查网络或 Supabase 区域

---

### 任务 3.3：OpenTelemetry 集成

#### 3.3.1 实现查询 Tracer

**文件:** `internal/infrastructure/database/tracer.go`

```go
package database

import (
    "context"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/tracelog"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

type otelTracer struct{}

func (t *otelTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data tracelog.TraceQueryStartData) context.Context {
    tracer := otel.Tracer("kratos-template/database")
    ctx, span := tracer.Start(ctx, "db.query", trace.WithSpanKind(trace.SpanKindClient))
    span.SetAttributes(attribute.String("db.statement", data.SQL))
    span.SetAttributes(attribute.String("db.system", "postgresql"))
    return context.WithValue(ctx, spanKey{}, span)
}

func (t *otelTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data tracelog.TraceQueryEndData) {
    span, ok := ctx.Value(spanKey{}).(trace.Span)
    if !ok {
        return
    }
    if data.Err != nil {
        span.RecordError(data.Err)
        span.SetStatus(codes.Error, data.Err.Error())
    }
    span.End()
}

type spanKey struct{}

func NewQueryTracer() pgx.QueryTracer {
    return &otelTracer{}
}
```

**集成步骤:**

1. 在 `database.go` 中的 `pgxpool.Config` 初始化后添加：
   ```go
   cfg.ConnConfig.Tracer = NewQueryTracer()
   ```
2. 确保 `enable_prepared_statements` 为 `false` 时也能记录语句，可根据需要截断/脱敏 SQL。

#### 3.3.2 注册连接池指标

**文件:** `internal/infrastructure/database/metrics.go`

```go
package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// RegisterPoolMetrics 注册连接池指标到 OpenTelemetry。
func RegisterPoolMetrics(pool *pgxpool.Pool) error {
	meter := otel.Meter("kratos-template/database")

	// 最大连接数
	maxConns, err := meter.Int64ObservableGauge("db.pool.max_conns")
	if err != nil {
		return err
	}

	// 当前活跃连接数
	activeConns, err := meter.Int64ObservableGauge("db.pool.active_conns")
	if err != nil {
		return err
	}

	// 空闲连接数
	idleConns, err := meter.Int64ObservableGauge("db.pool.idle_conns")
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		stat := pool.Stat()
		o.ObserveInt64(maxConns, int64(stat.MaxConns()))
		o.ObserveInt64(activeConns, int64(stat.AcquiredConns()))
		o.ObserveInt64(idleConns, int64(stat.IdleConns()))
		return nil
	}, maxConns, activeConns, idleConns)

	return err
}
```

---

### 任务 3.4：文档更新（按需）

> 注意：遵循仓库规范，优先维护既有文档，未获批准不要新增新的 Markdown 文档。

#### 3.4.1 更新 README.md（如已有相关章节）

- 核对 README 中的数据库配置说明，如未覆盖 Supabase/pgx 使用，可追加精简示例（示例连接串、环境变量、迁移命令）。
- 保持与 `configs/config.yaml` 中的字段命名一致，避免重复或矛盾描述。

#### 3.4.2 同步其它文档（可选）

- 若仓库已有 `docs/database.md` 等资料，可增补连接池参数、常见问题等；若无现成文档，则在 TODO 中记录后续需求，暂不新建文件。
- 变更后运行 `make lint` 确保文档引用的示例命令与配置有效。

---

## ✅ 验收总清单

### 阶段 1 验收

- [ ] `go.mod` 包含 `github.com/jackc/pgx/v5`
- [ ] `conf.proto` 不包含 `Redis` 配置
- [ ] `config.yaml` 包含 `postgres` 配置示例
- [ ] `infrastructure/database` 包已创建
- [ ] Wire 生成代码包含 `NewPgxPool` 调用
- [ ] 服务启动成功，日志显示 "database health check passed"
- [ ] 数据库中可观测到来自服务的连接（通过 `pg_stat_activity`）

### 阶段 2 验收

- [ ] Supabase schema `kratos_template` 已创建
- [ ] Table `greetings` 已创建，包含所有字段和索引
- [ ] `po.Greeting` 模型已更新
- [ ] Repository 所有方法已实现
- [ ] gRPC 调用成功写入数据库
- [ ] 数据库可查询到写入的记录

### 阶段 3 验收

- [ ] 集成测试通过
- [ ] 基准测试达到目标（示例阈值 QPS ≥ 500）
- [ ] OpenTelemetry 追踪与连接池指标正常采集
- [ ] README／现有文档已按需同步

---

## 🚨 常见问题排查

### 问题 1: `prepared statement does not exist`

**原因:** Supabase Pooler 模式不支持 prepared statements

**解决:**
```yaml
data:
  postgres:
    enable_prepared_statements: false  # ← 必须禁用
```

### 问题 2: `connection refused`

**原因:** DSN 配置错误或网络问题

**排查步骤:**
```bash
# 1. 测试连接
psql "$DATABASE_URL" -c "SELECT version();"

# 2. 检查端口
# Pooler: 6543
# Direct: 5432

# 3. 检查 SSL 模式
# 生产: sslmode=require
# 本地: sslmode=disable
```

### 问题 3: `too many connections`

**原因:** 连接数超过 Supabase 限制（免费版 60 个）

**解决:**
```yaml
data:
  postgres:
    max_open_conns: 5  # ← 降低连接数
```

### 问题 4: `relation does not exist`

**原因:** Schema 或表未创建

**排查步骤:**
```bash
# 检查 schema
psql $DATABASE_URL -c "\dn"

# 检查表
psql $DATABASE_URL -c "\dt kratos_template.*"

# 重新运行迁移
psql $DATABASE_URL -f migrations/002_create_greetings_table.sql
```

---

## 📚 参考资料

- [pgx 官方文档](https://pkg.go.dev/github.com/jackc/pgx/v5)
- [Supabase 数据库文档](https://supabase.com/docs/guides/database)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)
- [Wire 依赖注入指南](https://github.com/google/wire/blob/main/docs/guide.md)

---

## 📝 进度追踪

**最后更新:** 2025-01-22

| 阶段 | 状态 | 完成时间 | 备注 |
|------|------|----------|------|
| 阶段 1：基础设施层 | 🟡 进行中 | - | 数据库连接层 |
| 阶段 2：数据访问层 | ⚪ 待开始 | - | Repository 实现 |
| 阶段 3：测试与优化 | ⚪ 待开始 | - | 测试与文档 |

---

**下一步行动:**
1. 执行 `go get` 添加 pgx 依赖
2. 修改 `conf.proto` 和 `config.yaml`
3. 实现 `infrastructure/database` 组件

**需要帮助?**
- 查看 `docs/database.md` 详细文档
- 或在项目 issue 中提问
