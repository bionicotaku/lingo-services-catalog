#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/configs/.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  source "$ENV_FILE"
  set +a
fi

: "${DATABASE_URL:?DATABASE_URL is required}"

TMP_DIR="$(mktemp -d)"
SERVER_LOG="$TMP_DIR/full_e2e_server.log"
if [[ -z "${PORT:-}" || "$PORT" == "0" ]]; then
  PORT=$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("", 0))
port = s.getsockname()[1]
s.close()
print(port)
PY
  )
fi
GRPC_ENDPOINT="${GRPC_ENDPOINT:-localhost:${PORT}}"

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

touch "$SERVER_LOG"

pushd "$ROOT_DIR" >/dev/null

if command -v lsof >/dev/null 2>&1; then
  if existing_pids=$(lsof -ti tcp:"$PORT" 2>/dev/null); then
    for pid in $existing_pids; do
      if [[ -n "$pid" ]]; then
        echo "==> 终止占用端口 $PORT 的进程: $pid"
        kill "$pid" >/dev/null 2>&1 || true
        wait "$pid" 2>/dev/null || true
      fi
    done
  fi
fi

PORT="$PORT" DATABASE_URL="$DATABASE_URL" \
  go run ./cmd/grpc -conf configs/config.yaml \
  >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

wait_for_port() {
  python3 - "$GRPC_ENDPOINT" <<'PY'
import socket, sys, time
addr = sys.argv[1]
host, port = addr.split(':')
host = host or '127.0.0.1'
port = int(port)
for _ in range(30):
    with socket.socket() as s:
        s.settimeout(1)
        try:
            s.connect((host, port))
        except OSError:
            time.sleep(1)
        else:
            sys.exit(0)
sys.exit(1)
PY
}

if ! wait_for_port; then
  echo "[ERROR] gRPC 服务在指定时间内未监听端口 $GRPC_ENDPOINT"
  cat "$SERVER_LOG"
  exit 1
fi

if ! kill -0 "$SERVER_PID" 2>/dev/null; then
  echo "[ERROR] gRPC 服务启动失败，日志如下："
  cat "$SERVER_LOG"
  exit 1
fi

UPLOAD_USER_ID="${UPLOAD_USER_ID:-f0ad5a16-0d50-4f94-8ff7-b99dda13ee47}"
TITLE="Full E2E $(date +%Y-%m-%dT%H:%M:%S)"

echo "==> gRPC 创建视频"
create_payload=$(cat <<JSON
{
  "upload_user_id": "$UPLOAD_USER_ID",
  "title": "$TITLE",
  "description": "full e2e test",
  "raw_file_reference": "gcs://dev-bucket/full-e2e.mp4"
}
JSON
)

set +e
create_resp=$(grpcurl -plaintext -d "$create_payload" "$GRPC_ENDPOINT" video.v1.VideoCommandService/CreateVideo)
grpc_status=$?
set -e
if [[ $grpc_status -ne 0 ]]; then
  echo "[ERROR] gRPC CreateVideo 调用失败，server 日志："
  cat "$SERVER_LOG"
  exit $grpc_status
fi
echo "$create_resp"

video_id=$(RESP="$create_resp" python3 - <<'PY'
import json, os

payload = json.loads(os.environ['RESP'])
print(payload.get('videoId') or payload.get('video_id') or '')
PY
)

if [[ -z "$video_id" ]]; then
  echo "[ERROR] 未能解析 video_id"
  exit 1
fi

echo "==> gRPC 更新视频 (设置 ready/published)"
update_payload=$(cat <<JSON
{
  "video_id": "$video_id",
  "title": "${TITLE} (updated)",
  "status": "published",
  "media_status": "ready",
  "analysis_status": "ready"
}
JSON
)

set +e
update_resp=$(grpcurl -plaintext -d "$update_payload" "$GRPC_ENDPOINT" video.v1.VideoCommandService/UpdateVideo)
update_status=$?
set -e
if [[ $update_status -ne 0 ]]; then
  echo "[ERROR] gRPC UpdateVideo 调用失败，server 日志："
  cat "$SERVER_LOG"
  exit $update_status
fi
echo "$update_resp"

sleep 5

echo "==> 再次 gRPC 更新视频 (确保状态写入)"
second_update=$(cat <<JSON
{
  "video_id": "${video_id}",
  "status": "published",
  "media_status": "ready",
  "analysis_status": "ready"
}
JSON
)

set +e
second_resp=$(grpcurl -plaintext -d "$second_update" "$GRPC_ENDPOINT" video.v1.VideoCommandService/UpdateVideo)
second_status=$?
set -e
if [[ $second_status -ne 0 ]]; then
  echo "[ERROR] 第二次 UpdateVideo 调用失败，server 日志："
  cat "$SERVER_LOG"
  exit $second_status
fi
echo "==> 等待投影刷新"

query_payload=$(cat <<JSON
{
  "video_id": "$video_id"
}
JSON
)

projection_ready=false
for attempt in {1..20}; do
  db_row=$(psql "$DATABASE_URL" -At <<SQL
SELECT title, status, media_status, analysis_status
FROM catalog.video_projection
WHERE video_id = '$video_id'::uuid;
SQL
)

  if [[ -n "$db_row" ]]; then
    IFS='|' read -r proj_title proj_status proj_media proj_analysis <<<"$db_row"
    if [[ "$proj_status" == "published" && "$proj_media" == "ready" && "$proj_analysis" == "ready" ]]; then
      projection_ready=true
      if grpcurl -plaintext -d "$query_payload" "$GRPC_ENDPOINT" video.v1.VideoQueryService/GetVideoDetail > "$TMP_DIR/detail.json"; then
        cat "$TMP_DIR/detail.json"
        break
      fi
    fi
  fi

  sleep 2
  if [[ $attempt -eq 20 ]]; then
    echo "[ERROR] 投影在超时时间内未出现或状态未就绪"
    echo "当前投影行：$db_row"
    cat "$SERVER_LOG"
    exit 1
  fi
done

if [[ "$projection_ready" != true ]]; then
  echo "[ERROR] 未检测到 ready/published 投影记录"
  cat "$SERVER_LOG"
  exit 1
fi

export VIDEO_ID="$video_id" DETAIL_PATH="$TMP_DIR/detail.json"

python3 - <<'PY'
import json, os, sys

video_id = os.environ['VIDEO_ID']
with open(os.environ['DETAIL_PATH']) as f:
    data = json.load(f)
detail = data.get('detail')
if not detail:
    sys.exit("projection detail is empty")
if detail.get('videoId') != video_id and detail.get('video_id') != video_id:
    sys.exit("projection video_id mismatch")
if detail.get('status') != 'published':
    sys.exit("projection status mismatch: {detail.get('status')}")
if detail.get('mediaStatus') != 'ready':
    sys.exit("projection media status mismatch: {detail.get('mediaStatus')}")
if detail.get('analysisStatus') != 'ready':
    sys.exit("projection analysis status mismatch: {detail.get('analysisStatus')}")
PY
export VIDEO_ID="$video_id" DETAIL_PATH="$TMP_DIR/detail.json"

echo "==> 检查 Outbox 事件数量"
event_count=$(psql "$DATABASE_URL" -At <<SQL
SELECT COUNT(*)
FROM catalog.outbox_events
WHERE aggregate_id = '$video_id'::uuid
  AND published_at IS NOT NULL;
SQL
)

if [[ -z "$event_count" || "$event_count" -lt 2 ]]; then
  echo "[ERROR] Outbox 事件数量不足：$event_count"
  cat "$SERVER_LOG"
  exit 1
fi

echo "✅ 全链路验证成功：video_id=${video_id:-<nil>}，outbox_events=$event_count"

popd >/dev/null
