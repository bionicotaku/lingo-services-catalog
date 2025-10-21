#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_A="${ROOT_DIR}/configs/config.instance-a.yaml"
CONFIG_B="${ROOT_DIR}/configs/config.instance-b.yaml"
LOG_DIR="${ROOT_DIR}/.tmp/test-logs"
rm -rf "${LOG_DIR}"
mkdir -p "${LOG_DIR}"

BIN="${ROOT_DIR}/bin/server-test"

cleanup() {
  local code=$?
  for pid in "${SERVER_A_PID:-}" "${SERVER_B_PID:-}"; do
    if [[ -n "${pid}" ]]; then
      kill "-${pid}" >/dev/null 2>&1 || kill "${pid}" >/dev/null 2>&1 || true
      wait "${pid}" >/dev/null 2>&1 || true
    fi
  done
  exit "${code}"
}
trap cleanup EXIT

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd go
require_cmd curl
require_cmd grpcurl

echo "[step] building dedicated test binary via make build"
make -C "${ROOT_DIR}" build >/dev/null
cp "${ROOT_DIR}/bin/server" "${BIN}"

echo "[step] starting instance A"
env INSTANCE_ID=A "${BIN}" -conf "${CONFIG_A}" >"${LOG_DIR}/instance-a.log" 2>&1 &
SERVER_A_PID=$!

echo "[step] starting instance B"
env INSTANCE_ID=B "${BIN}" -conf "${CONFIG_B}" >"${LOG_DIR}/instance-b.log" 2>&1 &
SERVER_B_PID=$!

wait_http() {
  local url=$1
  local name=$2
  for attempt in {1..30}; do
    if curl -sSf "${url}" >/dev/null 2>&1; then
      echo "  ${name} ready (${url})"
      return 0
    fi
    sleep 1
  done
  echo "  ${name} did not become ready, last log lines:"
  tail -n 20 "${LOG_DIR}/instance-${name}.log" >&2 || true
  return 1
}

echo "[step] waiting for instances to become ready"
wait_http "http://127.0.0.1:8101/healthz" "a"
wait_http "http://127.0.0.1:8102/healthz" "b"

echo "[step] giving gRPC clients time to connect"
sleep 2

call_greeter() {
  local addr=$1
  local tag=$2
  grpcurl -plaintext -d '{"name":"Test"}' "${addr}" helloworld.v1.Greeter/SayHello
}

RESP_A=$(call_greeter "127.0.0.1:9101" "A")
RESP_B=$(call_greeter "127.0.0.1:9102" "B")

echo "[result] response from A: ${RESP_A}"
echo "[result] response from B: ${RESP_B}"

if [[ "${RESP_A}" != *"remote:"* ]] || [[ "${RESP_B}" != *"remote:"* ]]; then
  echo "remote aggregation missing; inspect ${LOG_DIR}/instance-a.log and instance-b.log" >&2
  exit 1
fi

echo "[success] dual-instance mutual call verified."
