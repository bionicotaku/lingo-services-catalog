# Catalog Query API 重构方案（草案 · 2025-10-27）

> 目标：在保持《ARCHITECTURE.md》既定职责的前提下，完善 Catalog 读 API，补齐缺失的 `GetVideoMetadata` 等接口，并梳理一条命名清晰、可扩展的数据处理流。本方案聚焦于 `CatalogQueryService` 及其依赖，写面（Lifecycle）不在本轮范围内。

---

## 1. 范围与验收

### 1.1 必须完成的 API

- **GetVideoMetadata**（新）：返回不依赖用户态的媒体/AI 元数据，供 Gateway/内部服务组合。
- **GetVideoDetail**（现有）：继续返回 `metadata + user_state`。
- **ListUserPublicVideos**（现有）：确认 `status=published` 过滤与排序，补齐分页游标校验。
- **ListMyUploads**（现有）：扩展 `stage_filter`、`status_filter` 参数支持。

> 所有响应需携带 `next_page_token`（如适用），失败路径统一 Problem Details。

### 1.2 成功标准

1. 新老 API 均通过 gRPC 合同（`buf lint`/`buf breaking`）。
2. 控制器、服务、仓储命名与《4MVC 架构.md》保持一致，不引入 “util” 类模糊文件。
3. `go test ./...` 及 `make lint` 通过；新增查询逻辑覆盖率 ≥ 80%。
4. `todo.md` / 本文件更新进度。

---

## 2. Proto 契约调整

文件：`api/video/v1/query.proto`

1. **新增** `GetVideoMetadataRequest/Response/VideoMetadata`：
   - `VideoMetadata` 包含媒体（duration/encoded_*）、AI（difficulty/summary/tags）字段。
2. **GetVideoDetailResponse**：
   - 内嵌 `VideoMetadata metadata = 2;`，并保留 `VideoDetail` 作为顶层结构（或直接折叠）。
3. **ListMyUploadsRequest**：
   - 扩展 `repeated string stage_filter = 4;`。
   - `repeated string status_filter` 保持存在但需要明确枚举约束。

生成流程：`buf generate` 后 `gofumpt`/`goimports`。

---

## 3. 控制器层重构

目录（保持现有分层）：

```
internal/controllers/
├── lifecycle_handler.go        // 写接口（已存在）
├── video_query_handler.go      // 读接口入口（重构）
└── dto/
    ├── lifecycle.go            // 写 DTO
    ├── query_detail.go         // Detail/Metadata DTO（新）
    └── query_list.go           // 列表 DTO（拆分）
```

### 3.1 Handler 要点

- `GetVideoMetadata`：调用新的 `VideoQueryService.GetMetadata`。
- `GetVideoDetail`：继续返回完整 detail，错误直接返回 Problem。
- `ListMyUploads`：解析 `stage_filter`、`status_filter`（验证合法性）并传入服务层。
- 所有方法使用 `BaseHandler` 提供的超时 & metadata 注入，确保日志字段一致。

---

## 4. 服务层设计

文件命名保持单一职责，所有新结构定义于 `internal/services/video_query_service.go`：

### 4.1 接口新增

```go
type VideoQueryService struct {
    repo    VideoQueryRepo          // 主表查询
    states  VideoUserStatesRepo     // 用户态
    cache   MetadataCache (可选)    // TODO: 后续按需引入
    logger  *log.Helper
    clock   Clock                   // 便于测试
}

type MetadataResult struct {
    Metadata *vo.VideoMetadata
}

func (s *VideoQueryService) GetMetadata(ctx context.Context, id uuid.UUID) (*MetadataResult, error)
func (s *VideoQueryService) GetVideoDetail(ctx context.Context, id uuid.UUID) (*vo.VideoDetail, error)
```

### 4.2 阶段/状态过滤

- `ListMyUploadsInput` 增加 `StageFilters []po.StageStatus`，在仓储中构造 `WHERE media_status = ANY(...)` / `analysis_status = ANY(...)`。
- `StatusFilters []po.VideoStatus` 继续使用，但在 service 层校验枚举，防止 SQL 注入。

---

## 5. 仓储与数据流命名

目录结构保持：

```
internal/repositories/
├── video_repo.go                 // VideoRepository（读写）
├── video_user_state_repo.go      // 用户态读写
└── sqlc/
    ├── queries.sql               // 只读查询
    └── lists.sql                 // 列表 SQL（按功能拆分）
```

### 5.1 SQL 调整

- `queries.sql` 增加 `GetVideoMetadata`（SELECT 媒体/AI 字段 + version + updated_at）。
- `lists.sql`（新）存放 `ListPublishedVideos`、`ListUploads`、`CountUploads`。
- 所有文件命名与用途一致，避免 “misc.sql”。

### 5.2 Repository 命名

- `FindMetadata(ctx, videoID)` → 返回 `po.VideoMetadata`。
- `ListUploads(ctx, input ListUploadsInput)` → 返回 `[]po.MyUploadEntry` + `dbCursor`。
- `VideoUserStatesRepository` 保持 `Get(ctx, userID, videoID)`/`GetBulk(ctx, videoIDs)`。

---

## 6. 流程图（数据路径概览）

```
Handler -> dto/query_detail.go -> VideoQueryService
    -> repo.VideoRepository.FindMetadata
    -> repo.VideoUserStatesRepository.Get (并发调用，ctx.WithTimeout)
    -> dto 返回 -> gRPC 响应
```

列表 API 同理，只是通过 `ListPageCursor` 包装游标。

命名约束：

- DTO 转换函数统一 `ToXxxInput` / `NewXxxResponse`。
- Service 私有辅助函数使用 `lowerCamel`，保持职责单一（例如 `applyStageFilters`).
- SQLC 输出结构保持 `po` 后缀，例如 `po.VideoMetadata`.

---

## 7. 测试计划

1. **Service 单元测试**：位于 `internal/services/test/query_service_test.go`（新）：
   - Metadata 命中/未命中场景。
   - Detail 用户态成功/失败（失败时直接返回错误）。
2. **Repository 集成测试**：`internal/repositories/test/query_repo_integration_test.go`：
   - 使用 testcontainers PG 校验游标/排序。
3. **gRPC 层测试**：更新 `internal/infrastructure/grpc_server/test/grpc_test.go`：
   - 验证 `GetVideoMetadata` Problem & metadata 透传。
4. **契约测试**：`buf lint` + `buf breaking`（与 `api/video/v1/query.proto`）。

---

## 8. 任务拆解

1. **Proto & 生成**  
   - 更新 `query.proto`，添加消息/枚举。  
   - 执行 `buf generate`，运行 `gofumpt/goimports`。
2. **DTO & Handler 重构**  
   - 拆分 `dto/query.go` 为 `query_detail.go` + `query_list.go`。  
   - 更新 `video_query_handler.go`，实现新方法与过滤参数校验。
3. **Service 改造**  
   - `VideoQueryService` 增加 Metadata 支撑。  
   - 更新列表实现支持 stage/status 过滤。
4. **Repository & SQLC**  
   - 新增/更新 SQL 并运行 `sqlc generate`。  
   - 调整仓储类型与输入结构。
5. **测试补全**  
   - 新增/更新 service、repository、grpc 测试。  
   - 确保覆盖率与 `make lint`。
6. **文档 & TODO 更新**  
   - `ARCHITECTURE.md` 若有字段变更同步。  
   - `services-catalog/todo.md` 标记完成情况。

---

## 9. 风险与回滚

- **过滤条件兼容性**：若 stage/status 过滤触发意外行为，可通过配置回退到无过滤模式。
- **SQL 兼容性**：所有新查询确保仅依赖现有列；若失败，回滚到旧版本 SQL（git revert）。

---

## 10. 后续展望（非本轮）

- 引入 `MetadataCache` （基于 Redis/本地 cache）减少重复查询。
- `CatalogAdminService` 独立 proto/handler（Post-MVP）。
- 接入 `If-None-Match` → HTTP Gateway 映射（待 Gateway 升级）。

---

> **执行提醒**：按章节顺序逐步推进；每完成一阶段需更新 `todo.md` 并跑 `make lint && go test ./...`。***
## 11. 详细 TODO 列表

1. **Proto**
   - [x] 在 `api/video/v1/query.proto` 增加 `GetVideoMetadata*`、`stage_filter` 字段（必要时更新 `video_types.proto`）。
   - [x] 运行 `buf generate` + `gofumpt` + `goimports` 并通过 `buf lint`（`buf breaking` 因缺少 baseline 未执行，需待参照仓库提供后补跑）。

2. **DTO**
   - [x] 拆分 `dto/query.go`：新建 `dto/query_detail.go`、`dto/query_list.go`，实现参数校验/响应映射。

3. **Handler**
   - [ ] 更新 `video_query_handler.go`：新增 `GetVideoMetadata`、完善 `stage_filter`/`status_filter` 解析与错误处理。

4. **Service**
   - [x] 在 `VideoQueryService` 中实现 `GetMetadata`、过滤逻辑；引入必要的辅助类型。
   - [x] 编写 `internal/services/test/query_service_test.go` 覆盖成功/失败场景。

5. **Repository & SQLC**
   - [x] 新增 `GetVideoMetadata`、`ListPublishedVideos`、`ListUploads` 等 SQL，并运行 `sqlc generate`。
   - [ ] 补充仓储集成测试（使用 testcontainers PG）。

6. **基础设施**
   - [ ] 若构造函数签名变化，更新 `cmd/grpc/wire.go` 并重新生成 `wire_gen.go`。
   - [ ] 调整 gRPC server/client 测试，Mock 新方法。

7. **质量检查**
   - [ ] `make lint`
   - [ ] `go test ./...`

8. **文档**
   - [ ] 更新 `services-catalog/todo.md`，视情况同步 `ARCHITECTURE.md`。
