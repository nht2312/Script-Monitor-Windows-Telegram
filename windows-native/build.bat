@echo off
REM Build WinMonitor.exe (native Go) tren Windows cho amd64 + arm64. Can cai Go.
cd /d %~dp0
set GOOS=windows
set CGO_ENABLED=0

set GOARCH=amd64
go build -trimpath -ldflags="-s -w -H windowsgui" -o WinMonitor.exe .
echo OK -^> %cd%\WinMonitor.exe

set GOARCH=arm64
go build -trimpath -ldflags="-s -w -H windowsgui" -o WinMonitor-arm64.exe .
echo OK -^> %cd%\WinMonitor-arm64.exe

certutil -hashfile WinMonitor.exe SHA256 > SHA256SUMS.txt
certutil -hashfile WinMonitor-arm64.exe SHA256 >> SHA256SUMS.txt
echo --- SHA256SUMS ---
type SHA256SUMS.txt
