#!/usr/bin/env bash
# Build and push the OpenZerg PI attacker image to DigitalOcean Container
# Registry. Intended to be run from the repo root.
#
# Prereqs:
#   - doctl is installed and authenticated (`doctl auth init`).
#   - docker buildx is available (Docker 20.10+).
#   - The user has push rights on registry.digitalocean.com/openzerg.
#
# Output: pushes registry.digitalocean.com/openzerg/pi-attacker:latest
# (linux/amd64 — DigitalOcean managed Kubernetes nodes are x86_64).

set -euo pipefail

IMAGE="${IMAGE:-registry.digitalocean.com/openzerg/pi-attacker:latest}"
CONTEXT_DIR="$(cd "$(dirname "$0")/.." && pwd)/backend/docker/pi-attacker"

if [ ! -f "${CONTEXT_DIR}/Dockerfile" ]; then
  echo "error: Dockerfile not found at ${CONTEXT_DIR}/Dockerfile" >&2
  exit 1
fi

echo "==> doctl registry login"
doctl registry login

echo "==> docker buildx build --push ${IMAGE}"
docker buildx build \
  --platform=linux/amd64 \
  -t "${IMAGE}" \
  --push \
  "${CONTEXT_DIR}"

echo "==> done. pushed ${IMAGE}"
