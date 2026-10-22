#!/usr/bin/env bash
# Build WinMonitor.exe (native Go) cho Windows amd64 + arm64 từ Linux/macOS.
# Cross-compile, KHÔNG cần máy Windows.
set -e
cd "$(dirname "$0")"
export PATH="$PATH:/usr/local/go/bin"
export GOOS=windows
export CGO_ENABLED=0

build() {
  local arch="$1"
  local out="$2"
  GOARCH="$arch" go build -trimpath -ldflags="-s -w -H windowsgui" -o "$out" .
  echo "OK -> $out"
}

# amd64 (phổ biến nhất) — cũng là tên mặc định WinMonitor.exe
build amd64 WinMonitor.exe
# arm64 (Surface / laptop Snapdragon) — issue #28
build arm64 WinMonitor-arm64.exe

# Checksum để xác minh toàn vẹn (issue #22)
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum WinMonitor.exe WinMonitor-arm64.exe > SHA256SUMS.txt
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 WinMonitor.exe WinMonitor-arm64.exe > SHA256SUMS.txt
fi
echo "--- SHA256SUMS ---"; cat SHA256SUMS.txt 2>/dev/null || true
file WinMonitor.exe WinMonitor-arm64.exe 2>/dev/null || true
