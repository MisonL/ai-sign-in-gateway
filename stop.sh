#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_DIR="$ROOT_DIR/.run"
BACKEND_PID_FILE="$RUN_DIR/backend.pid"
FRONTEND_PID_FILE="$RUN_DIR/frontend.pid"
BACKEND_PORT_FILE="$RUN_DIR/backend.port"
FRONTEND_PORT_FILE="$RUN_DIR/frontend.port"

stop_service() {
  local name="$1"
  local pid_file="$2"
  local port_file="${3:-}"

  if [[ ! -f "$pid_file" ]]; then
    if [[ -n "$port_file" ]]; then
      rm -f "$port_file"
    fi
    echo "$name 未运行"
    return 0
  fi

  local pid
  pid="$(cat "$pid_file")"

  if [[ -z "$pid" ]]; then
    rm -f "$pid_file"
    if [[ -n "$port_file" ]]; then
      rm -f "$port_file"
    fi
    echo "$name PID 文件为空，已清理"
    return 0
  fi

  if ! kill -0 "$pid" >/dev/null 2>&1; then
    rm -f "$pid_file"
    if [[ -n "$port_file" ]]; then
      rm -f "$port_file"
    fi
    echo "$name 进程不存在，已清理 PID 文件"
    return 0
  fi

  kill -TERM "-$pid" >/dev/null 2>&1 || kill -TERM "$pid" >/dev/null 2>&1 || true

  for _ in {1..20}; do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      rm -f "$pid_file"
      if [[ -n "$port_file" ]]; then
        rm -f "$port_file"
      fi
      echo "$name 已停止"
      return 0
    fi
    sleep 0.5
  done

  kill -KILL "-$pid" >/dev/null 2>&1 || kill -KILL "$pid" >/dev/null 2>&1 || true
  rm -f "$pid_file"
  if [[ -n "$port_file" ]]; then
    rm -f "$port_file"
  fi
  echo "$name 已强制停止"
}

stop_service "前端" "$FRONTEND_PID_FILE" "$FRONTEND_PORT_FILE"
stop_service "后端" "$BACKEND_PID_FILE" "$BACKEND_PORT_FILE"
