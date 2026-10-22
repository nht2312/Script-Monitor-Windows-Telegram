# QA Checklist — Kiểm thử trên Windows thật

> Unit test (Go + Python) và cross-compile đã tự động hoá. Nhưng các API Win32
> (GDI screenshot, BlockInput, keyboard hook, shortcut, schtasks) **phải kiểm thử
> thủ công trên Windows thật** vì môi trường CI/Linux không chạy được.
> Đánh dấu ✅ sau mỗi lần kiểm tra.

## Chuẩn bị
- [ ] Windows 10 hoặc 11 (thử cả 2 nếu có).
- [ ] Có 1 tài khoản Admin để test `/block`.
- [ ] Bot Telegram đã tạo, có token + chat id.

## A. Khởi động & cấu hình
- [ ] Chạy `WinMonitor.exe --token ... --chat ...` lần đầu → nhận tin "đã khởi động".
- [ ] File `%APPDATA%\WinMonitor\config.json` được tạo, **ẩn** (attrib).
- [ ] Nhắn `/start` → hiện help + inline keyboard.
- [ ] Menu lệnh (nút ☰) hiển thị đúng danh sách.

## B. Giám sát
- [ ] `/shot` → nhận ảnh chụp đúng màn hình.
- [ ] Đợi tới chu kỳ (đặt `/interval 60`) → tự gửi ảnh.
- [ ] Đổi cửa sổ → nhận thông báo "Cửa sổ".
- [ ] Mở app mới → nhận "Tiến trình mới".
- [ ] `/clip on` rồi copy text → nhận clipboard.
- [ ] `/keylog on` rồi gõ → nhận phím gõ (thử cả hook lẫn fallback).

## C. Đa màn hình / DPI / RDP
- [ ] 2 màn hình DPI khác nhau → ảnh chụp đủ, đúng kích thước.
- [ ] Qua RDP → `/status` báo RDP, ảnh chụp được (hoặc biết giới hạn).

## D. Điều khiển
- [ ] `/lock` → màn hình khóa.
- [ ] `/block` (chạy Admin) → bàn phím+chuột đứng; `/unblock` → hoạt động lại.
- [ ] `/block` không auto → tự mở sau tối đa 30 phút (van an toàn).
- [ ] Kill tiến trình khi đang block → bàn phím/chuột **vẫn dùng lại được** sau khi khởi động lại phiên (không kẹt vĩnh viễn).

## E. OTP
- [ ] `/shutdown` → nhận mã, `/confirm sai` → báo sai + đếm lần.
- [ ] Sai 5 lần → hủy yêu cầu.
- [ ] `/confirm đúng` → tắt máy sau 5s.
- [ ] `/stop` đúng mã → gỡ khỏi Startup/Registry/Scheduled Task và thoát.

## F. Tự khởi động & phục hồi
- [ ] Khởi động lại Windows → app tự chạy lại.
- [ ] Kill tiến trình → trong ≤5 phút Scheduled Task bật lại.
- [ ] Kiểm tra Task Scheduler có `WinMonitorGuard`, `WinMonitorLogon`.
- [ ] `HKCU\...\Run` có `WinMonitor`.

## G. Mạng
- [ ] Ngắt mạng 10 phút → không crash; có mạng lại → gửi tiếp (không mất ảnh mới).
- [ ] Chạy 2 máy cùng token → log báo 409 Conflict.

## H. Antivirus
- [ ] Windows Defender không chặn (nếu chặn → thêm exclusion / ký số).
- [ ] SmartScreen: đã ký số → không cảnh báo (nếu có chứng chỉ).

## I. Gỡ cài đặt
- [ ] `/stop` xác thực → không còn task/registry/file.
- [ ] Khởi động lại → app KHÔNG tự chạy nữa.
