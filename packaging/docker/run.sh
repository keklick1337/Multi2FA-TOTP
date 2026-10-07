#!/bin/sh
# run.sh OUTDIR DOCKERFILE BASE_IMAGE SCRIPT [ARCH]
# Builds (or reuses) the build image for BASE_IMAGE and runs SCRIPT in it. ARCH selects a cross
# target (arm64, armhf, aarch64); empty or the host architecture builds natively.
# Every image gets its own Go build cache because cgo objects depend on the system headers;
# the module cache is shared between all of them.
set -eu

OUT=$1 DOCKERFILE=$2 BASE=$3 SCRIPT=$4 ARCH=${5:-}
DOCKER=${DOCKER:-docker}
GO_VERSION=${GO_VERSION:-1.26.8}
case "$ARCH" in amd64|x86_64) ARCH="" ;; esac
NAME=$(echo "$BASE-${DOCKERFILE%.Dockerfile}${ARCH:+-$ARCH}" | tr ':/' '--')
IMAGE="multi2fa-build:$NAME"

echo "==> $BASE ${ARCH:-native}: $SCRIPT"
$DOCKER build -q -t "$IMAGE" --build-arg BASE="$BASE" --build-arg GO_VERSION="$GO_VERSION" --build-arg CROSS="$ARCH" \
    -f "packaging/docker/$DOCKERFILE" packaging/docker >/dev/null

mkdir -p "$OUT"
$DOCKER run --rm \
    -v "$(pwd)":/src:ro \
    -v "$(cd "$OUT" && pwd)":/out \
    -v multi2fa-gomod:/root/go/pkg/mod \
    -v "multi2fa-gocache-$NAME":/root/.cache/go-build \
    -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" -e CLI_TARGETS="${CLI_TARGETS:-}" -e BUILD="${BUILD:-1}" \
    "$IMAGE" sh "/src/packaging/docker/$SCRIPT"
