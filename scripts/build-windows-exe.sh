#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-ai-sign-in-gateway}"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/.release}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
OUTPUT_PATH="${OUTPUT_PATH:-$OUTPUT_DIR/${APP_NAME}-windows-${TARGET_ARCH}.exe}"
WINDOWS_GUI="${WINDOWS_GUI:-true}"
WINDOWS_ICON="${WINDOWS_ICON:-true}"
WINDOWS_ICON_PATH="${WINDOWS_ICON_PATH:-$ROOT_DIR/frontend/public/desktop-icons/${APP_NAME}.ico}"
WINDOWS_RESOURCE_DIR="$ROOT_DIR/cmd/ai-sign-in-gateway"
WINDOWS_RESOURCE_RC="$OUTPUT_DIR/windows-icon-${TARGET_ARCH}.rc"
WINDOWS_RESOURCE_SYSO="$WINDOWS_RESOURCE_DIR/rsrc_windows_${TARGET_ARCH}.syso"

resolve_windres() {
  local candidates=()
  case "$TARGET_ARCH" in
    amd64)
      candidates=(x86_64-w64-mingw32-windres windres)
      ;;
    386)
      candidates=(i686-w64-mingw32-windres windres)
      ;;
    arm64)
      candidates=(aarch64-w64-mingw32-windres windres)
      ;;
    *)
      candidates=(windres)
      ;;
  esac

  local candidate
  for candidate in "${candidates[@]}"; do
    if command -v "$candidate" >/dev/null 2>&1; then
      command -v "$candidate"
      return 0
    fi
  done

  return 1
}

cleanup() {
  rm -f "$WINDOWS_RESOURCE_RC" "$WINDOWS_RESOURCE_SYSO"
}

prepare_windows_icon() {
  if [[ "$WINDOWS_ICON" != "true" ]]; then
    return 0
  fi

  if [[ ! -f "$WINDOWS_ICON_PATH" ]]; then
    echo "Windows 图标不存在: $WINDOWS_ICON_PATH" >&2
    exit 1
  fi

  local windres_bin
  if ! windres_bin="$(resolve_windres)"; then
    echo "缺少 windres，无法注入 Windows exe 图标。可安装 mingw-w64，或设置 WINDOWS_ICON=false 跳过图标。" >&2
    exit 1
  fi

  mkdir -p "$OUTPUT_DIR"
  printf '1 ICON "%s"\n' "$WINDOWS_ICON_PATH" >"$WINDOWS_RESOURCE_RC"
  "$windres_bin" -O coff -o "$WINDOWS_RESOURCE_SYSO" "$WINDOWS_RESOURCE_RC"
}

if [[ "$WINDOWS_GUI" == "true" ]]; then
  GO_LDFLAGS="${GO_LDFLAGS:--s -w -H=windowsgui}"
else
  GO_LDFLAGS="${GO_LDFLAGS:--s -w}"
fi

trap cleanup EXIT
prepare_windows_icon

TARGET_GOOS=windows \
TARGET_GOARCH="$TARGET_ARCH" \
GO_LDFLAGS="$GO_LDFLAGS" \
OUTPUT_PATH="$OUTPUT_PATH" \
"$ROOT_DIR/scripts/build-desktop-single.sh"

echo "Windows 单文件 exe 已生成:"
echo "  $OUTPUT_PATH"
