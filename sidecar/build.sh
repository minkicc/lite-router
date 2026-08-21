#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST="$ROOT/../src-tauri/binaries"
mkdir -p "$DIST"
cd "$ROOT"

build() {
  local goos="$1"
  local goarch="$2"
  local triple="$3"
  local ext=""
  [[ "$goos" == "windows" ]] && ext=".exe"
  local name="lite-router-${triple}${ext}"
  echo "building $name"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w" -o "$DIST/$name" .
}

build windows amd64 x86_64-pc-windows-msvc
build darwin amd64 x86_64-apple-darwin
build darwin arm64 aarch64-apple-darwin
build linux amd64 x86_64-unknown-linux-gnu
build linux arm64 aarch64-unknown-linux-gnu

echo "done -> $DIST"
