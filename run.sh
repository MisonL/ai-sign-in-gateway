#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_DIR="$ROOT_DIR/.run"
BACKEND_PID_FILE="$RUN_DIR/backend.pid"
FRONTEND_PID_FILE="$RUN_DIR/frontend.pid"
BACKEND_PORT_FILE="$RUN_DIR/backend.port"
FRONTEND_PORT_FILE="$RUN_DIR/frontend.port"
BACKEND_LOG_FILE="$RUN_DIR/backend.log"
FRONTEND_LOG_FILE="$RUN_DIR/frontend.log"
BACKEND_PORT="${BACKEND_PORT:-8972}"
FRONTEND_PORT="${FRONTEND_PORT:-3721}"
START_BACKEND_PORT="$BACKEND_PORT"
START_FRONTEND_PORT="$FRONTEND_PORT"

mkdir -p "$RUN_DIR"

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少命令: $command_name"
    exit 1
  fi
}

is_running() {
  local pid_file="$1"
  if [[ ! -f "$pid_file" ]]; then
    return 1
  fi

  local pid
  pid="$(cat "$pid_file")"
  if [[ -z "$pid" ]]; then
    return 1
  fi

  kill -0 "$pid" >/dev/null 2>&1
}

port_in_use() {
  local port="$1"
  ss -ltnH "( sport = :$port )" | grep -q .
}

find_available_port() {
  local start_port="$1"
  local port="$start_port"

  while port_in_use "$port"; do
    ((port++))
    if ((port > 65535)); then
      echo "没有找到可用端口，起始端口: $start_port" >&2
      exit 1
    fi
  done

  echo "$port"
}

start_service() {
  local name="$1"
  local workdir="$2"
  local pid_file="$3"
  local log_file="$4"
  shift 4
  local command=("$@")

  if is_running "$pid_file"; then
    echo "$name 已在运行，PID: $(cat "$pid_file")"
    return 0
  fi

  rm -f "$pid_file"
  : > "$log_file"

  (
    cd "$workdir"
    setsid "${command[@]}" >>"$log_file" 2>&1 &
    echo $! >"$pid_file"
  )

  sleep 2

  if ! is_running "$pid_file"; then
    rm -f "$pid_file"
    echo "$name 启动失败，请检查日志: $log_file"
    return 1
  fi

  echo "$name 已启动，PID: $(cat "$pid_file")"
}

stop_service() {
  local pid_file="$1"
  local port_file="${2:-}"

  if [[ ! -f "$pid_file" ]]; then
    return 0
  fi

  local pid
  pid="$(cat "$pid_file")"

  if [[ -n "$pid" ]]; then
    kill -TERM "-$pid" >/dev/null 2>&1 || kill -TERM "$pid" >/dev/null 2>&1 || true
  fi

  rm -f "$pid_file"
  if [[ -n "$port_file" ]]; then
    rm -f "$port_file"
  fi
}

require_command go
require_command npm
require_command setsid
require_command ss

if is_running "$BACKEND_PID_FILE"; then
  if [[ -f "$BACKEND_PORT_FILE" ]]; then
    BACKEND_PORT="$(cat "$BACKEND_PORT_FILE")"
  fi
else
  BACKEND_PORT="$(find_available_port "$BACKEND_PORT")"
fi

if is_running "$FRONTEND_PID_FILE"; then
  if [[ -f "$FRONTEND_PORT_FILE" ]]; then
    FRONTEND_PORT="$(cat "$FRONTEND_PORT_FILE")"
  fi
elif port_in_use "$FRONTEND_PORT"; then
  echo "前端端口 $FRONTEND_PORT 已被占用，请先释放后再启动。可用 lsof -i :$FRONTEND_PORT 排查。" >&2
  exit 1
fi

if [[ "$BACKEND_PORT" != "$START_BACKEND_PORT" ]]; then
  echo "后端起始端口 $START_BACKEND_PORT 已被占用，改用端口 $BACKEND_PORT"
fi

echo "构建后端二进制..."
GO_BIN_DIR="$RUN_DIR/bin"
mkdir -p "$GO_BIN_DIR"
GO_BIN="$GO_BIN_DIR/ai-sign-in-gateway"
(
  cd "$ROOT_DIR"
  go build -o "$GO_BIN" ./cmd/ai-sign-in-gateway
)

echo "准备前端依赖..."
(
  cd "$ROOT_DIR/frontend"
  if [[ -d node_modules ]]; then
    npm install
  else
    npm ci
  fi
)

start_service \
  "后端" \
  "$ROOT_DIR" \
  "$BACKEND_PID_FILE" \
  "$BACKEND_LOG_FILE" \
  env \
    "AI_SIGN_IN_GATEWAY_HOST=0.0.0.0" \
    "AI_SIGN_IN_GATEWAY_PORT=${BACKEND_PORT}" \
    "AI_SIGN_IN_GATEWAY_OPEN_BROWSER=false" \
    "DATABASE_URL=${DATABASE_URL:-sqlite:///${ROOT_DIR}/.run/ai-sign-in-gateway-go.db}" \
    "CORS_ORIGINS=http://localhost:${FRONTEND_PORT},http://127.0.0.1:${FRONTEND_PORT}" \
  "$GO_BIN"
printf '%s\n' "$BACKEND_PORT" >"$BACKEND_PORT_FILE"

if ! start_service \
  "前端" \
  "$ROOT_DIR/frontend" \
  "$FRONTEND_PID_FILE" \
  "$FRONTEND_LOG_FILE" \
  env "VITE_API_BASE=/api" "VITE_PROXY_TARGET=http://127.0.0.1:${BACKEND_PORT}" \
  npm run dev -- --host 0.0.0.0 --port "$FRONTEND_PORT" --strictPort; then
  rm -f "$FRONTEND_PORT_FILE"
  stop_service "$BACKEND_PID_FILE" "$BACKEND_PORT_FILE"
  echo "前端启动失败，已停止后端。"
  exit 1
fi
printf '%s\n' "$FRONTEND_PORT" >"$FRONTEND_PORT_FILE"

echo
echo "访问地址:"
echo "  前端: http://127.0.0.1:${FRONTEND_PORT}"
echo "  后端: http://127.0.0.1:${BACKEND_PORT}"
echo
echo "日志文件:"
echo "  $BACKEND_LOG_FILE"
echo "  $FRONTEND_LOG_FILE"
echo
echo "停止服务请执行: ./stop.sh"
