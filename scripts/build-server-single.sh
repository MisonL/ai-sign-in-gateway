#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-ai-sign-in-gateway}"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/.release}"
TARGET_GOOS="${TARGET_GOOS:-linux}"
TARGET_GOARCH="${TARGET_GOARCH:-amd64}"
SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
SERVER_OPEN_BROWSER="${SERVER_OPEN_BROWSER:-false}"
OUTPUT_EXT=""

if [[ "$TARGET_GOOS" == "windows" ]]; then
  OUTPUT_EXT=".exe"
fi

OUTPUT_PATH="${OUTPUT_PATH:-$OUTPUT_DIR/${APP_NAME}-server-${TARGET_GOOS}-${TARGET_GOARCH}${OUTPUT_EXT}}"
BASE_GO_LDFLAGS="${GO_LDFLAGS:--s -w}"
SERVER_GO_LDFLAGS="$BASE_GO_LDFLAGS -X main.defaultHost=$SERVER_HOST -X main.defaultOpenBrowser=$SERVER_OPEN_BROWSER"

DESKTOP_SHELL=false \
CGO_ENABLED=0 \
BUILD_TAGS=embedded_assets \
BUILD_LABEL=服务版 \
TARGET_GOOS="$TARGET_GOOS" \
TARGET_GOARCH="$TARGET_GOARCH" \
GO_LDFLAGS="$SERVER_GO_LDFLAGS" \
OUTPUT_PATH="$OUTPUT_PATH" \
"$ROOT_DIR/scripts/build-desktop-single.sh"

echo "服务版单文件已生成:"
echo "  $OUTPUT_PATH"
echo "默认监听: ${SERVER_HOST}:8972，端口占用时继续沿用自动偏移策略"
