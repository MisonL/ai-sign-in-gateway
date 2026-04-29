#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGE_NAME="${IMAGE_NAME:-ai-sign-in-gateway}"
IMAGE_TAG="${IMAGE_TAG:-local}"

if ! command -v docker >/dev/null 2>&1; then
  echo "缺少命令: docker"
  exit 1
fi

echo "开始构建 Docker 镜像: ${IMAGE_NAME}:${IMAGE_TAG}"
docker build -t "${IMAGE_NAME}:${IMAGE_TAG}" "$ROOT_DIR"
echo "构建完成: ${IMAGE_NAME}:${IMAGE_TAG}"
