#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_DIR="$ROOT_DIR/.run"
PID_FILE="$RUN_DIR/prod.pid"
PORT_FILE="$RUN_DIR/prod.port"

if [[ ! -f "$PID_FILE" ]]; then
  rm -f "$PORT_FILE"
  echo "生产服务未运行"
  exit 0
fi

pid="$(cat "$PID_FILE")"

if [[ -z "$pid" ]]; then
  rm -f "$PID_FILE" "$PORT_FILE"
  echo "生产服务 PID 文件为空，已清理"
  exit 0
fi

if ! kill -0 "$pid" >/dev/null 2>&1; then
  rm -f "$PID_FILE" "$PORT_FILE"
  echo "生产服务进程不存在，已清理 PID 文件"
  exit 0
fi

kill -TERM "-$pid" >/dev/null 2>&1 || kill -TERM "$pid" >/dev/null 2>&1 || true

for _ in {1..20}; do
  if ! kill -0 "$pid" >/dev/null 2>&1; then
    rm -f "$PID_FILE" "$PORT_FILE"
    echo "生产服务已停止"
    exit 0
  fi
  sleep 0.5
done

kill -KILL "-$pid" >/dev/null 2>&1 || kill -KILL "$pid" >/dev/null 2>&1 || true
rm -f "$PID_FILE" "$PORT_FILE"
echo "生产服务已强制停止"
