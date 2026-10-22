#!/usr/bin/env bash
# Build WinMonitor.exe (native Go) từ Linux/macOS — cross-compile, KHÔNG cần Windows.
set -e
cd "$(dirname "$0")"
export PATH="$PATH:/usr/local/go/bin"
export GOOS=windows
export GOARCH=amd64
export CGO_ENABLED=0
go build -trimpath -ldflags="-s -w -H windowsgui" -o WinMonitor.exe .
echo "OK -> $(pwd)/WinMonitor.exe"
file WinMonitor.exe 2>/dev/null || true
