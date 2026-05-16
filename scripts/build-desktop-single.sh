#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-ai-sign-in-gateway}"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/.release}"
OUTPUT_PATH="${OUTPUT_PATH:-$OUTPUT_DIR/$APP_NAME}"
TARGET_GOOS="${TARGET_GOOS:-$(go env GOOS)}"
TARGET_GOARCH="${TARGET_GOARCH:-$(go env GOARCH)}"
GO_LDFLAGS="${GO_LDFLAGS:--s -w}"
EMBED_DIR="$ROOT_DIR/cmd/ai-sign-in-gateway/embedded_dist"
DESKTOP_SHELL="${DESKTOP_SHELL:-true}"
BUILD_TAGS="${BUILD_TAGS:-embedded_assets}"
BUILD_LABEL="${BUILD_LABEL:-桌面}"

if [[ "$DESKTOP_SHELL" == "true" ]]; then
  BUILD_TAGS="$BUILD_TAGS desktop_shell"
  CGO_ENABLED_VALUE="${CGO_ENABLED:-1}"
else
  CGO_ENABLED_VALUE="${CGO_ENABLED:-0}"
fi

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少命令: $command_name"
    exit 1
  fi
}

prepare_frontend() {
  echo "构建前端..."
  (
    cd "$ROOT_DIR/frontend"
    if [[ -d node_modules ]]; then
      npm install
    else
      npm ci
    fi
    npm run build
  )

  rm -rf "$EMBED_DIR"
  mkdir -p "$EMBED_DIR"
  cp -R "$ROOT_DIR/frontend/dist" "$EMBED_DIR/dist"
}

cleanup() {
  rm -rf "$EMBED_DIR"
}

prepare_windows_webview_headers() {
  local include_dir="$OUTPUT_DIR/windows-webview2-compat"
  mkdir -p "$include_dir"
  cat >"$include_dir/EventToken.h" <<'EOF'
#pragma once

typedef struct EventRegistrationToken {
  __int64 value;
} EventRegistrationToken;
EOF
  printf '%s\n' "$include_dir"
}

prepare_linux_webkit_pkg_config_compat() {
  local pkg_config_dir="$OUTPUT_DIR/pkgconfig-compat"
  local version

  if pkg-config --exists webkit2gtk-4.0; then
    return 1
  fi
  if ! pkg-config --exists webkit2gtk-4.1; then
    echo "缺少 Linux 桌面构建依赖: webkit2gtk-4.0 或 webkit2gtk-4.1" >&2
    exit 1
  fi

  version="$(pkg-config --modversion webkit2gtk-4.1)"
  mkdir -p "$pkg_config_dir"
  cat >"$pkg_config_dir/webkit2gtk-4.0.pc" <<EOF
Name: WebKitGTK
Description: WebKitGTK 4.1 compatibility entry for webview_go
Version: $version
Requires: webkit2gtk-4.1
Libs:
Cflags:
EOF
  echo "检测到 webkit2gtk-4.1，已生成 webview_go 的 pkg-config 兼容入口。" >&2
  printf '%s\n' "$pkg_config_dir"
}

require_command go
require_command npm
if [[ "$DESKTOP_SHELL" == "true" && "$CGO_ENABLED_VALUE" != "0" && "$TARGET_GOOS" == "linux" ]]; then
  require_command pkg-config
fi

mkdir -p "$OUTPUT_DIR"
trap cleanup EXIT

prepare_frontend

echo "构建自包含${BUILD_LABEL}二进制..."
(
  cd "$ROOT_DIR"
  build_env=(
    "CGO_ENABLED=$CGO_ENABLED_VALUE"
    "GOOS=$TARGET_GOOS"
    "GOARCH=$TARGET_GOARCH"
  )
  if [[ "$CGO_ENABLED_VALUE" != "0" && "$TARGET_GOOS" == "windows" ]]; then
    webview_include_dir="$(prepare_windows_webview_headers)"
    build_env+=("CGO_CXXFLAGS=${CGO_CXXFLAGS:-} -I$webview_include_dir")
    build_env+=("CGO_CFLAGS=${CGO_CFLAGS:-} -I$webview_include_dir")
    case "$TARGET_GOARCH" in
      amd64)
        build_env+=("CC=${CC:-x86_64-w64-mingw32-gcc}")
        build_env+=("CXX=${CXX:-x86_64-w64-mingw32-g++}")
        ;;
      386)
        build_env+=("CC=${CC:-i686-w64-mingw32-gcc}")
        build_env+=("CXX=${CXX:-i686-w64-mingw32-g++}")
        ;;
    esac
  fi
  if [[ "$CGO_ENABLED_VALUE" != "0" && "$TARGET_GOOS" == "linux" ]]; then
    if webkit_pkg_config_dir="$(prepare_linux_webkit_pkg_config_compat)"; then
      build_env+=("PKG_CONFIG_PATH=$webkit_pkg_config_dir${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}")
    fi
  fi
  env "${build_env[@]}" \
    go build -tags "$BUILD_TAGS" -trimpath -ldflags "$GO_LDFLAGS" -o "$OUTPUT_PATH" ./cmd/ai-sign-in-gateway
)

echo "自包含${BUILD_LABEL}二进制已生成:"
echo "  $OUTPUT_PATH"
