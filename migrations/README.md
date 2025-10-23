# Database Migrations

本目录包含 kratos-template 服务的数据库迁移脚本。

## 执行顺序

迁移按文件名数字顺序执行：

1. `001_create_catalog_schema.sql` - 创建 catalog schema、videos 表、索引、触发器

## 执行方式

### 方式 1: 使用 psql 直接执行

```bash
# 加载环境变量
export $(cat configs/.env | xargs)

# 执行迁移
psql "$DATABASE_URL" -f migrations/001_create_catalog_schema.sql
```

### 方式 2: 使用 Supabase CLI

```bash
# 本地开发环境
supabase db reset

# 生产环境
supabase db push --file migrations/001_create_catalog_schema.sql
```

## 验证迁移

```bash
# 验证 schema 创建
psql "$DATABASE_URL" -c "\dn catalog"

# 验证表结构
psql "$DATABASE_URL" -c "\d catalog.videos"

# 验证索引
psql "$DATABASE_URL" -c "\di catalog.*"

# 验证枚举类型
psql "$DATABASE_URL" -c "\dT catalog.*"
```

## 回滚策略

当前迁移脚本使用 `IF NOT EXISTS` 保证幂等性，可安全重复执行。

如需回滚：

```sql
-- 删除表（会级联删除索引和触发器）
DROP TABLE IF EXISTS catalog.videos CASCADE;

-- 删除枚举类型
DROP TYPE IF EXISTS catalog.video_status CASCADE;
DROP TYPE IF EXISTS catalog.stage_status CASCADE;

-- 删除触发器函数
DROP FUNCTION IF EXISTS catalog.tg_set_updated_at() CASCADE;

-- 删除 schema（仅当完全清理时）
DROP SCHEMA IF EXISTS catalog CASCADE;
```

## 注意事项

1. **生产环境**: 迁移前务必备份数据库
2. **外键约束**: videos 表依赖 `auth.users`，确保 Supabase Auth 已启用
3. **幂等性**: 所有脚本支持重复执行
4. **版本控制**: 迁移脚本提交到 Git，不可修改已执行的脚本
