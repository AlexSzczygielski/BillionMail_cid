#!/bin/sh
# build-core.sh — rebuild the patched BillionMail core image
# Usage: ./build-core.sh [version] [x86|arm]
set -e

VERSION="${1:-4.9.3}"
ARCH="${2:-x86}"
REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
CTX_DIR="/tmp/core-ctx"
IMAGE_NAME="billionmail-core-cid:${VERSION}"

case "$ARCH" in
  x86) BINARY="core/billionmail-amd64" ;;
  arm) BINARY="core/billionmail-arm64" ;;
  *) echo "ERROR: arch must be x86 or arm"; exit 1 ;;
esac

echo "=================================================="
echo "BUILD START — version=${VERSION} arch=${ARCH}"
echo "=================================================="

echo ""
echo "[1/7] Removing any leftover build container from a previous run..."
docker rm -f p-g-alpine >/dev/null 2>&1 || true
echo "      done."

echo ""
echo "[2/7] Starting throwaway Alpine container to compile the Go binary..."
cd "$REPO_DIR"
docker run -d --name p-g-alpine --hostname p-g-alpine \
  -v ./core:/opt/core \
  -v ./Dockerfiles/core/repositories:/etc/apk/repositories \
  alpine:3.20 tail -f /dev/null
echo "      container started."

echo ""
echo "[3/7] Compiling (this is the slow step — go mod tidy + go build)..."
docker exec p-g-alpine sh -c "
  export GOTOOLCHAIN=auto
  cd /opt/core
  sh go-build.sh ${ARCH}
"
echo "      compile finished."

echo ""
echo "[4/7] Cleaning up the build container..."
docker stop p-g-alpine >/dev/null
docker rm p-g-alpine >/dev/null
echo "      container removed."

if [ ! -f "$BINARY" ]; then
    echo ""
    echo "ERROR: expected binary not found at $BINARY — aborting."
    exit 1
fi
echo ""
echo "[5/7] Binary confirmed: $BINARY"
ls -la "$BINARY"

echo ""
echo "[6/7] Staging Docker build context at ${CTX_DIR}..."
rm -rf "$CTX_DIR"
mkdir -p "$CTX_DIR/core"
cp Dockerfiles/core/*.sh Dockerfiles/core/*.conf "$CTX_DIR/"
echo "      copied helper scripts + configs"
cp "$BINARY" "$CTX_DIR/core/"
echo "      copied binary"
cp -r core/manifest core/languages core/public core/resource core/template "$CTX_DIR/core/"
echo "      copied static assets (manifest, languages, public, resource, template)"
echo "      context ready:"
ls -la "$CTX_DIR" "$CTX_DIR/core"

echo ""
echo "[7/7] Running docker build -> ${IMAGE_NAME}..."
docker build -t "$IMAGE_NAME" -f Dockerfiles/core/Dockerfile "$CTX_DIR"

echo ""
echo "=================================================="
echo "BUILD COMPLETE — ${IMAGE_NAME}"
echo "=================================================="
docker images | grep billionmail-core-cid
