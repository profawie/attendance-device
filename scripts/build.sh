#!/usr/bin/env bash
# Cross-compile for Linux and Windows. Output goes to dist/.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
for target in linux/amd64 linux/arm64 windows/amd64; do
  os=${target%/*}; arch=${target#*/}
  out="dist/attendance-device-$os-$arch"
  [ "$os" = windows ] && out="$out.exe"
  echo "building $out"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$out" .
done
