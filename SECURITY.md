# 安全配置指南

> **重要提示:** 本文档说明如何安全地管理敏感数据（数据库密码、API 密钥等）

---

## 🔐 敏感数据管理原则

### ❌ 禁止的做法

1. **硬编码密码到配置文件**
   ```yaml
   # ❌ 错误示例（会提交到 Git）
   data:
     postgres:
       dsn: postgresql://user:MyRealPassword@db.supabase.co:5432/postgres
   ```

2. **在注释中包含密码**
   ```yaml
   # ❌ 错误示例
   # 生产密码: RealPassword123
   dsn: ${DATABASE_URL}
   ```

3. **提交 .env 文件到 Git**
   ```bash
   # ❌ 错误操作
   git add .env
   ```

### ✅ 正确的做法

1. **使用环境变量占位符**
   ```yaml
   # ✅ 正确示例（配置文件）
   data:
     postgres:
       dsn: ${DATABASE_URL:-postgresql://postgres:postgres@localhost:54322/postgres}
   ```

2. **真实密码存储在 .env 文件**
   ```bash
   # ✅ 正确示例（.env 文件，不提交）
   DATABASE_URL=postgresql://postgres:RealPassword@db.supabase.co:5432/postgres
   ```

3. **提供 .env.example 模板**
   ```bash
   # ✅ 正确示例（.env.example，提交到 Git）
   DATABASE_URL=postgresql://postgres:[YOUR_PASSWORD]@db.supabase.co:5432/postgres
   ```

---

## 📁 文件清单

### 已提交到 Git（安全）

| 文件 | 说明 | 包含密码？ |
|------|------|-----------|
| `configs/.env.example` | 环境变量模板 | ❌ 占位符 |
| `configs/config.yaml` | 基础配置 | ❌ 使用 `${DATABASE_URL}` |
| `configs/config.*.yaml` | 示例配置 | ❌ 占位符或本地默认值 |
| `.gitignore` | Git 忽略规则 | N/A |
| `TODO.md` | 任务清单 | ❌ 仅有占位符 |
| `SECURITY.md` | 本文档 | ❌ 安全指南 |
| `configs/README.md` | 配置说明 | ❌ 使用指南 |

### 不提交到 Git（被忽略）

| 文件/模式 | 说明 | 原因 |
|----------|------|------|
| `.env` | 真实环境变量 | ✅ 包含密码 |
| `.env.local` | 本地环境变量 | ✅ 包含密码 |
| `.env.production` | 生产环境变量 | ✅ 包含密码 |
| `configs/*.secret.yaml` | 包含密钥的配置 | ✅ 包含密码 |
| `configs/*.local.yaml` | 个人本地配置 | ✅ 可能包含密码 |
| `*.key`, `*.pem`, `*.cert` | 证书和密钥文件 | ✅ 敏感凭据 |
| `*-service-account.json` | 云服务凭据 | ✅ 敏感凭据 |

---

## 🚀 快速开始

### 1. 初始化环境配置

```bash
cd /Users/evan/Code/learning-app/back-end/kratos-template

# 复制环境变量模板
cp configs/.env.example .env

# 编辑 .env，填入真实密码
vim .env
```

### 2. 从 Supabase 获取连接串

1. 登录 [Supabase 控制台](https://app.supabase.com)
2. 选择项目 → Settings → Database
3. 复制 **Connection string** (Transaction pooler 模式)
4. 粘贴到 `.env` 文件

### 3. 验证配置

```bash
# 加载环境变量
source .env

# 验证
echo $DATABASE_URL
# 应输出：postgresql://postgres.xxxxx:RealPassword@...

# 运行服务
./bin/grpc -conf configs/config.yaml
```

---

## 🔍 安全检查

### 提交前检查

```bash
# 1. 检查暂存区是否有硬编码密码
git diff --cached | grep -iE "(password|secret|token).*:.*[^[]"

# 2. 验证 .env 被忽略
git check-ignore .env
# 应输出：.env

# 3. 验证 .env.example 可以提交
git check-ignore configs/.env.example
# 应无输出（返回错误）

# 4. 检查配置文件是否有密码
grep -r "password.*:.*[^[]" configs/ --exclude="*.example" --exclude="README.md"
# 应无输出
```

### 如果已经提交密码

**⚠️ 立即执行以下步骤：**

```bash
# 1. 从 Git 历史中删除敏感文件
git filter-branch --force --index-filter \
  "git rm --cached --ignore-unmatch .env" \
  --prune-empty --tag-name-filter cat -- --all

# 2. 强制推送（⚠️ 需团队协调）
git push origin --force --all

# 3. 立即更换泄露的密码
# 登录 Supabase → Database → Reset password
```

**更重要的是：**
- ✅ 立即在 Supabase 重置数据库密码
- ✅ 检查访问日志是否有异常
- ✅ 通知团队成员同步仓库

---

## 🌍 环境变量管理

### 本地开发

```bash
# .env（本地机器，不提交）
DATABASE_URL=postgresql://postgres:postgres@localhost:54322/postgres?sslmode=disable
```

### CI/CD 环境

**GitHub Actions:**
```yaml
# .github/workflows/deploy.yml
env:
  DATABASE_URL: ${{ secrets.DATABASE_URL }}
```

**配置步骤:**
1. 仓库设置 → Secrets and variables → Actions
2. 添加 `DATABASE_URL` 密钥

**GitLab CI:**
```yaml
# .gitlab-ci.yml
variables:
  DATABASE_URL: $DATABASE_URL
```

**配置步骤:**
1. 项目设置 → CI/CD → Variables
2. 添加 `DATABASE_URL` 变量（勾选 Masked）

### 生产环境

**Docker/Kubernetes:**
```yaml
# docker-compose.yml
services:
  app:
    environment:
      - DATABASE_URL=${DATABASE_URL}
```

```bash
# 运行时传递
docker run -e DATABASE_URL="postgresql://..." app:latest
```

**云平台:**
- **AWS:** Systems Manager Parameter Store / Secrets Manager
- **GCP:** Secret Manager
- **Azure:** Key Vault
- **Vercel/Netlify:** Environment Variables 面板

---

## 📚 相关文档

- [configs/README.md](configs/README.md) - 配置文件使用说明
- [TODO.md](TODO.md) - 实施任务清单
- [12-Factor App: Config](https://12factor.net/config)
- [OWASP Top 10](https://owasp.org/www-project-top-ten/)

---

## 🆘 常见问题

### Q1: 为什么不能直接写密码到配置文件？

**A:**
1. Git 历史永久保存（即使删除文件）
2. 配置文件经常被分享、复制、截图
3. 任何能读代码的人都能看到密码
4. 违反安全合规要求

### Q2: 默认值可以包含真实密码吗？

**A:**
```yaml
# ❌ 错误：默认值不应该是真实密码
dsn: ${DATABASE_URL:-postgresql://user:RealPassword@db.supabase.co:5432/postgres}

# ✅ 正确：默认值只用于本地开发
dsn: ${DATABASE_URL:-postgresql://postgres:postgres@localhost:54322/postgres}
```

### Q3: .env.example 应该包含什么？

**A:**
- ✅ 占位符：`[YOUR_PASSWORD]`、`[PROJECT_REF]`
- ✅ 格式示例：完整的连接串格式
- ✅ 说明注释：如何获取真实值
- ❌ 真实密码

### Q4: 如何在团队中共享配置？

**A:**
1. 提交 `.env.example` 到 Git
2. 团队成员复制并填入自己的密码
3. 生产密码通过密钥管理系统共享
4. 不要通过聊天工具发送密码

---

**最后更新:** 2025-01-22
