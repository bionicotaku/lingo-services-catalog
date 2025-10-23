# 测试数据 (Test Fixtures)

本目录包含集成测试所需的测试数据和说明文档。

## 文件说明

- **`seed_test_videos.sql`** - 测试视频数据脚本（14 条记录）
- **`SEED_DATA.md`** - 详细的测试数据说明文档

## 快速开始

### 1. 初始化测试数据

在首次运行集成测试前，需要执行一次数据初始化：

```bash
# 从项目根目录执行
source configs/.env
psql "$DATABASE_URL" -f test/fixtures/seed_test_videos.sql
```

### 2. 验证数据

```bash
source configs/.env
psql "$DATABASE_URL" -c "SELECT COUNT(*) FROM catalog.videos WHERE upload_user_id = 'f0ad5a16-0d50-4f94-8ff7-b99dda13ee47';"
```

应该返回 `14` 条记录。

### 3. 运行集成测试

```bash
go test ./test/integration/... -v
```

## 数据覆盖

测试数据涵盖视频生命周期的所有状态：

| 状态 | 数量 | 用途 |
|------|------|------|
| `published` | 4 | 测试正常查询、列表、筛选 |
| `ready` | 2 | 测试待发布状态 |
| `processing` | 2 | 测试处理中状态 |
| `pending_upload` | 2 | 测试上传流程 |
| `failed` | 2 | 测试错误处理 |
| `rejected` | 1 | 测试审核流程 |
| `archived` | 1 | 测试归档查询 |

详细说明请参考 [`SEED_DATA.md`](./SEED_DATA.md)。

## 重置测试数据

如果测试数据被修改或损坏，可以重新执行初始化脚本：

```bash
# 清空现有数据
source configs/.env
psql "$DATABASE_URL" -c "DELETE FROM catalog.videos WHERE upload_user_id = 'f0ad5a16-0d50-4f94-8ff7-b99dda13ee47';"

# 重新插入
psql "$DATABASE_URL" -f test/fixtures/seed_test_videos.sql
```

## 注意事项

⚠️ **这些测试数据仅用于开发和测试环境**，不应在生产环境中使用。

⚠️ 集成测试依赖这些固定的 video_id，请勿手动修改数据库中的测试记录。

⚠️ 测试用户 `test@test.com` 的 ID 为 `f0ad5a16-0d50-4f94-8ff7-b99dda13ee47`。
