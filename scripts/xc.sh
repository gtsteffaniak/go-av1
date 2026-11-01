#!/usr/bin/env bash
set -euo pipefail
export GOEXPERIMENT="${GOEXPERIMENT-simd}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="linux/amd64 linux/arm64 linux/arm linux/386 linux/riscv64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64"
r=""
for t in $T; do
  GOOS="${t%/*}" GOARCH="${t#*/}" GOARM=7 CGO_ENABLED=0 \
    go build -C "$ROOT" ./... >/tmp/xc_goav1.log 2>&1 && r="$r ok" || r="$r FAIL:$t"
done
echo "go-av1:$r"
