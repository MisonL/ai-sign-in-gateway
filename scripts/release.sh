#!/usr/bin/env bash

set -euo pipefail

# ============================================================
# release.sh — 本地打包后发布到 GitHub Release，并同步 release 分支
#
# 用法:
#   ./scripts/release.sh                  # 使用最新 git tag 作为版本
#   ./scripts/release.sh v1.3.0           # 指定版本号；不存在时自动创建 tag
#   ./scripts/release.sh v1.3.0 --skip-build
#   ./scripts/release.sh v1.3.0 --retag-current -y
#
# 前置条件:
#   - 已安装 git、go、npm
#   - 发布 GitHub Release 时，优先使用 gh；没有 gh 时可设置 GH_TOKEN/GITHUB_TOKEN
#   - 本地存在 GitHub remote，默认 origin，可用 GIT_REMOTE 覆盖
#   - release 分支会被更新为纯产物分支，不保留源码内容
# ============================================================

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-ai-sign-in-gateway}"
RELEASE_DIR="${RELEASE_DIR:-$ROOT_DIR/.release}"
BUILD_COMMAND="${BUILD_COMMAND:-$ROOT_DIR/scripts/build-single-release.sh}"
GIT_REMOTE="${GIT_REMOTE:-origin}"
RELEASE_BRANCH="${RELEASE_BRANCH:-release}"
GH_REPO="${GH_REPO:-}"
GITHUB_API_URL="${GITHUB_API_URL:-https://api.github.com}"
GITHUB_UPLOAD_URL="${GITHUB_UPLOAD_URL:-https://uploads.github.com}"
REMOTE_URL=""
VERSION=""
SKIP_BUILD=0
SKIP_CONFIRM=0
FORCE_RETAG_CURRENT=0
TAG_PUSH_FORCE=0
RELEASE_BRANCH_EXISTS=0
RELEASE_PUBLISHER=""
RELEASE_TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"

ARTIFACTS=()
UPLOAD_ARTIFACTS=()
CHECKSUM_PATH=""
NOTES_PATH=""
BRANCH_WORKDIR=""

usage() {
  cat <<'EOF'
用法:
  ./scripts/release.sh [version] [options]

参数:
  version             发布版本号，例如 v1.5.0；省略时使用最新 git tag

选项:
  --skip-build        不重新打包，直接发布 .release/ 下已有产物
  --retag-current     将已有版本 tag 强制移动到当前 HEAD，再覆盖发布 release
  -y, --yes           跳过交互确认
  -h, --help          显示帮助

环境变量:
  GIT_REMOTE          Git remote 名称，默认 origin
  GH_REPO             GitHub 仓库，格式 owner/repo；默认从 remote URL 解析
  RELEASE_BRANCH      纯产物发布分支，默认 release
  BUILD_COMMAND       本地打包命令，默认 ./scripts/build-single-release.sh
  RELEASE_DIR         产物目录，默认 .release

示例:
  ./scripts/release.sh v1.0.0
  ./scripts/release.sh v1.0.0 --skip-build
  GH_REPO=owner/repo ./scripts/release.sh v1.0.0 -y
EOF
}

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "错误: 缺少命令: $command_name"
    exit 1
  fi
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --skip-build)
        SKIP_BUILD=1
        ;;
      --retag-current)
        FORCE_RETAG_CURRENT=1
        ;;
      -y|--yes)
        SKIP_CONFIRM=1
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      -*)
        echo "错误: 未知参数 $1"
        echo
        usage
        exit 1
        ;;
      *)
        if [[ -n "$VERSION" ]]; then
          echo "错误: 只能指定一个版本号，收到重复参数: $VERSION 和 $1"
          exit 1
        fi
        VERSION="$1"
        ;;
    esac
    shift
  done
}

parse_github_repo() {
  local remote_url="$1"
  local repo_path="$remote_url"

  repo_path="${repo_path#git@github.com:}"
  repo_path="${repo_path#ssh://git@github.com/}"
  repo_path="${repo_path#https://github.com/}"
  repo_path="${repo_path#http://github.com/}"
  repo_path="${repo_path%.git}"

  if [[ "$repo_path" == */* && "$repo_path" != "$remote_url" ]]; then
    echo "$repo_path"
    return 0
  fi

  return 1
}

resolve_git_remote() {
  if ! git -C "$ROOT_DIR" remote get-url "$GIT_REMOTE" >/dev/null 2>&1; then
    echo "错误: 未找到 Git remote: $GIT_REMOTE"
    echo "请先添加 GitHub remote，或设置 GIT_REMOTE=<remote-name>"
    exit 1
  fi

  REMOTE_URL="$(git -C "$ROOT_DIR" remote get-url "$GIT_REMOTE")"
}

resolve_github_repo() {
  if [[ -n "$GH_REPO" ]]; then
    return
  fi

  if GH_REPO="$(parse_github_repo "$REMOTE_URL")"; then
    return
  fi

  echo "错误: 无法从 remote $GIT_REMOTE 解析 GitHub 仓库"
  echo "当前 remote URL: $REMOTE_URL"
  echo "请设置 GH_REPO=<owner>/<repo> 后重试"
  exit 1
}

detect_release_publisher() {
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    RELEASE_PUBLISHER="gh"
    return
  fi

  if [[ -n "$RELEASE_TOKEN" ]]; then
    require_command curl
    require_command jq
    RELEASE_PUBLISHER="api"
    return
  fi

  RELEASE_PUBLISHER="workflow"
  echo ">> 未检测到可用 gh 或 GH_TOKEN/GITHUB_TOKEN；本地将跳过 GitHub Release API。"
  echo ">> release 分支推送后，仓库工作流会使用 GitHub Actions 创建/更新 Release。"
}

ensure_version() {
  git -C "$ROOT_DIR" fetch "$GIT_REMOTE" --tags --prune >/dev/null 2>&1 || true

  if [[ -z "$VERSION" ]]; then
    VERSION="$(git -C "$ROOT_DIR" describe --tags --abbrev=0 2>/dev/null || true)"
    if [[ -z "$VERSION" ]]; then
      echo "错误: 未找到 git tag，请指定版本号，例如: ./scripts/release.sh v1.0.0"
      exit 1
    fi
  fi
}

prepare_tag() {
  local head_commit
  local tag_commit=""

  head_commit="$(git -C "$ROOT_DIR" rev-parse HEAD)"
  if git -C "$ROOT_DIR" rev-parse "$VERSION" >/dev/null 2>&1; then
    tag_commit="$(git -C "$ROOT_DIR" rev-parse "$VERSION^{}")"
  fi

  if [[ "$FORCE_RETAG_CURRENT" -eq 1 ]]; then
    if [[ -n "$tag_commit" && "$tag_commit" == "$head_commit" ]]; then
      echo ">> $VERSION 已指向当前 HEAD，无需重写 tag"
    else
      echo ">> 将 $VERSION 重新指向当前 HEAD ($head_commit)"
      git -C "$ROOT_DIR" tag -fa "$VERSION" -m "Release $VERSION" "$head_commit"
      TAG_PUSH_FORCE=1
    fi
  elif [[ -z "$tag_commit" ]]; then
    echo ">> 创建 tag: $VERSION"
    git -C "$ROOT_DIR" tag -a "$VERSION" -m "Release $VERSION" "$head_commit"
  fi

  if [[ "$TAG_PUSH_FORCE" -eq 1 ]]; then
    echo ">> 强制推送 tag 到 $GIT_REMOTE"
    git -C "$ROOT_DIR" push --force "$GIT_REMOTE" "refs/tags/$VERSION"
  else
    echo ">> 推送 tag 到 $GIT_REMOTE"
    git -C "$ROOT_DIR" push "$GIT_REMOTE" "$VERSION"
  fi
}

confirm_release() {
  local dirty_status
  dirty_status="$(git -C "$ROOT_DIR" status --porcelain)"

  echo "=========================================="
  echo "  发布版本: $VERSION"
  echo "  GitHub:   $GH_REPO"
  echo "  Remote:   $GIT_REMOTE"
  echo "  分支:     $RELEASE_BRANCH"
  echo "  构建:     $([[ "$SKIP_BUILD" -eq 1 ]] && echo "跳过，使用现有产物" || echo "$BUILD_COMMAND")"
  if [[ -n "$dirty_status" ]]; then
    echo "  注意:     当前工作区有未提交变更；产物会按本地文件构建，tag 仍指向当前 HEAD"
  fi
  echo "=========================================="

  if [[ "$SKIP_CONFIRM" -eq 1 ]]; then
    echo ">> 已指定 --yes，跳过确认"
    return
  fi

  if [[ -t 0 ]]; then
    read -rp "确认发布? (y/N) " confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
      echo "已取消"
      exit 0
    fi
  else
    echo ">> 非交互环境，跳过确认"
  fi
}

build_artifacts() {
  if [[ "$SKIP_BUILD" -eq 1 ]]; then
    echo ">> 跳过本地打包"
    return
  fi

  if [[ ! -x "$BUILD_COMMAND" ]]; then
    echo "错误: 打包命令不可执行: $BUILD_COMMAND"
    exit 1
  fi

  echo ">> 执行本地打包: $BUILD_COMMAND"
  "$BUILD_COMMAND"
}

collect_artifacts() {
  if [[ ! -d "$RELEASE_DIR" ]]; then
    echo "错误: 产物目录不存在: $RELEASE_DIR"
    exit 1
  fi

  mapfile -t ARTIFACTS < <(
    find "$RELEASE_DIR" -maxdepth 1 -type f \
      \( \
        -name "$APP_NAME" \
        -o -name "$APP_NAME-server-*" \
        -o -name "$APP_NAME-windows-*.exe" \
        -o -name "$APP_NAME-*.AppImage" \
        -o -name "$APP_NAME-*.tar.gz" \
      \) \
      ! -name "*SHA256SUMS*" \
      ! -name "*RELEASE_NOTES*" \
      | sort
  )

  if [[ "${#ARTIFACTS[@]}" -eq 0 ]]; then
    echo "错误: $RELEASE_DIR 下没有可发布产物"
    echo "请先执行 $BUILD_COMMAND，或检查 RELEASE_DIR"
    exit 1
  fi

  echo ">> 待发布产物:"
  for artifact in "${ARTIFACTS[@]}"; do
    ls -lh "$artifact"
  done
}

generate_checksums() {
  require_command sha256sum

  CHECKSUM_PATH="$RELEASE_DIR/${APP_NAME}-${VERSION}-SHA256SUMS.txt"
  : >"$CHECKSUM_PATH"

  for artifact in "${ARTIFACTS[@]}"; do
    (
      cd "$RELEASE_DIR"
      sha256sum "$(basename "$artifact")"
    ) >>"$CHECKSUM_PATH"
  done

  UPLOAD_ARTIFACTS=("${ARTIFACTS[@]}" "$CHECKSUM_PATH")
}

generate_release_notes() {
  local history_start
  local commits
  local downloads

  history_start="$(git -C "$ROOT_DIR" describe --tags --abbrev=0 HEAD^ 2>/dev/null || git -C "$ROOT_DIR" rev-list --max-parents=0 HEAD | tail -n 1)"
  commits="$(git -C "$ROOT_DIR" log "$history_start"..HEAD --oneline 2>/dev/null || true)"
  if [[ -z "$commits" ]]; then
    commits="Initial release"
  fi

  downloads=""
  for artifact in "${UPLOAD_ARTIFACTS[@]}"; do
    downloads+="- $(basename "$artifact")"$'\n'
  done

  NOTES_PATH="$RELEASE_DIR/${APP_NAME}-${VERSION}-RELEASE_NOTES.md"
  cat >"$NOTES_PATH" <<EOF
## $VERSION

### Downloads

${downloads}
### Defaults

- 服务版默认监听 \`0.0.0.0:8972\`
- 桌面端默认入口 \`127.0.0.1:3721\`，后端/API/网关 \`127.0.0.1:8972\`
- 端口占用时沿用自动偏移策略

### Commits since last release

${commits}
EOF
}

publish_github_release() {
  if [[ "$RELEASE_PUBLISHER" == "workflow" ]]; then
    echo ">> 跳过本地 GitHub Release API，等待远端工作流发布: $VERSION"
    return
  fi

  if gh release view "$VERSION" --repo "$GH_REPO" >/dev/null 2>&1; then
    echo ">> 更新 GitHub Release: $VERSION"
    gh release edit "$VERSION" \
      --repo "$GH_REPO" \
      --title "$VERSION" \
      --notes-file "$NOTES_PATH"
    gh release upload "$VERSION" \
      "${UPLOAD_ARTIFACTS[@]}" \
      --repo "$GH_REPO" \
      --clobber
  else
    echo ">> 创建 GitHub Release: $VERSION"
    gh release create "$VERSION" \
      "${UPLOAD_ARTIFACTS[@]}" \
      --repo "$GH_REPO" \
      --title "$VERSION" \
      --notes-file "$NOTES_PATH" \
      --verify-tag
  fi
}

github_api() {
  local method="$1"
  local path="$2"
  local data_path="${3:-}"

  if [[ -n "$data_path" ]]; then
    curl -fsS \
      -X "$method" \
      -H "Authorization: Bearer $RELEASE_TOKEN" \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      -H "Content-Type: application/json" \
      --data-binary "@$data_path" \
      "$GITHUB_API_URL/repos/$GH_REPO$path"
  else
    curl -fsS \
      -X "$method" \
      -H "Authorization: Bearer $RELEASE_TOKEN" \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "$GITHUB_API_URL/repos/$GH_REPO$path"
  fi
}

publish_github_release_api() {
  local release_json
  local release_id
  local payload_path
  local asset
  local asset_name
  local asset_id

  payload_path="$(mktemp "${TMPDIR:-/tmp}/${APP_NAME}-release-payload-XXXXXX.json")"
  jq -n \
    --arg tag_name "$VERSION" \
    --arg name "$VERSION" \
    --rawfile body "$NOTES_PATH" \
    '{tag_name:$tag_name, name:$name, body:$body, draft:false, prerelease:false}' \
    >"$payload_path"

  if release_json="$(github_api GET "/releases/tags/$VERSION" 2>/dev/null)"; then
    release_id="$(jq -r '.id' <<<"$release_json")"
    echo ">> 更新 GitHub Release: $VERSION"
    github_api PATCH "/releases/$release_id" "$payload_path" >/dev/null
  else
    echo ">> 创建 GitHub Release: $VERSION"
    release_json="$(github_api POST "/releases" "$payload_path")"
    release_id="$(jq -r '.id' <<<"$release_json")"
  fi

  for asset in "${UPLOAD_ARTIFACTS[@]}"; do
    asset_name="$(basename "$asset")"
    asset_id="$(
      github_api GET "/releases/$release_id/assets" \
        | jq -r --arg name "$asset_name" '.[] | select(.name == $name) | .id' \
        | head -n 1
    )"
    if [[ -n "$asset_id" ]]; then
      github_api DELETE "/releases/assets/$asset_id" >/dev/null
    fi
    echo ">> 上传 Release 资产: $asset_name"
    curl -fsS \
      -X POST \
      -H "Authorization: Bearer $RELEASE_TOKEN" \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      -H "Content-Type: application/octet-stream" \
      --data-binary "@$asset" \
      "$GITHUB_UPLOAD_URL/repos/$GH_REPO/releases/$release_id/assets?name=$asset_name" \
      >/dev/null
  done
}

cleanup() {
  if [[ -n "$BRANCH_WORKDIR" && -d "$BRANCH_WORKDIR" ]]; then
    rm -rf "$BRANCH_WORKDIR"
  fi
}

prepare_release_branch_workdir() {
  BRANCH_WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/${APP_NAME}-release-branch-XXXXXX")"

  if git -C "$ROOT_DIR" ls-remote --exit-code --heads "$GIT_REMOTE" "$RELEASE_BRANCH" >/dev/null 2>&1; then
    RELEASE_BRANCH_EXISTS=1
  fi

  git clone --no-checkout "$REMOTE_URL" "$BRANCH_WORKDIR" >/dev/null
  (
    cd "$BRANCH_WORKDIR"
    if [[ "$RELEASE_BRANCH_EXISTS" -eq 1 ]]; then
      git fetch origin "$RELEASE_BRANCH" >/dev/null
      git switch --detach "origin/$RELEASE_BRANCH" >/dev/null
    else
      git switch --orphan "$RELEASE_BRANCH" >/dev/null
    fi

    git config user.name "${GIT_AUTHOR_NAME:-$(git -C "$ROOT_DIR" config user.name || echo "ai-sign-in-gateway release")}"
    git config user.email "${GIT_AUTHOR_EMAIL:-$(git -C "$ROOT_DIR" config user.email || echo "release@local")}"
  )
}

publish_release_branch() {
  local branch_readme

  prepare_release_branch_workdir

  (
    cd "$BRANCH_WORKDIR"
    git rm -r --ignore-unmatch . >/dev/null 2>&1 || true
    find . -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +

    for artifact in "${UPLOAD_ARTIFACTS[@]}"; do
      cp "$artifact" .
    done
    cp "$NOTES_PATH" RELEASE_NOTES.md

    cat >RELEASE.txt <<EOF
app=$APP_NAME
version=$VERSION
built_at=$(date -Iseconds)
source_commit=$(git -C "$ROOT_DIR" rev-parse HEAD)
github_release=https://github.com/$GH_REPO/releases/tag/$VERSION
EOF

    branch_readme="README.md"
    cat >"$branch_readme" <<EOF
# $APP_NAME release artifacts

此分支只保存最新发布产物，不保存源码。

- Version: $VERSION
- GitHub Release: https://github.com/$GH_REPO/releases/tag/$VERSION
- Source commit: $(git -C "$ROOT_DIR" rev-parse --short HEAD)

校验文件: $(basename "$CHECKSUM_PATH")
EOF

    git add -A
    if git diff --cached --quiet; then
      echo ">> release 分支内容未变化"
      return
    fi

    git commit -m "Release $VERSION" >/dev/null
    if [[ "$RELEASE_BRANCH_EXISTS" -eq 1 ]]; then
      git push --force-with-lease origin "HEAD:$RELEASE_BRANCH"
    else
      git push origin "HEAD:$RELEASE_BRANCH"
    fi
  )
}

parse_args "$@"
require_command git
resolve_git_remote
resolve_github_repo
detect_release_publisher
ensure_version
confirm_release
trap cleanup EXIT

build_artifacts
collect_artifacts
generate_checksums
generate_release_notes
publish_release_branch
prepare_tag
if [[ "$RELEASE_PUBLISHER" == "api" ]]; then
  publish_github_release_api
else
  publish_github_release
fi

echo
echo "=========================================="
echo "  发布完成: $VERSION"
if [[ "$RELEASE_PUBLISHER" == "workflow" ]]; then
  echo "  GitHub Release: 由远端工作流发布 https://github.com/$GH_REPO/releases/tag/$VERSION"
else
  echo "  GitHub Release: https://github.com/$GH_REPO/releases/tag/$VERSION"
fi
echo "  Release Branch: $RELEASE_BRANCH"
echo "=========================================="
