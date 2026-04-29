#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_DIR="$ROOT_DIR/.run"
PID_FILE="$RUN_DIR/prod.pid"
PORT_FILE="$RUN_DIR/prod.port"
LOG_FILE="$RUN_DIR/prod.log"
HOST="${HOST:-0.0.0.0}"
PORT="${PORT:-8972}"

mkdir -p "$RUN_DIR"

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少命令: $command_name"
    exit 1
  fi
}

is_running() {
  if [[ ! -f "$PID_FILE" ]]; then
    return 1
  fi

  local pid
  pid="$(cat "$PID_FILE")"
  [[ -n "$pid" ]] || return 1
  kill -0 "$pid" >/dev/null 2>&1
}

require_command go
require_command setsid

if is_running; then
  current_port="$PORT"
  if [[ -f "$PORT_FILE" ]]; then
    current_port="$(cat "$PORT_FILE")"
  fi
  echo "生产服务已在运行，PID: $(cat "$PID_FILE")，端口: $current_port"
  exit 0
fi

echo "构建后端二进制..."
GO_BIN_DIR="$RUN_DIR/bin"
mkdir -p "$GO_BIN_DIR"
GO_BIN="$GO_BIN_DIR/ai-sign-in-gateway"
(
  cd "$ROOT_DIR"
  go build -trimpath -ldflags "-s -w" -o "$GO_BIN" ./cmd/ai-sign-in-gateway
)

rm -f "$PID_FILE"
: >"$LOG_FILE"

echo "启动生产服务..."
(
  cd "$ROOT_DIR"
  setsid env \
    "AI_SIGN_IN_GATEWAY_HOST=${HOST}" \
    "AI_SIGN_IN_GATEWAY_PORT=${PORT}" \
    "AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false" \
    "DATABASE_URL=${DATABASE_URL:-sqlite:///${ROOT_DIR}/.run/ai-sign-in-gateway-go.db}" \
    "CORS_ORIGINS=${CORS_ORIGINS:-http://localhost:${PORT},http://127.0.0.1:${PORT}}" \
    "$GO_BIN" >>"$LOG_FILE" 2>&1 &
  echo $! >"$PID_FILE"
)

sleep 2

if ! is_running; then
  rm -f "$PID_FILE"
  echo "生产服务启动失败，请检查日志: $LOG_FILE"
  exit 1
fi

printf '%s\n' "$PORT" >"$PORT_FILE"
echo "生产服务已启动，PID: $(cat "$PID_FILE")"
echo "访问地址: http://127.0.0.1:${PORT}"
echo "日志文件: $LOG_FILE"
echo "停止服务请执行: ./stop-prod.sh"
