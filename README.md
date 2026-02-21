# Services-Catalog

**Catalog microservice** — Maintains the authoritative video metadata and orchestrates the full pipeline from upload → transcoding → AI analysis → publishing.

---

## Overview

The Catalog service is responsible for:

* **Video metadata management**: Maintains basic video info, media attributes, and AI analysis results
* **Lifecycle orchestration**: Manages state transitions from upload to publish (`pending_upload` → `processing` → `ready` → `published`)
* **Event-driven integration**: Publishes domain events via Outbox + GCP Pub/Sub for downstream consumers (Search/Feed/Progress)

**Architecture highlights:**

* DDD-lite + CQRS + Event Sourcing
* Kratos microservice framework + Wire dependency injection
* PostgreSQL (Supabase) + SQLC
* Outbox Pattern for downstream integration
* OpenTelemetry observability

---

## Quick Start

### 1. Prerequisites

**Required:**

* Go 1.22+
* PostgreSQL 15+ (Supabase recommended)
* GCP Pub/Sub (or local emulator)

**Toolchain:**

```bash
# Install development tools
make init

# Tools installed include:
# - buf (Protocol Buffers management)
# - wire (DI code generation)
# - sqlc (SQL query code generation)
# - gofumpt, goimports (formatting)
# - staticcheck, revive (static analysis)
```

### 2. Configuration

> **Config convention**: All business configuration comes from `configs/config.yaml` (optionally override by copying `config.$ENV.yaml`). `.env` is only for sensitive values like `DATABASE_URL` or platform-injected `PORT`, and no longer controls business feature flags.

Example `.env` only needs the database connection string:

```env
DATABASE_URL=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable&search_path=catalog
```

### 3. Database migrations

```bash
# Run migration scripts
psql "$DATABASE_URL" -f migrations/001_init_catalog_schema.sql
psql "$DATABASE_URL" -f migrations/002_create_catalog_event_tables.sql
psql "$DATABASE_URL" -f migrations/003_create_catalog_videos_table.sql
psql "$DATABASE_URL" -f migrations/004_create_catalog_video_user_engagements_projection.sql
```

### 4. Start the service

```bash
# Dev mode
go run ./cmd/grpc -conf configs/config.yaml

# Or build and run
make build
./bin/grpc -conf configs/config.yaml
```

After startup, the service listens on:

* **gRPC**: `0.0.0.0:9000`
* **Metrics**: `0.0.0.0:9090/metrics` (if enabled)

### 5. Request metadata headers (Headers)

The Catalog service consistently extracts identity information from metadata injected by the GCP API Gateway, while preserving context fields prefixed with `x-md-*`. Key headers include:

| Header name                            | Description                                                                                                                                                    | Example                |
| -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| `X-Apigateway-Api-Userinfo`            | JWT payload (Base64Url encoded) after API Gateway verification. The server extracts the end-user UUID from `sub` / `user_id`. Optional for anonymous requests. | `eyJzdWIiOiI3YjYxZ...` |
| `x-md-actor-type`                      | (Post-MVP reserved) actor type; ignored by the current service; optional.                                                                                      | —                      |
| `x-md-actor-id`                        | (Post-MVP reserved) actor identifier; ignored by the current service; optional.                                                                                | —                      |
| `x-md-idempotency-key`                 | Idempotency key for write requests; used only by write APIs.                                                                                                   | `req-20251026-001`     |
| `x-md-if-match` / `x-md-if-none-match` | Conditional requests / cache control; included as needed for read/write APIs.                                                                                  | `"W/\"etag\""`         |

In `configs/config.yaml`, `data.grpc_client.metadata_keys` should keep only actively used fields (for MVP: only `X-Apigateway-Api-Userinfo` plus idempotency/conditional request headers). The `internal/metadata` package parses these values uniformly; `x-md-actor-*` is reserved for future auditing extensions but is not consumed today.

### 6. Run the Outbox publisher independently

For local debugging or split deployments, you can run the Outbox Runner separately:

```bash
go run ./cmd/tasks/outbox -conf configs/config.yaml
```

This command reads the same configuration as the main service and reuses Pub/Sub parameters, DB connection, and observability settings. It only scans `catalog.outbox_events` and publishes events to `messaging.pubsub.topic_id`.

### 7. Run the Engagement projection independently

```bash
go run ./cmd/tasks/engagement -conf configs/config.yaml
```

This task subscribes to `profile.engagement.*` events published by the Profile service (configured under `messaging.engagement`) and continuously updates the `catalog.video_user_engagements_projection` projection. It can be deployed as a standalone background worker.

---

## Project Structure

```
services-catalog/
├── cmd/grpc/               # Kratos gRPC service entrypoint
│   ├── main.go             # Main program
│   ├── wire.go             # Wire DI configuration
│   └── wire_gen.go         # Wire-generated code (auto)
├── configs/                # Configuration files
│   ├── config.yaml         # Base config
│   ├── conf.proto          # Config schema definition
│   └── .env                # Env overrides (not committed)
├── api/
│   └── video/v1/           # gRPC Proto definitions
│       ├── video.proto     # Video query/command services
│       ├── events.proto    # Domain event definitions
│       └── error_reason.proto
├── internal/
│   ├── controllers/        # gRPC handlers (API layer)
│   │   ├── video_command_handler.go
│   │   ├── video_query_handler.go
│   │   └── dto/            # Request/response mapping
│   ├── services/           # Business logic layer
│   │   ├── video_command_service.go
│   │   ├── video_query_service.go
│   │   └── video_types.go
│   ├── repositories/       # Data access layer
│   │   ├── video_repo.go
│   │   ├── outbox_repo.go
│   │   ├── sqlc/           # SQLC-generated code
│   │   └── mappers/        # DB model mapping
│   ├── models/
│   │   ├── po/             # Persistent objects (DB models)
│   │   ├── vo/             # View objects (client-facing)
│   │   └── outbox_events/  # Domain event builders
│   ├── infrastructure/     # Infrastructure
│   │   ├── configloader/   # Config loading
│   │   ├── grpc_server/    # gRPC server setup
│   │   └── grpc_client/    # gRPC client setup
│   └── tasks/              # Background jobs
│       └── outbox/         # Outbox publisher
├── migrations/             # DB migration scripts
├── sqlc/
│   └── schema/             # Schemas used by SQLC
├── test/                   # End-to-end test scripts
├── Makefile                # Common commands
├── buf.yaml                # Buf config
├── sqlc.yaml               # SQLC config
└── catalog design.md       # Detailed design doc
```

---

## API

### VideoQueryService (read-only)

```protobuf
service VideoQueryService {
  // Get video detail (read from projection table)
  rpc GetVideoDetail(GetVideoDetailRequest) returns (GetVideoDetailResponse);
}
```

**Example call:**

```bash
grpcurl -plaintext \
  -d '{"video_id":"550e8400-e29b-41d4-a716-446655440000"}' \
  localhost:9000 video.v1.VideoQueryService/GetVideoDetail
```

### VideoCommandService (writes)

```protobuf
service VideoCommandService {
  // Create a new video record
  rpc CreateVideo(CreateVideoRequest) returns (CreateVideoResponse);

  // Update video metadata
  rpc UpdateVideo(UpdateVideoRequest) returns (UpdateVideoResponse);

  // Delete a video record
  rpc DeleteVideo(DeleteVideoRequest) returns (DeleteVideoResponse);
}
```

**Example calls:**

```bash
# Create video
grpcurl -plaintext \
  -d '{
    "upload_user_id":"123e4567-e89b-12d3-a456-426614174000",
    "title":"My Video",
    "description":"Test video",
    "raw_file_reference":"gs://bucket/videos/test.mp4"
  }' \
  localhost:9000 video.v1.VideoCommandService/CreateVideo

# Update video (e.g., after media processing completes)
grpcurl -plaintext \
  -d '{
    "video_id":"550e8400-e29b-41d4-a716-446655440000",
    "media_status":"ready",
    "duration_micros":120000000,
    "thumbnail_url":"gs://bucket/thumbnails/test.jpg",
    "hls_master_playlist":"gs://bucket/hls/test/master.m3u8"
  }' \
  localhost:9000 video.v1.VideoCommandService/UpdateVideo
```

---

## Data Model

### Primary table: `catalog.videos`

**Core fields:**

* `video_id` (UUID): primary key
* `upload_user_id` (UUID): uploader
* `title`, `description`: basic info
* `raw_file_reference`: original file path (GCS)
* `status` (enum): overall state (`pending_upload` → `processing` → `ready` → `published`)
* `version` (bigint): optimistic locking version

**Media processing fields:**

* `media_status`, `media_job_id`, `media_emitted_at`
* `duration_micros`, `encoded_resolution`, `thumbnail_url`, `hls_master_playlist`

**AI analysis fields:**

* `analysis_status`, `analysis_job_id`, `analysis_emitted_at`
* `difficulty`, `summary`, `tags[]`, `raw_subtitle_url`

## Event-driven Integration

### Published events

| Event type              | When triggered   | Key fields                            | Subscribers             |
| ----------------------- | ---------------- | ------------------------------------- | ----------------------- |
| `catalog.video.created` | Video created    | `video_id`, `title`, `upload_user_id` | Search, Feed, Reporting |
| `catalog.video.updated` | Metadata updated | `video_id`, updated fields            | Search, Feed            |
| `catalog.video.deleted` | Video deleted    | `video_id`                            | Search, Feed            |

**Event flow:**

1. The service layer writes business data + Outbox records in the same transaction
2. The Outbox worker periodically scans unpublished events
3. Publishes to the `video-events` topic via GCP Pub/Sub
4. Downstream services (Search/Feed, etc.) subscribe and maintain their own read models

---

## Development Guide

### Common commands

```bash
# Format code
make fmt

# Lint
make lint

# Run tests
make test

# Build
make build

# Generate gRPC code
buf generate

# Generate SQLC code
sqlc generate

# Regenerate Wire code
wire ./cmd/grpc
```

### Adding new fields

1. **Update database schema**:

   * Create a new migration in `migrations/`
   * Update schema definitions in `sqlc/schema/`

2. **Update SQLC queries**:

   * Add queries in `internal/repositories/sqlc/*.sql`
   * Run `sqlc generate`

3. **Update models**:

   * Update `internal/models/po/video.go`
   * Update `internal/models/vo/video.go`

4. **Update API**:

   * Modify `api/video/v1/*.proto`
   * Run `buf generate`

5. **Update business logic**:

   * Add logic in `internal/services/`

## Production Deployment

### Environment variables (required)

```env
DATABASE_URL=postgres://...          # DB connection
PUBSUB_PROJECT_ID=production-project # GCP project ID
PUBSUB_VIDEO_TOPIC=video-events      # Pub/Sub topic
SERVICE_NAME=services-catalog
APP_ENV=production
```

### Health checks

The service provides gRPC Health Check:

```bash
grpc-health-probe -addr=localhost:9000
```

## Troubleshooting

### Metrics

Key metrics are exposed via OpenTelemetry:

* `catalog_outbox_publish_success_total` / `_failure_total`
* `catalog_outbox_publish_latency_ms`
* `catalog_engagement_apply_success_total` / `_failure_total`
* `catalog_engagement_event_lag_ms`

---

## Troubleshooting

### Issue: Events not published to Pub/Sub

**Check:**

1. Query the Outbox table:

   ```sql
   SELECT * FROM catalog.outbox_events WHERE published_at IS NULL;
   ```
2. Check logs for publish errors

**Fix:**

* Verify GCP credentials are configured correctly
* Check Pub/Sub topic permissions
* Restart the Outbox background task

### Issue: Video status stuck in `processing`

**Check:**

```sql
SELECT video_id, status, media_status, analysis_status, error_message
FROM catalog.videos
WHERE status = 'processing' AND updated_at < NOW() - INTERVAL '1 hour';
```

**Fix:**

* Verify Media/AI services are calling back normally
* Inspect the `error_message` field
* Manually update the state or trigger a retry

---

## References

* [Detailed design doc](./catalog%20design.md)
* [Read-only projection approach](./docs/只读投影方案.md)
* [GCP Pub/Sub setup](./docs/gcp-pubsub-setup.md)
* [Pub/Sub conventions](./docs/pubsub-conventions.md)
* [Go-Kratos official docs](https://go-kratos.dev/)
* [lingo-utils utility library](https://github.com/bionicotaku/lingo-utils)

---
