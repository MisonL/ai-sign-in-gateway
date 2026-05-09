#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$TMP_DIR/scripts" "$TMP_DIR/bin"
cp "$ROOT_DIR/scripts/build-server-single.sh" "$TMP_DIR/scripts/build-server-single.sh"
chmod +x "$TMP_DIR/scripts/build-server-single.sh"

cat >"$TMP_DIR/scripts/build-desktop-single.sh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
mkdir -p "$(dirname "$OUTPUT_PATH")"
printf 'server-binary' >"$OUTPUT_PATH"
STUB
chmod +x "$TMP_DIR/scripts/build-desktop-single.sh"

cat >"$TMP_DIR/bin/scp" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >"$SCP_LOG"
STUB
chmod +x "$TMP_DIR/bin/scp"

cat >"$TMP_DIR/.deploy.config" <<'CONFIG'
host=example.com
path=/opt/ai-sign-in-gateway/ai-sign-in-gateway
CONFIG

SCP_LOG="$TMP_DIR/scp.log" \
PATH="$TMP_DIR/bin:$PATH" \
BUILD_SERVER_SINGLE_FORCE_TUI=true \
bash "$TMP_DIR/scripts/build-server-single.sh" <<<"1"

if [[ ! -s "$TMP_DIR/scp.log" ]]; then
  echo "expected scp to be called"
  exit 1
fi

scp_args="$(cat "$TMP_DIR/scp.log")"

case "$scp_args" in
  *"-P 22"*) ;;
  *)
    echo "expected scp to use default port 22, got: $scp_args"
    exit 1
    ;;
esac

case "$scp_args" in
  *"$TMP_DIR/.release/ai-sign-in-gateway-server-linux-amd64"*) ;;
  *)
    echo "expected scp source artifact, got: $scp_args"
    exit 1
    ;;
esac

case "$scp_args" in
  *"example.com:/opt/ai-sign-in-gateway/ai-sign-in-gateway"*) ;;
  *)
    echo "expected scp remote target, got: $scp_args"
    exit 1
    ;;
esac
