#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-ai-sign-in-gateway}"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/.release}"
TARGET_GOOS="${TARGET_GOOS:-linux}"
TARGET_GOARCH="${TARGET_GOARCH:-amd64}"
SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
SERVER_OPEN_BROWSER="${SERVER_OPEN_BROWSER:-false}"
DEPLOY_CONFIG="${DEPLOY_CONFIG:-$ROOT_DIR/.deploy.config}"
BUILD_SERVER_SINGLE_FORCE_TUI="${BUILD_SERVER_SINGLE_FORCE_TUI:-false}"
OUTPUT_EXT=""

if [[ "$TARGET_GOOS" == "windows" ]]; then
  OUTPUT_EXT=".exe"
fi

OUTPUT_PATH="${OUTPUT_PATH:-$OUTPUT_DIR/${APP_NAME}-server-${TARGET_GOOS}-${TARGET_GOARCH}${OUTPUT_EXT}}"
BASE_GO_LDFLAGS="${GO_LDFLAGS:--s -w}"
SERVER_GO_LDFLAGS="$BASE_GO_LDFLAGS -X main.defaultHost=$SERVER_HOST -X main.defaultOpenBrowser=$SERVER_OPEN_BROWSER"

deploy_host=""
deploy_user=""
deploy_path=""
deploy_port="22"
deploy_identity_file=""

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

strip_quotes() {
  local value="$1"
  if [[ ${#value} -ge 2 ]]; then
    if [[ "${value:0:1}" == '"' && "${value: -1}" == '"' ]]; then
      value="${value:1:${#value}-2}"
    elif [[ "${value:0:1}" == "'" && "${value: -1}" == "'" ]]; then
      value="${value:1:${#value}-2}"
    fi
  fi
  printf '%s' "$value"
}

load_deploy_config() {
  local config_path="$1"
  [[ -f "$config_path" ]] || return 1

  local line key value
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="$(trim "$line")"
    [[ -z "$line" || "${line:0:1}" == "#" ]] && continue
    [[ "$line" == *=* ]] || continue
    key="$(trim "${line%%=*}")"
    value="$(trim "${line#*=}")"
    value="$(strip_quotes "$value")"
    key="${key,,}"
    case "$key" in
      host)
        deploy_host="$value"
        ;;
      user|username)
        deploy_user="$value"
        ;;
      path|remote_path)
        deploy_path="$value"
        ;;
      port)
        deploy_port="${value:-22}"
        ;;
      identity_file|identityfile|key_file|keyfile)
        deploy_identity_file="$value"
        ;;
    esac
  done <"$config_path"

  deploy_port="${deploy_port:-22}"
  [[ -n "$deploy_host" && -n "$deploy_path" ]]
}

is_interactive_deploy_available() {
  [[ -t 0 && -t 1 ]] || [[ "$BUILD_SERVER_SINGLE_FORCE_TUI" == "true" ]]
}

deploy_remote_target() {
  local remote_host="$deploy_host"
  if [[ -n "$deploy_user" ]]; then
    remote_host="$deploy_user@$remote_host"
  fi
  printf '%s:%s' "$remote_host" "$deploy_path"
}

run_scp_deploy() {
  local remote_target
  remote_target="$(deploy_remote_target)"

  if ! command -v scp >/dev/null 2>&1; then
    echo "缺少命令: scp"
    return 1
  fi

  local scp_args=(-P "$deploy_port")
  if [[ -n "$deploy_identity_file" ]]; then
    scp_args+=(-i "$deploy_identity_file")
  fi
  scp_args+=("$OUTPUT_PATH" "$remote_target")

  echo "开始上传构建产物:"
  echo "  本地: $OUTPUT_PATH"
  echo "  远端: $remote_target"
  echo "  SSH 端口: $deploy_port"
  scp "${scp_args[@]}"
  echo "上传完成。"
}

open_deploy_tui() {
  if ! load_deploy_config "$DEPLOY_CONFIG"; then
    if [[ -f "$DEPLOY_CONFIG" ]]; then
      echo "检测到 $DEPLOY_CONFIG，但缺少 host 或 path，跳过部署。"
    else
      echo "未检测到 .deploy.config，跳过部署。"
    fi
    return 0
  fi

  if ! is_interactive_deploy_available; then
    echo "检测到部署配置，但当前不是交互式终端，跳过部署。"
    return 0
  fi

  local remote_target
  remote_target="$(deploy_remote_target)"

  echo
  echo "部署配置已加载: $DEPLOY_CONFIG"
  echo "  目标: $remote_target"
  echo "  端口: $deploy_port"
  if [[ -n "$deploy_identity_file" ]]; then
    echo "  IdentityFile: $deploy_identity_file"
  fi
  echo
  echo "请选择下一步:"
  echo "  1) 上传构建产物"
  echo "  2) 跳过部署"
  printf "输入选项 [1/2]: "

  local choice
  read -r choice
  case "$(trim "$choice")" in
    1|y|Y|yes|YES)
      run_scp_deploy
      ;;
    *)
      echo "已跳过部署。"
      ;;
  esac
}

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

open_deploy_tui
