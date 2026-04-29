#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/.release}"

echo "构建当前系统桌面单文件..."
"$ROOT_DIR/scripts/build-desktop-single.sh"

echo "构建 Linux AppImage..."
"$ROOT_DIR/scripts/build-appimage.sh"

echo "构建 Windows 单文件 exe..."
"$ROOT_DIR/scripts/build-windows-exe.sh"

echo "桌面端产物已生成到:"
echo "  $OUTPUT_DIR"
