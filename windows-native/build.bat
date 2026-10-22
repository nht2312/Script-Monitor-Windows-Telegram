@echo off
REM Build WinMonitor.exe (native Go) tren Windows. Can cai Go: https://go.dev/dl/
cd /d %~dp0
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags="-s -w -H windowsgui" -o WinMonitor.exe .
echo OK -^> %cd%\WinMonitor.exe
