#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RELEASE_ROOT="$ROOT_DIR/.release"
APP_NAME="${APP_NAME:-ai-sign-in-gateway}"
VERSION="${VERSION:-$(date +%Y%m%d-%H%M%S)}"
PACKAGE_DIR="$RELEASE_ROOT/${APP_NAME}"
ARCHIVE_PATH="$RELEASE_ROOT/${APP_NAME}-${VERSION}.tar.gz"

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少命令: $command_name"
    exit 1
  fi
}

require_command npm
require_command tar
require_command go

echo "构建前端静态资源..."
(
  cd "$ROOT_DIR/frontend"
  if [[ -d node_modules ]]; then
    npm install
  else
    npm ci
  fi
  npm run build
)

echo "构建自包含 Go 桌面二进制..."
OUTPUT_PATH="$ROOT_DIR/ai-sign-in-gateway" "$ROOT_DIR/scripts/build-desktop-single.sh"

echo "准备发布目录..."
rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR/frontend"

cp -R "$ROOT_DIR/frontend/dist" "$PACKAGE_DIR/frontend/dist"
cp -R "$ROOT_DIR/docs" "$PACKAGE_DIR/docs"
cp "$ROOT_DIR/README.md" "$PACKAGE_DIR/README.md"
cp "$ROOT_DIR/start-prod.sh" "$PACKAGE_DIR/start-prod.sh"
cp "$ROOT_DIR/stop-prod.sh" "$PACKAGE_DIR/stop-prod.sh"
cp "$ROOT_DIR/ai-sign-in-gateway" "$PACKAGE_DIR/ai-sign-in-gateway"

chmod +x "$PACKAGE_DIR/start-prod.sh" "$PACKAGE_DIR/stop-prod.sh" "$PACKAGE_DIR/ai-sign-in-gateway"

echo "写入发布信息..."
cat >"$PACKAGE_DIR/RELEASE.txt" <<EOF
app=${APP_NAME}
version=${VERSION}
built_at=$(date -Iseconds)
EOF

echo "打包发布文件..."
mkdir -p "$RELEASE_ROOT"
tar -C "$RELEASE_ROOT" -czf "$ARCHIVE_PATH" "$APP_NAME"

echo "发布包已生成:"
echo "  $ARCHIVE_PATH"
echo
echo "部署后可执行:"
echo "  ./start-prod.sh"
echo "  ./stop-prod.sh"
