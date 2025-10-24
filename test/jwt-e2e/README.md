# E2E 测试指南

## 📁 文件结构

```
test/jwt-e2e/
├── jwt_mock_test.go           # Mock Token 测试（默认运行，CI 友好）
├── jwt_real_test.go           # 真实 GCP Token 测试（需要 gcloud）
├── test_utils/                # 测试工具包
│   ├── env_loader.go          # 环境配置加载器
│   ├── mock_token.go          # Mock Token 生成器（自签名）
│   └── real_token.go          # 真实 Token 获取器（gcloud）
└── README.md
```

---

## 🏷️ 测试分类

### 1. Mock Token 测试（无需 gcloud）

**文件**: `jwt_mock_test.go`

**测试列表**:
- `TestE2E_JWT_MockToken_SkipValidate` - Mock Token 传递流程
- `TestE2E_JWT_NoToken_Required` - 无 Token + 强制要求
- `TestE2E_JWT_NoToken_Optional` - 无 Token + 可选模式

**特点**:
- ✅ 无需 Google Cloud 凭证
- ✅ 快速执行（< 1 秒）
- ✅ CI/CD 友好
- ✅ 默认运行
- 🔧 使用 `test_utils/mock_token.go` 生成自签名 Token

**运行**:
```bash
# 默认运行（CI 环境）
go test -v ./test/jwt-e2e

# 只运行 Mock 测试
go test -v ./test/jwt-e2e -run TestE2E_JWT_Mock
```

---

### 2. 真实 GCP Token 测试（需要 integration 标签）

**文件**: `jwt_real_test.go`

**构建标签**: `//go:build integration`

**测试列表**:
- `TestE2E_JWT_RealEnv_SkipValidate` - 真实 Token 传递（跳过签名验证）
- `TestE2E_JWT_RealEnv_FullValidation` - 真实 Token 完整验证
- `TestE2E_JWT_RealEnv_PrintToken` - 打印 Token 内容（调试）

**前置条件**:
1. 已运行 `gcloud auth application-default login`
2. 配置文件 `configs/.env.test` 存在并填写实际值

**工具依赖**:
- 🔧 使用 `test_utils/env_loader.go` 加载配置
- 🔧 使用 `test_utils/real_token.go` 获取真实 Token

**运行**:
```bash
# 运行真实环境测试
go test -tags=integration -v ./test/jwt-e2e

# 运行单个真实环境测试
go test -tags=integration -v ./test/jwt-e2e -run TestE2E_JWT_RealEnv_SkipValidate
```

---

## 🧰 test_utils 工具包说明

### 文件职责

| 文件 | 职责 | 使用场景 |
|------|------|---------|
| `env_loader.go` | 从 `configs/.env.test` 加载配置 | 真实环境测试 |
| `mock_token.go` | 生成自签名 JWT Token | Mock 测试 |
| `real_token.go` | 调用 gcloud 获取真实 Token | 真实环境测试 |

### 使用示例

#### 1. 生成 Mock Token

```go
import "github.com/bionicotaku/lingo-services-catalog/test/jwt-e2e/test_utils"

// 生成自签名 JWT Token
token := test_utils.GenerateValidCloudRunToken(t,
    "https://test-service.run.app/",
    "test@project.iam.gserviceaccount.com")
```

#### 2. 加载测试配置

```go
// 从 configs/.env.test 加载
cfg, err := test_utils.LoadTestEnv()
// cfg.ProjectID = "your-project"
// cfg.ServiceAccountEmail = "sa@project.iam.gserviceaccount.com"
```

#### 3. 获取真实 Token

```go
// 使用 gcloud impersonate 获取真实 Token
ts, err := test_utils.NewTokenSource(ctx,
    "sa@project.iam.gserviceaccount.com",
    "https://test-service.run.app/")
token, _ := ts.Token()
```

---

## 🚀 快速开始

### 本地开发

```bash
# 1. 登录 gcloud（一次性）
gcloud auth application-default login

# 2. 配置测试环境（一次性）
cp configs/.env.test.example configs/.env.test
# 编辑 configs/.env.test 填写实际值

# 3. 运行所有测试
go test -tags=integration -v ./test/jwt-e2e
```

### CI/CD 环境

```bash
# 只运行 Mock 测试（无需 gcloud）
go test -v ./test/jwt-e2e

# 预期输出：
# - TestE2E_JWT_MockToken_SkipValidate ✅ PASS
# - TestE2E_JWT_NoToken_Required ✅ PASS
# - TestE2E_JWT_NoToken_Optional ✅ PASS
# - TestE2E_JWT_RealEnv_* (跳过，因为没有 integration 标签)
```

---

## 📊 测试对比

| 特性 | Mock Token 测试 | 真实 GCP 测试 |
|------|----------------|--------------|
| **需要 gcloud** | ❌ | ✅ |
| **需要网络** | ❌ | ✅ |
| **执行速度** | 快（< 1s） | 中（1-2s） |
| **构建标签** | 无 | `integration` |
| **CI 环境** | ✅ 默认运行 | ⚠️ 需要配置 |
| **验证签名** | ❌ | ✅（可选） |
| **Token 来源** | 自签名 | Google 签发 |
| **使用工具** | `mock_token.go` | `real_token.go` + `env_loader.go` |

---

## 🔧 配置文件

### configs/.env.test

```bash
# Google Cloud 项目配置
GCP_PROJECT_ID=your-project-id

# Service Account 配置
JWT_TEST_SERVICE_ACCOUNT=jwt-test@your-project-id.iam.gserviceaccount.com

# JWT 测试配置
JWT_TEST_AUDIENCE=https://jwt-test-service.run.app/

# 其他配置
E2E_TEST_TIMEOUT=30
E2E_TEST_VERBOSE=true
```

**注意**: 此文件已添加到 `.gitignore`，不会提交到 Git。

---

## 🎯 CI 集成示例

### GitHub Actions

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      # 只运行 Mock 测试（无需 gcloud）
      - name: Run E2E Tests (Mock)
        run: go test -v ./test/jwt-e2e

      # 可选：在有 gcloud 凭证时运行真实测试
      # - name: Run E2E Tests (Real)
      #   if: env.GOOGLE_CREDENTIALS != ''
      #   run: |
      #     echo "$GOOGLE_CREDENTIALS" > /tmp/gcloud-key.json
      #     gcloud auth activate-service-account --key-file=/tmp/gcloud-key.json
      #     go test -tags=integration -v ./test/jwt-e2e
```

---

## 🐛 故障排查

### 问题 1: "no tests to run"

**原因**: 真实环境测试需要 `integration` 标签

**解决**:
```bash
# ❌ 错误
go test -v ./test/jwt-e2e -run TestE2E_JWT_RealEnv

# ✅ 正确
go test -tags=integration -v ./test/jwt-e2e -run TestE2E_JWT_RealEnv
```

---

### 问题 2: "无法加载配置"

**原因**: `configs/.env.test` 不存在

**解决**:
```bash
cp configs/.env.test.example configs/.env.test
# 编辑文件填写实际值
```

---

### 问题 3: "gcloud command failed"

**原因**: 未登录 gcloud 或没有权限

**解决**:
```bash
# 登录 gcloud
gcloud auth application-default login

# 验证权限
gcloud auth print-identity-token \
  --impersonate-service-account=jwt-test@PROJECT.iam.gserviceaccount.com \
  --audiences=https://test.example.com/
```

---

## 📚 相关文档

- [JWT 真实环境测试指南](../../docs/JWT_REAL_TESTING_GUIDE.md)
- [安全架构详解](../../docs/SECURITY_ARCHITECTURE.md)
- [gcjwt 集成清单](../../docs/gcjwt-integration-todo.md)
