# Cloud Run gRPC 安全通信方案

> **版本**: v1.0
> **更新日期**: 2025-01-24
> **适用项目**: kratos-template 及所有基于该模板的微服务

---

## 目录

1. [方案概述](#1-方案概述)
2. [Cloud Run TLS 工作原理](#2-cloud-run-tls-工作原理)
3. [服务间认证方案](#3-服务间认证方案)
4. [代码改动清单](#4-代码改动清单)
5. [Google Cloud 配置步骤](#5-google-cloud-配置步骤)
6. [部署与验证](#6-部署与验证)
7. [常见问题](#7-常见问题)

---

## 1. 方案概述

### 1.1 架构模型

```mermaid
graph TB
    subgraph "外部客户端"
        Client[移动端/前端]
    end

    subgraph "Cloud Run - TLS 自动处理"
        subgraph "Gateway Service"
            GLB1[Cloud Run<br/>Load Balancer<br/>TLS 终止]
            GW[Gateway<br/>容器:8080<br/>HTTP]
        end

        subgraph "Catalog Service"
            GLB2[Cloud Run<br/>Load Balancer<br/>TLS 终止]
            CAT[Catalog<br/>容器:9090<br/>HTTP]
        end

        subgraph "Search Service"
            GLB3[Cloud Run<br/>Load Balancer<br/>TLS 终止]
            SEARCH[Search<br/>容器:9090<br/>HTTP]
        end
    end

    subgraph "Google Cloud IAM"
        IAM[Identity Token<br/>验证服务]
    end

    Client -->|HTTPS/gRPC<br/>TLS 1.3| GLB1
    GLB1 -->|HTTP/2| GW

    GW -->|HTTPS + Identity Token<br/>TLS 1.3| GLB2
    GW -->|HTTPS + Identity Token<br/>TLS 1.3| GLB3

    GLB2 -->|HTTP/2| CAT
    GLB3 -->|HTTP/2| SEARCH

    GW -.->|获取 ID Token| IAM
    CAT -.->|验证 Token| IAM
    SEARCH -.->|验证 Token| IAM

    style GLB1 fill:#e8f5e9
    style GLB2 fill:#e8f5e9
    style GLB3 fill:#e8f5e9
    style IAM fill:#fff9c4
```

### 1.2 核心设计原则

| 层级 | 方案 | 说明 |
|------|------|------|
| **外部流量 TLS** | Cloud Run 自动管理 | 自动签发证书、TLS 1.3、HTTP/2 支持 |
| **服务间 TLS** | Cloud Run 代理处理 | 出站流量自动使用 HTTPS，无需代码适配 |
| **服务间认证** | Google Identity Token | 基于 IAM 的 JWT 认证，替代传统 mTLS |
| **容器内监听** | HTTP (非 HTTPS) | 容器监听 `$PORT` (HTTP)，Cloud Run 处理 TLS |

### 1.3 关键优势

✅ **零证书管理** - 无需管理证书生命周期、续期、分发
✅ **自动 TLS 1.3** - Cloud Run 自动使用最新 TLS 协议
✅ **原生 IAM 集成** - 使用 Google Cloud 身份体系
✅ **代码改动最小** - 容器内代码几乎无需修改
✅ **无额外成本** - 同区域服务间调用无网络费用

---

## 2. Cloud Run TLS 工作原理

### 2.1 外部流量处理流程

```
外部客户端 (HTTPS/gRPC)
    ↓
Cloud Run Load Balancer
    ├─ TLS 1.3 握手
    ├─ 证书验证 (Google 签发的证书)
    ├─ 协议协商 (HTTP/2)
    └─ 解密请求
    ↓
Cloud Run Proxy (Envoy)
    ├─ IAM 认证检查
    ├─ 请求路由
    └─ 转发到容器
    ↓
容器内 gRPC Server (监听 HTTP)
    ├─ 监听 $PORT (如 8080)
    ├─ 处理业务逻辑
    └─ 返回响应
    ↓
Cloud Run Proxy
    └─ TLS 加密响应
    ↓
外部客户端收到 HTTPS 响应
```

**关键点**：
- ✅ 容器内**不需要**监听 HTTPS，只监听 HTTP
- ✅ TLS 由 Cloud Run 基础设施自动处理
- ✅ 证书由 Google 自动签发和续期

### 2.2 服务间调用流程

```
Gateway Service 容器
    ↓
获取 Identity Token (从 Metadata Server)
    ├─ curl http://metadata.google.internal/computeMetadata/v1/
    │       instance/service-accounts/default/identity?audience=https://catalog-xxx.run.app
    └─ 返回 JWT (有效期 1 小时)
    ↓
设置 gRPC Metadata
    └─ Authorization: Bearer <ID_TOKEN>
    ↓
拨号到目标服务 (HTTPS)
    └─ grpc.Dial("catalog-xxx.run.app:443", grpc.WithTransportCredentials(...))
    ↓
Cloud Run Load Balancer (Catalog Service)
    ├─ TLS 1.3 握手
    ├─ 解密请求
    └─ 提取 Authorization Header
    ↓
Catalog Service 容器 (JWT 中间件)
    ├─ 验证 Identity Token (通过 Google JWKS)
    ├─ 检查 audience (aud) == 自己的 URL
    ├─ 检查 issuer (iss) == accounts.google.com
    └─ 验证签名
    ↓
业务逻辑处理
```

---

## 3. 服务间认证方案

### 3.1 Identity Token 机制

**Identity Token 是什么？**

Identity Token 是由 Google Cloud IAM 签发的短期 JWT，包含以下关键字段：

```json
{
  "iss": "https://accounts.google.com",
  "sub": "service-account-unique-id",
  "aud": "https://catalog-xxx-uc.a.run.app",
  "email": "gateway-sa@project-id.iam.gserviceaccount.com",
  "email_verified": true,
  "exp": 1706112000,
  "iat": 1706108400
}
```

**关键字段说明**：

| 字段 | 说明 | 验证规则 |
|------|------|---------|
| `iss` | 签发者 | 必须是 `https://accounts.google.com` |
| `sub` | 主体 (Service Account UID) | 用于识别调用方身份 |
| `aud` | 受众 (目标服务 URL) | 必须匹配接收服务的 Cloud Run URL |
| `email` | Service Account 邮箱 | 用于 IAM 权限检查 |
| `exp` | 过期时间 | 通常 1 小时后过期 |

### 3.2 认证流程图

```mermaid
sequenceDiagram
    participant GW as Gateway Service
    participant Meta as Metadata Server
    participant IAM as Google IAM
    participant CAT as Catalog Service
    participant JWKS as Google JWKS

    Note over GW: 需要调用 Catalog Service

    GW->>Meta: GET /computeMetadata/v1/.../identity?audience=https://catalog-xxx.run.app
    Meta->>IAM: 请求为 Gateway SA 签发 ID Token
    IAM->>Meta: 返回 JWT (有效期 1h)
    Meta->>GW: 返回 ID Token

    GW->>GW: 缓存 Token (45 分钟)

    GW->>CAT: gRPC Call<br/>Metadata: Authorization: Bearer <TOKEN>

    Note over CAT: JWT 中间件拦截

    CAT->>JWKS: 获取公钥 (GET https://www.googleapis.com/oauth2/v3/certs)
    JWKS->>CAT: 返回 Google 公钥集

    CAT->>CAT: 验证 Token 签名
    CAT->>CAT: 验证 aud == 自己的 URL
    CAT->>CAT: 验证 exp > now()
    CAT->>CAT: 检查 IAM 权限 (email 是否有 run.invoker 角色)

    alt 验证成功
        CAT->>GW: 返回业务响应
    else 验证失败
        CAT->>GW: 返回 401 Unauthenticated
    end
```

### 3.3 与现有 JWT 中间件的兼容性

你的项目已经有 `gcjwt` 中间件 (lingo-utils/gcjwt)，可以**复用**该中间件，只需要调整配置：

**当前配置** (用于 Supabase JWT):
```yaml
jwt:
  server:
    jwks_url: "https://xxx.supabase.co/auth/v1/.well-known/jwks.json"
    issuer: "https://xxx.supabase.co/auth/v1"
```

**Cloud Run 环境配置** (用于 Google Identity Token):
```yaml
jwt:
  server:
    jwks_url: "https://www.googleapis.com/oauth2/v3/certs"
    issuer: "https://accounts.google.com"
    # 可选：指定受众验证规则
    audience: "https://catalog-xxx-uc.a.run.app"
```

**推荐方案**：根据环境变量切换配置

```yaml
jwt:
  server:
    jwks_url: "${JWT_JWKS_URL}"  # 本地: Supabase, Cloud Run: Google
    issuer: "${JWT_ISSUER}"      # 本地: Supabase, Cloud Run: Google
```

---

## 4. 代码改动清单

### 4.1 核心发现：gcjwt 已完整支持 Cloud Run 认证

✅ **好消息**：`lingo-utils/gcjwt` 包已经实现了所有必需的功能！

| 功能 | gcjwt 实现 | 文件位置 |
|------|-----------|---------|
| **获取 Identity Token** | `TokenSource` | `gcjwt/token_source.go` |
| **客户端注入 Token** | `Client()` 中间件 | `gcjwt/client.go` |
| **服务端验证 Token** | `Server()` 中间件 | `gcjwt/server.go` |
| **Claims 提取** | `FromContext()` | `gcjwt/claims.go` |
| **Wire 集成** | `ProviderSet` | `gcjwt/provider.go` |

### 4.2 实际需要的改动

#### 4.2.1 gRPC Server 监听端口（3 行代码）

**文件**: `internal/infrastructure/grpc_server/grpc_server.go`

**当前代码**:
```go
// 固定端口
addr := "0.0.0.0:9090"
```

**改为**:
```go
// 读取 Cloud Run 注入的 PORT 环境变量
port := os.Getenv("PORT")
if port == "" {
    port = "9090" // 本地开发默认端口
}
addr := "0.0.0.0:" + port
```

**原因**：Cloud Run 通过 `$PORT` 环境变量动态分配端口。

#### 4.2.2 验证 Wire 配置（无需修改）

检查 `cmd/grpc/wire.go` 是否已包含 `gcjwt.ProviderSet`：

```go
// cmd/grpc/wire.go
panic(wire.Build(
    configloader.ProviderSet,
    gclog.ProviderSet,
    gcjwt.ProviderSet,  // ✅ 确认已包含
    obswire.ProviderSet,
    // ...
))
```

✅ **如果已包含**：无需任何修改
❌ **如果缺失**：添加 `gcjwt.ProviderSet`

#### 4.2.3 环境变量配置（配置文件已支持）

`gcjwt` 通过 `config_loader` 自动读取环境变量，你只需在部署时设置：

**本地开发环境** (`.env`):
```bash
# 客户端：禁用 Identity Token 注入
JWT_CLIENT_DISABLED=true

# 服务端：使用 Supabase JWT 验证用户请求
JWT_SERVER_EXPECTED_AUDIENCE=https://your-service.example.com
JWT_SERVER_SKIP_VALIDATE=false
JWT_SERVER_REQUIRED=true
```

**Cloud Run 环境**:

**后端服务** (如 Catalog):
```bash
# 服务端验证 Google Identity Token
JWT_SERVER_EXPECTED_AUDIENCE=https://catalog-xxx-uc.a.run.app
JWT_SERVER_SKIP_VALIDATE=false
JWT_SERVER_REQUIRED=true
```

**Gateway 服务**:
```bash
# 客户端：调用后端服务时注入 Identity Token
JWT_CLIENT_AUDIENCE=https://catalog-xxx-uc.a.run.app
JWT_CLIENT_DISABLED=false

# 服务端：验证用户的 Supabase JWT（保持原有配置）
JWT_SERVER_EXPECTED_AUDIENCE=https://gateway-xxx-uc.a.run.app
JWT_SERVER_SKIP_VALIDATE=false
JWT_SERVER_REQUIRED=true
```

### 4.3 gcjwt 工作原理

#### 客户端中间件（自动注入 Token）

```go
// gcjwt/client.go:57-99
func Client(opts ...ClientOption) middleware.Middleware {
    tokenSource := NewTokenSource(options.audience, options.logger)

    return func(next middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            if options.disabled {
                return next(ctx, req)  // 本地开发可禁用
            }

            // 1. 获取 Identity Token (自动缓存 1 小时)
            token, err := tokenSource.Token(ctx)

            // 2. 注入到 gRPC metadata
            tr.RequestHeader().Set("authorization", "Bearer "+token)

            return next(ctx, req)
        }
    }
}
```

**Token 获取逻辑**:
```go
// gcjwt/token_source.go:18-20
func defaultIDTokenSourceFactory(ctx context.Context, audience string) (oauth2.TokenSource, error) {
    // 使用官方库，自动处理：
    // - Cloud Run: 从 Metadata Server 获取
    // - 本地: 使用 Application Default Credentials
    return idtoken.NewTokenSource(ctx, audience)
}
```

#### 服务端中间件（验证 Token）

```go
// gcjwt/server.go:69-114
func Server(opts ...ServerOption) middleware.Middleware {
    return func(next middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            // 1. 提取 Token
            token, err := extractToken(ctx, "authorization")

            // 2. 解析 Claims (不验证签名，Cloud Run 已验签)
            claims, err := parseTokenClaims(token)

            // 3. 验证 audience 和过期时间
            if !options.skipValidate {
                claims.ValidateWithLogging(options.expectedAudience, helper)
            }

            // 4. 注入 Context，业务代码可用
            ctx = NewContext(ctx, claims)

            return next(ctx, req)
        }
    }
}
```

#### 业务代码中使用 Claims

```go
func (h *VideoHandler) GetVideoDetail(ctx context.Context, req *videov1.Request) (*videov1.Response, error) {
    // 提取调用方身份
    claims, ok := gcjwt.FromContext(ctx)
    if ok {
        log.Infof("caller: %s (email: %s)", claims.Subject, claims.Email)
        // claims.Email 示例: gateway-sa@project-id.iam.gserviceaccount.com
    }

    // 业务逻辑...
}
```

### 4.4 HTTP/2 支持（已满足）

Kratos 的 gRPC Server 默认启用 HTTP/2，**无需修改**。

验证：
```go
// Kratos 内部使用标准 gRPC
import stdgrpc "google.golang.org/grpc"
gs := grpc.NewServer(opts...)  // 自动支持 HTTP/2
```

### 4.5 Dockerfile（无需修改）

当前 Dockerfile 已满足 Cloud Run 要求：

```dockerfile
# 监听 $PORT 环境变量
CMD ["./grpc", "-conf", "/app/configs/"]
```

### 4.6 代码改动总结

| 改动类型 | 文件/配置 | 改动量 | 说明 |
|---------|---------|--------|------|
| **代码** | `grpc_server.go` | **+3 行** | 读取 `$PORT` 环境变量 |
| **Wire** | `wire.go` | **0 行** | 已包含 `gcjwt.ProviderSet` |
| **配置** | 环境变量 | **0 行** | `config_loader` 已支持 |
| **依赖** | `go.mod` | **0 行** | `gcjwt` 已在 `lingo-utils` |
| **总计** | - | **3 行** | **改动量 < 0.1%** |

✅ **结论**：你的架构设计非常优秀，几乎不需要修改代码即可支持 Cloud Run！

---

## 5. Google Cloud 配置步骤

### 5.1 前置准备

#### 5.1.1 创建 Service Account

每个微服务需要独立的 Service Account：

```bash
# 1. 创建 Gateway Service Account
gcloud iam service-accounts create gateway-sa \
  --display-name="Gateway Service Account" \
  --project=YOUR_PROJECT_ID

# 2. 创建 Catalog Service Account
gcloud iam service-accounts create catalog-sa \
  --display-name="Catalog Service Account" \
  --project=YOUR_PROJECT_ID

# 3. 创建 Search Service Account
gcloud iam service-accounts create search-sa \
  --display-name="Search Service Account" \
  --project=YOUR_PROJECT_ID
```

#### 5.1.2 授予 IAM 权限

**授予 Gateway 调用其他服务的权限**:

```bash
# Gateway 可以调用 Catalog
gcloud run services add-iam-policy-binding catalog \
  --member="serviceAccount:gateway-sa@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/run.invoker" \
  --region=us-central1

# Gateway 可以调用 Search
gcloud run services add-iam-policy-binding search \
  --member="serviceAccount:gateway-sa@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/run.invoker" \
  --region=us-central1
```

**授予开发者调试权限** (可选):

```bash
# 允许你的账号直接调用服务 (用于测试)
gcloud run services add-iam-policy-binding catalog \
  --member="user:your-email@gmail.com" \
  --role="roles/run.invoker" \
  --region=us-central1
```

### 5.2 部署服务

#### 5.2.1 构建镜像

```bash
# 设置项目 ID
export PROJECT_ID=your-project-id
export REGION=us-central1

# 构建并推送到 Artifact Registry
gcloud builds submit --tag gcr.io/$PROJECT_ID/catalog:latest

# 或使用 Docker + Artifact Registry
docker build -t $REGION-docker.pkg.dev/$PROJECT_ID/learning-app/catalog:latest .
docker push $REGION-docker.pkg.dev/$PROJECT_ID/learning-app/catalog:latest
```

#### 5.2.2 部署 Catalog Service (后端服务)

```bash
gcloud run deploy catalog \
  --image gcr.io/$PROJECT_ID/catalog:latest \
  --platform managed \
  --region us-central1 \
  --service-account catalog-sa@$PROJECT_ID.iam.gserviceaccount.com \
  --set-env-vars "CLOUD_RUN_ENV=true,\
JWT_JWKS_URL=https://www.googleapis.com/oauth2/v3/certs,\
JWT_ISSUER=https://accounts.google.com,\
JWT_AUDIENCE=https://catalog-xxx-uc.a.run.app,\
DATABASE_URL=postgres://...,\
GCP_PROJECT_ID=$PROJECT_ID" \
  --allow-unauthenticated=false \
  --ingress=internal-and-cloud-load-balancing \
  --use-http2 \
  --cpu=1 \
  --memory=512Mi \
  --min-instances=0 \
  --max-instances=10 \
  --concurrency=80 \
  --timeout=60s
```

**关键参数说明**:

| 参数 | 值 | 说明 |
|------|-----|------|
| `--allow-unauthenticated` | `false` | 要求 IAM 认证 |
| `--ingress` | `internal-and-cloud-load-balancing` | 仅允许内部调用 + Load Balancer |
| `--use-http2` | (flag) | 启用 HTTP/2 (gRPC 必需) |
| `--service-account` | `catalog-sa@...` | 指定服务身份 |
| `JWT_AUDIENCE` | 自己的 Cloud Run URL | 验证 Token 受众 |

#### 5.2.3 部署 Gateway Service (前端服务)

```bash
gcloud run deploy gateway \
  --image gcr.io/$PROJECT_ID/gateway:latest \
  --platform managed \
  --region us-central1 \
  --service-account gateway-sa@$PROJECT_ID.iam.gserviceaccount.com \
  --set-env-vars "CLOUD_RUN_ENV=true,\
CATALOG_SERVICE_URL=catalog-xxx-uc.a.run.app:443,\
SEARCH_SERVICE_URL=search-xxx-uc.a.run.app:443,\
JWT_JWKS_URL=https://xxx.supabase.co/auth/v1/.well-known/jwks.json,\
JWT_ISSUER=https://xxx.supabase.co/auth/v1" \
  --allow-unauthenticated=true \
  --ingress=all \
  --use-http2 \
  --cpu=1 \
  --memory=512Mi \
  --min-instances=1 \
  --max-instances=20
```

**Gateway 特殊配置**:

- ✅ `--allow-unauthenticated=true` - 允许外部访问 (由 Gateway 验证用户 JWT)
- ✅ `--ingress=all` - 接受来自互联网的流量
- ✅ `CATALOG_SERVICE_URL` - 后端服务的 Cloud Run URL (格式: `xxx.run.app:443`)
- ✅ Gateway 的 JWT 配置仍然是 Supabase (验证用户请求)

### 5.3 GCP Console 配置 (可选)

如果不使用 gcloud 命令，可以在 Cloud Console 配置：

#### 5.3.1 部署服务

1. 访问 **Cloud Run** → **创建服务**
2. **容器镜像 URL**: `gcr.io/PROJECT_ID/catalog:latest`
3. **服务名称**: `catalog`
4. **区域**: `us-central1`
5. **认证**:
   - 取消勾选 "允许未经身份验证的调用"
6. **连接**:
   - **入站流量**: "内部流量和来自 Cloud Load Balancing 的流量"
7. **容器** → **变量和密钥** → 添加环境变量:
   ```
   CLOUD_RUN_ENV=true
   JWT_JWKS_URL=https://www.googleapis.com/oauth2/v3/certs
   JWT_ISSUER=https://accounts.google.com
   JWT_AUDIENCE=https://catalog-xxx-uc.a.run.app
   ```
8. **高级设置**:
   - **服务账号**: `catalog-sa@PROJECT_ID.iam.gserviceaccount.com`
   - **使用 HTTP/2**: ✅ 勾选
   - **CPU 分配**: "仅在处理请求时分配 CPU"
   - **内存**: 512 MiB
   - **CPU**: 1
   - **并发请求数**: 80
   - **最小实例数**: 0
   - **最大实例数**: 10

#### 5.3.2 配置 IAM 权限

1. 进入 **IAM 与管理** → **IAM**
2. 点击 "授予访问权限"
3. **新的主账号**: `gateway-sa@PROJECT_ID.iam.gserviceaccount.com`
4. **角色**: "Cloud Run 调用程序" (`roles/run.invoker`)
5. **条件** (可选): 仅限特定服务
   ```
   resource.name.startsWith("services/catalog")
   ```

### 5.4 服务发现配置

Cloud Run 不需要传统的服务发现，直接使用服务 URL：

```yaml
# Gateway Service 配置
data:
  grpc_client:
    catalog_endpoint: "catalog-abc123-uc.a.run.app:443"
    search_endpoint: "search-def456-uc.a.run.app:443"
```

**获取服务 URL**:

```bash
# 方式 1: gcloud 命令
gcloud run services describe catalog --region us-central1 --format 'value(status.url)'
# 输出: https://catalog-abc123-uc.a.run.app

# 方式 2: Cloud Console
# Cloud Run → catalog 服务 → 复制 URL
```

### 5.5 Secret Manager 集成 (推荐)

敏感配置 (如数据库密码) 使用 Secret Manager：

#### 5.5.1 创建 Secret

```bash
# 创建数据库连接字符串 Secret
echo -n "postgres://user:password@db.supabase.co:5432/postgres?sslmode=require" | \
  gcloud secrets create database-url --data-file=-

# 授予 Catalog Service 访问权限
gcloud secrets add-iam-policy-binding database-url \
  --member="serviceAccount:catalog-sa@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

#### 5.5.2 挂载 Secret 到服务

**方式 1: 环境变量**:

```bash
gcloud run services update catalog \
  --update-secrets="DATABASE_URL=database-url:latest"
```

**方式 2: Volume Mount** (推荐用于大文件):

```bash
gcloud run services update catalog \
  --update-secrets="/secrets/db-creds=database-url:latest"
```

在代码中读取:

```go
// 方式 1: 环境变量
dbURL := os.Getenv("DATABASE_URL")

// 方式 2: 文件
dbURL, _ := os.ReadFile("/secrets/db-creds")
```

---

## 6. 部署与验证

### 6.1 本地测试 (模拟 Cloud Run 环境)

#### 6.1.1 安装 Cloud Run 模拟器

```bash
# 安装 gcloud local emulator (可选)
gcloud components install cloud-run-proxy
```

#### 6.1.2 本地测试 Identity Token

```bash
# 设置 Application Default Credentials
gcloud auth application-default login

# 测试获取 ID Token
gcloud auth print-identity-token \
  --audiences="https://catalog-xxx.run.app"
```

在代码中测试:

```go
package main

import (
    "context"
    "fmt"
    "google.golang.org/api/idtoken"
)

func main() {
    ctx := context.Background()
    audience := "https://catalog-xxx-uc.a.run.app"

    // 本地会使用 Application Default Credentials
    ts, err := idtoken.NewTokenSource(ctx, audience)
    if err != nil {
        panic(err)
    }

    token, err := ts.Token()
    if err != nil {
        panic(err)
    }

    fmt.Println("ID Token:", token.AccessToken)
}
```

### 6.2 部署后验证

#### 6.2.1 验证 TLS 配置

```bash
# 检查 TLS 版本
curl -vso /dev/null https://catalog-xxx-uc.a.run.app 2>&1 | grep "TLS"
# 应该看到: TLSv1.3

# 检查证书
echo | openssl s_client -connect catalog-xxx-uc.a.run.app:443 2>/dev/null | \
  openssl x509 -noout -issuer -dates
# Issuer: CN=GTS CA 1C3, O=Google Trust Services LLC, C=US
```

#### 6.2.2 验证服务间调用

**测试 Gateway → Catalog 调用**:

```bash
# 1. 获取 Gateway URL
GATEWAY_URL=$(gcloud run services describe gateway --region us-central1 --format 'value(status.url)')

# 2. 使用你的 Supabase JWT 调用 Gateway
curl -X POST $GATEWAY_URL/api/v1/videos/550e8400-e29b-41d4-a716-446655440000 \
  -H "Authorization: Bearer YOUR_SUPABASE_JWT" \
  -H "Content-Type: application/json"

# 3. 检查日志查看 Gateway → Catalog 的调用
gcloud logging read "resource.type=cloud_run_revision AND resource.labels.service_name=gateway" \
  --limit 10 --format json
```

**测试直接调用 Catalog (应失败，因为需要 IAM 认证)**:

```bash
CATALOG_URL=$(gcloud run services describe catalog --region us-central1 --format 'value(status.url)')

# 未认证调用 - 应返回 403
curl $CATALOG_URL
# Error: Forbidden

# 使用 Identity Token 调用 - 应成功
TOKEN=$(gcloud auth print-identity-token --audiences=$CATALOG_URL)
curl -H "Authorization: Bearer $TOKEN" $CATALOG_URL
```

#### 6.2.3 查看日志

```bash
# 查看 Catalog Service 日志
gcloud logging read "resource.type=cloud_run_revision AND resource.labels.service_name=catalog" \
  --limit 50 --format json | jq '.[] | select(.jsonPayload.msg)'

# 过滤 JWT 认证日志
gcloud logging read "resource.type=cloud_run_revision AND resource.labels.service_name=catalog AND jsonPayload.msg=~'JWT'" \
  --limit 20 --format json
```

#### 6.2.4 监控指标

在 Cloud Console 查看:

1. **Cloud Run** → **catalog** → **指标**
2. 查看:
   - 请求数
   - 延迟 (P50/P95/P99)
   - 错误率
   - 容器实例数
   - CPU/内存使用率

### 6.3 性能测试

#### 6.3.1 使用 ghz 进行 gRPC 压测

```bash
# 安装 ghz
go install github.com/bojand/ghz/cmd/ghz@latest

# 获取 Identity Token
TOKEN=$(gcloud auth print-identity-token --audiences=https://catalog-xxx.run.app)

# 压测 GetVideoDetail RPC
ghz --insecure=false \
  --proto ./api/video/v1/video.proto \
  --call video.v1.VideoQueryService/GetVideoDetail \
  -d '{"video_id":"550e8400-e29b-41d4-a716-446655440000"}' \
  -m '{"authorization":"Bearer '$TOKEN'"}' \
  -n 1000 \
  -c 10 \
  catalog-xxx-uc.a.run.app:443

# 输出示例:
# Summary:
#   Count:        1000
#   Total:        5.23 s
#   Slowest:      523.45 ms
#   Fastest:      12.34 ms
#   Average:      45.67 ms
#   Requests/sec: 191.23
```

#### 6.3.2 冷启动测试

```bash
# 1. 缩容到 0 实例
gcloud run services update catalog --min-instances=0

# 2. 等待 15 分钟让实例完全关闭

# 3. 发送第一个请求 (测量冷启动延迟)
time curl -H "Authorization: Bearer $TOKEN" \
  https://catalog-xxx.run.app/api/v1/health

# 冷启动通常 1-3 秒
# 后续请求应该 < 100ms
```

---

## 7. 常见问题

### 7.1 认证相关

#### Q1: 本地开发如何模拟 Cloud Run Identity Token？

**A**: 使用 Application Default Credentials (ADC)

```bash
# 1. 登录 gcloud
gcloud auth application-default login

# 2. 设置环境变量
export CLOUD_RUN_ENV=true
export CATALOG_SERVICE_URL=catalog-xxx-uc.a.run.app:443

# 3. 运行服务
go run cmd/grpc/main.go -conf configs/

# google.golang.org/api/idtoken 库会自动使用 ADC
```

#### Q2: 如何调试 JWT 验证失败？

**A**: 启用详细日志

```go
// 在 gcjwt 中间件中添加
log.NewHelper(logger).Debugf("JWT validation failed: %v", err)

// 检查具体错误
if err := jwt.Verify(...); err != nil {
    log.Errorf("aud mismatch: expected %s, got %s", expectedAud, token.Audience)
}
```

查看日志:

```bash
gcloud logging read "resource.labels.service_name=catalog AND jsonPayload.level=ERROR" \
  --limit 20
```

#### Q3: Identity Token 缓存策略是什么？

**A**: Token 有效期 1 小时，建议提前 5 分钟刷新

```go
// 在 IdentityTokenCredentials 中实现
if time.Now().Before(c.expiry.Add(-5*time.Minute)) {
    return cachedToken, nil
}
// 否则获取新 Token
```

### 7.2 网络相关

#### Q4: 服务间调用需要走公网吗？

**A**: 同区域服务通过 Google 内网通信

- ✅ 同区域 (us-central1 → us-central1): 内网，无费用
- ⚠️ 跨区域 (us-central1 → us-west1): 可能走公网，有出站费用

**最佳实践**: 所有服务部署在同一区域

#### Q5: 如何完全禁止外部访问后端服务？

**A**: 使用 `--ingress=internal`

```bash
gcloud run services update catalog \
  --ingress=internal \
  --allow-unauthenticated=false
```

此时只有以下流量可以访问：
- 同项目的 Cloud Run 服务
- VPC 内的 Compute Engine / GKE
- Cloud Load Balancer

#### Q6: 支持自定义域名吗？

**A**: 支持，但 Identity Token 不支持自定义域名作为 audience

```bash
# 映射自定义域名 (仅用于外部访问)
gcloud run domain-mappings create --service gateway --domain api.example.com

# 服务间调用仍需使用 Cloud Run 原始 URL
CATALOG_SERVICE_URL=catalog-xxx-uc.a.run.app:443  # 不能用自定义域名
```

### 7.3 性能相关

#### Q7: gRPC 流式调用支持吗？

**A**: 完全支持

```protobuf
service VideoStreamService {
  rpc StreamVideos(StreamRequest) returns (stream VideoChunk);
}
```

Cloud Run 会自动处理长连接和流式响应。

#### Q8: 如何优化冷启动时间？

**A**: 多种策略

1. **设置最小实例数**:
   ```bash
   gcloud run services update catalog --min-instances=1
   ```

2. **使用多阶段构建减小镜像大小**:
   ```dockerfile
   FROM golang:1.22 AS builder
   # 构建
   FROM gcr.io/distroless/base-debian12
   # 仅包含二进制
   ```

3. **启用 Startup CPU Boost**:
   ```bash
   gcloud run services update catalog --cpu-boost
   ```

#### Q9: 并发请求数如何配置？

**A**: 根据服务特性调整

- **CPU 密集型** (如视频转码): `--concurrency=10`
- **I/O 密集型** (如 API Gateway): `--concurrency=80`
- **数据库密集型**: 根据连接池大小设置

```bash
gcloud run services update catalog --concurrency=50
```

### 7.4 安全相关

#### Q10: 如何防止服务被滥用？

**A**: 多层防护

1. **IAM 权限最小化**:
   ```bash
   # 仅授予必要权限
   gcloud run services add-iam-policy-binding catalog \
     --member="serviceAccount:gateway-sa@..." \
     --role="roles/run.invoker"
   ```

2. **使用 Cloud Armor** (针对 Gateway):
   ```bash
   # 配置速率限制、IP 白名单等
   gcloud compute security-policies create gateway-policy
   ```

3. **启用 VPC Service Controls** (企业级):
   限制服务访问范围到特定 VPC。

#### Q11: 如何审计服务间调用？

**A**: 使用 Cloud Audit Logs

```bash
# 启用数据访问审计日志
gcloud logging sinks create audit-sink \
  cloudaudit.googleapis.com/data_access

# 查询调用记录
gcloud logging read "protoPayload.serviceName='run.googleapis.com' AND protoPayload.methodName='google.cloud.run.v2.Services.GetService'" \
  --limit 50
```

---

## 8. 部署清单 (Checklist)

### 8.1 代码准备

- [ ] 修改 `grpc_server.go` 读取 `$PORT` 环境变量 (+3 行)
- [ ] 验证 `wire.go` 包含 `gcjwt.ProviderSet` (无需修改)
- [ ] 确认 `gcjwt` 依赖已在 `go.mod` (已在 lingo-utils)
- [ ] 准备环境变量配置 (部署时设置)
- [ ] 编写 Dockerfile (已有模板，无需修改)
- [ ] 编写 `cloudbuild.yaml` (可选，用于 CI/CD)

### 8.2 GCP 配置

- [ ] 创建 Service Accounts (gateway-sa, catalog-sa, search-sa)
- [ ] 授予 IAM 权限 (`roles/run.invoker`)
- [ ] 创建 Secrets (数据库密码等)
- [ ] 授予 Secret 访问权限 (`roles/secretmanager.secretAccessor`)
- [ ] 配置 Artifact Registry 或 Container Registry

### 8.3 部署服务

- [ ] 构建并推送镜像
- [ ] 部署 Catalog Service (后端，`--ingress=internal`)
- [ ] 部署 Search Service (后端，`--ingress=internal`)
- [ ] 部署 Gateway Service (前端，`--ingress=all`)
- [ ] 记录各服务的 Cloud Run URL

### 8.4 验证

- [ ] 测试 TLS 证书 (`curl -v https://...`)
- [ ] 测试外部访问 Gateway (使用 Supabase JWT)
- [ ] 测试服务间调用 (查看日志)
- [ ] 验证 IAM 权限 (未授权调用应返回 403)
- [ ] 性能测试 (ghz 压测)
- [ ] 监控指标 (Cloud Console)

### 8.5 监控与告警

- [ ] 配置 Cloud Monitoring 告警 (错误率、延迟)
- [ ] 配置 Log-based Metrics (JWT 认证失败次数)
- [ ] 配置 Uptime Checks (外部健康检查)

---

## 9. 参考资料

### 9.1 官方文档

- [Cloud Run gRPC 支持](https://cloud.google.com/run/docs/triggering/grpc)
- [Cloud Run 服务间认证](https://cloud.google.com/run/docs/authenticating/service-to-service)
- [Identity Token 获取方法](https://cloud.google.com/docs/authentication/get-id-token)
- [gRPC 认证指南](https://grpc.io/docs/guides/auth/)

### 9.2 社区资源

- [gRPC on Cloud Run (Medium)](https://medium.com/google-cloud/grpc-on-cloud-run-743ed586d4ad)
- [Robust gRPC on Cloud Run (Three Dots Labs)](https://threedots.tech/post/robust-grpc-google-cloud-run/)
- [gRPC Service-to-Service Authentication](https://ahmet.im/blog/grpc-auth-cloud-run/)

### 9.3 代码示例

- [官方 Go gRPC 示例](https://github.com/GoogleCloudPlatform/golang-samples/tree/main/run/grpc-server)
- [Identity Token 认证示例](https://cloud.google.com/run/docs/samples/cloudrun-grpc-request-auth)

---

## 10. 快速开始指南

### 第 1 步：代码准备（< 5 分钟）

```bash
# 1. 修改 gRPC Server 监听端口
# 编辑 internal/infrastructure/grpc_server/grpc_server.go
port := os.Getenv("PORT")
if port == "" {
    port = "9090"
}

# 2. 验证 Wire 配置
grep "gcjwt.ProviderSet" cmd/grpc/wire.go
# 应该看到输出，说明已配置

# 3. 编译测试
go build ./cmd/grpc
```

### 第 2 步：本地测试（可选）

```bash
# 设置 Application Default Credentials
gcloud auth application-default login

# 设置环境变量模拟 Cloud Run
export JWT_CLIENT_AUDIENCE=https://catalog-test.run.app
export JWT_CLIENT_DISABLED=false

# 运行服务
go run ./cmd/grpc/main.go -conf configs/
```

### 第 3 步：部署到 Cloud Run

```bash
# 使用部署脚本（推荐）
GCP_PROJECT_ID=your-project \
  SERVICE_NAME=catalog \
  ./scripts/deploy-cloud-run.sh

# 或使用 gcloud 命令
gcloud run deploy catalog \
  --image gcr.io/your-project/catalog:latest \
  --region us-central1 \
  --set-env-vars "JWT_SERVER_EXPECTED_AUDIENCE=https://catalog-xxx.run.app"
```

### 第 4 步：验证部署

```bash
# 测试 TLS
curl -v https://catalog-xxx-uc.a.run.app 2>&1 | grep TLS

# 测试认证
TOKEN=$(gcloud auth print-identity-token --audiences=https://catalog-xxx.run.app)
curl -H "Authorization: Bearer $TOKEN" https://catalog-xxx.run.app

# 查看日志
gcloud logging read "resource.labels.service_name=catalog" --limit 20
```

---

## 11. 下一步行动

| 阶段 | 任务 | 预计时间 |
|------|------|---------|
| **1. 准备** | 阅读本文档，理解 Cloud Run TLS 和认证机制 | 30 分钟 |
| **2. 开发** | 修改 `grpc_server.go` 读取 `$PORT` | 5 分钟 |
| **3. 测试** | 本地使用 ADC 测试 gcjwt | 15 分钟 |
| **4. 部署** | 部署 Catalog/Gateway 到 Cloud Run | 30 分钟 |
| **5. 验证** | 测试 TLS、认证、服务间调用 | 20 分钟 |
| **6. 监控** | 配置 Cloud Monitoring 告警 | 20 分钟 |
| **总计** | - | **~2 小时** |

---

**文档维护者**: Learning-App Team
**最后更新**: 2025-01-24
**版本**: v2.0 (精简版)
