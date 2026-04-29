#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-ai-sign-in-gateway}"
APP_DISPLAY_NAME="${APP_DISPLAY_NAME:-爱签网关}"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/.release}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
APPIMAGE_ARCH="${APPIMAGE_ARCH:-x86_64}"
APPDIR="${APPDIR:-$OUTPUT_DIR/AppDir}"
APPIMAGETOOL="${APPIMAGETOOL:-}"
LINUX_BINARY="$OUTPUT_DIR/${APP_NAME}-linux-${TARGET_ARCH}"
APPIMAGE_PATH="${APPIMAGE_PATH:-$OUTPUT_DIR/${APP_NAME}-${APPIMAGE_ARCH}.AppImage}"

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少命令: $command_name"
    exit 1
  fi
}

supports_official_appimagetool() {
  local candidate="$1"
  "$candidate" --help 2>&1 | grep -q "SOURCE \\[DESTINATION\\]"
}

resolve_appimagetool() {
  local candidate=""
  if [[ -n "$APPIMAGETOOL" ]]; then
    if supports_official_appimagetool "$APPIMAGETOOL"; then
      printf '%s\n' "$APPIMAGETOOL"
      return 0
    fi
    echo "APPIMAGETOOL 指向的工具不是 AppImageKit appimagetool: $APPIMAGETOOL" >&2
    return 1
  fi

  if command -v appimagetool >/dev/null 2>&1; then
    candidate="$(command -v appimagetool)"
    if supports_official_appimagetool "$candidate"; then
      printf '%s\n' "$candidate"
      return 0
    fi
  fi

  local cached="$OUTPUT_DIR/tools/appimagetool-${APPIMAGE_ARCH}.AppImage"
  if [[ -x "$cached" ]] && supports_official_appimagetool "$cached"; then
    printf '%s\n' "$cached"
    return 0
  fi

  require_command curl
  mkdir -p "$(dirname "$cached")"
  echo "下载 AppImageKit appimagetool..." >&2
  curl -fsSL \
    "https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-${APPIMAGE_ARCH}.AppImage" \
    -o "$cached"
  chmod +x "$cached"
  if ! supports_official_appimagetool "$cached"; then
    echo "下载的 appimagetool 无法识别，请手动设置 APPIMAGETOOL=/path/to/appimagetool-x86_64.AppImage" >&2
    return 1
  fi
  printf '%s\n' "$cached"
}

APPIMAGETOOL_BIN="$(resolve_appimagetool)"

TARGET_GOOS=linux \
TARGET_GOARCH="$TARGET_ARCH" \
OUTPUT_PATH="$LINUX_BINARY" \
"$ROOT_DIR/scripts/build-desktop-single.sh"

echo "准备 AppDir..."
rm -rf "$APPDIR"
mkdir -p \
  "$APPDIR/usr/bin" \
  "$APPDIR/usr/share/applications" \
  "$APPDIR/usr/share/icons/hicolor/16x16/apps" \
  "$APPDIR/usr/share/icons/hicolor/32x32/apps" \
  "$APPDIR/usr/share/icons/hicolor/64x64/apps" \
  "$APPDIR/usr/share/icons/hicolor/128x128/apps" \
  "$APPDIR/usr/share/icons/hicolor/256x256/apps" \
  "$APPDIR/usr/share/icons/hicolor/512x512/apps"

cp "$LINUX_BINARY" "$APPDIR/usr/bin/$APP_NAME"
chmod +x "$APPDIR/usr/bin/$APP_NAME"

cat >"$APPDIR/AppRun" <<EOF
#!/usr/bin/env sh
HERE="\$(dirname "\$(readlink -f "\$0")")"
exec "\$HERE/usr/bin/${APP_NAME}" "\$@"
EOF
chmod +x "$APPDIR/AppRun"

cat >"$APPDIR/usr/share/applications/${APP_NAME}.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=${APP_DISPLAY_NAME}
Comment=AI API sign-in and gateway desktop app
Exec=${APP_NAME}
Icon=${APP_NAME}
Terminal=false
Categories=Network;
StartupNotify=false
StartupWMClass=${APP_NAME}
EOF
cp "$APPDIR/usr/share/applications/${APP_NAME}.desktop" "$APPDIR/${APP_NAME}.desktop"

DESKTOP_ICON_DIR="$ROOT_DIR/frontend/public/desktop-icons"
for size in 16 32 64 128 256 512; do
  cp "$DESKTOP_ICON_DIR/${APP_NAME}-${size}.png" "$APPDIR/usr/share/icons/hicolor/${size}x${size}/apps/${APP_NAME}.png"
done
cp "$DESKTOP_ICON_DIR/${APP_NAME}-256.png" "$APPDIR/${APP_NAME}.png"
cp "$DESKTOP_ICON_DIR/${APP_NAME}-256.png" "$APPDIR/.DirIcon"
chmod -R u+rwX,go+rX "$APPDIR"

echo "构建 AppImage..."
rm -f "$APPIMAGE_PATH"
ARCH="$APPIMAGE_ARCH" "$APPIMAGETOOL_BIN" -n "$APPDIR" "$APPIMAGE_PATH"
chmod +x "$APPIMAGE_PATH"

echo "AppImage 已生成:"
echo "  $APPIMAGE_PATH"
