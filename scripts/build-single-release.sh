#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/.release}"

echo "构建无桌面服务器 Web 单文件..."
"$ROOT_DIR/scripts/build-server-single.sh"

echo "构建桌面端各平台单文件产物..."
"$ROOT_DIR/scripts/build-desktop-platforms.sh"

echo "全部单文件产物已生成到:"
echo "  $OUTPUT_DIR"
