# tool_mahoa — Mã hóa script Python thành `.pyw` tự chạy

Công cụ biến `win_monitor.pyw` (chứa toàn bộ code + token) thành một file **đã mã
hóa** `win_monitor_mahoa.pyw`. File mã hóa **vẫn chạy y như file gốc** nhưng mở
bằng Notepad chỉ thấy dữ liệu mã hóa — không đọc được code, không lộ token.

## Cơ chế

- Mã hóa bằng **AES-tự-xây từ thư viện chuẩn**: PBKDF2-SHA256 (200.000 vòng) để
  tạo khóa + dòng khóa HMAC-SHA256 (counter mode) + HMAC xác thực (chống sửa file).
- Khi chạy, file giải mã **trong bộ nhớ** rồi `exec()` — không ghi code ra đĩa.
- Thuần thư viện chuẩn Python, **không cần cài thêm gì**.

## 2 chế độ

| Chế độ | Lệnh | Ưu / Nhược |
|---|---|---|
| `source` (mặc định) | `python ma_hoa.py win_monitor.pyw` | Chạy trên **mọi** Python 3.x. Code gốc nằm trong payload đã mã hóa. |
| `compiled` | `python ma_hoa.py win_monitor.pyw --compile` | Nhúng **bytecode** (khó dịch ngược hơn), nhưng **phải cùng phiên bản Python** khi mã hóa và khi chạy. |

## Cách dùng

```bash
# 1) Tạo file mã hóa (sẽ hỏi mật khẩu 2 lần)
python tool_mahoa/ma_hoa.py win_monitor.pyw
#   -> tạo win_monitor_mahoa.pyw

# Hoặc truyền mật khẩu trực tiếp / đặt tên file khác
python tool_mahoa/ma_hoa.py win_monitor.pyw -o win_monitor_mahoa.pyw -p "MatKhauCuaBan"

# Chế độ compiled
python tool_mahoa/ma_hoa.py win_monitor.pyw --compile -p "MatKhauCuaBan"

# Kiểm tra nội bộ
python tool_mahoa/ma_hoa.py --selftest
```

## Mật khẩu được lấy từ đâu khi chạy file mã hóa?

Theo thứ tự ưu tiên:
1. Biến môi trường `WINMON_KEY`
2. File **`.env`** (định dạng `WINMON_KEY=...`) — tìm ở: cạnh file `.pyw`,
   thư mục hiện tại, `%APPDATA%\WinMonitor\.env`, `~/.win_monitor.env`
3. Nếu chạy có console → hỏi mật khẩu
4. Nếu chạy nền (không console) mà không có nguồn nào → báo lỗi và thoát

> **Quan trọng:** Vì file chạy từ Startup là chạy **nền (không console)**, bạn cần
> đặt mật khẩu trong `.env` hoặc biến `WINMON_KEY`. Xem `.env.example`.

## Quy trình triển khai trên máy Windows

1. Tạo `win_monitor_mahoa.pyw` bằng tool với mật khẩu của bạn.
2. Tạo file `.env` cạnh nó:
   ```
   WINMON_KEY=MatKhauCuaBan
   ```
   (đặt thuộc tính **Hidden** cho `.env` nếu muốn — xem mục "Ẩn file" ở README gốc)
3. Double-click `win_monitor_mahoa.pyw` (hoặc chạy `pythonw win_monitor_mahoa.pyw`).
4. Script sẽ tự copy **bản đã mã hóa** vào `%APPDATA%\WinMonitor\` và Startup dưới
   dạng **file ẩn** `.win_monitor.pyw`, đồng thời đăng ký tự khởi động.

> File `.env` chứa mật khẩu → **đừng commit** (đã nằm trong .gitignore).

## Lưu ý bảo mật (trung thực)

- Đây là **che giấu + mã hóa mã nguồn ở mức thực dụng**, KHÔNG phải bảo mật tuyệt đối:
  - Nếu kẻ tấn công có **quyền Administrator** trên máy và biết cách, họ vẫn có thể
    chặn/đọc bộ nhớ tiến trình lúc giải mã. Không có cách nào chống tuyệt đối khi
    chạy trên máy của kẻ đó.
  - Mật khẩu nằm trong `.env`/biến môi trường → ai đọc được `.env` thì giải mã được.
    Vì vậy cần đặt quyền file chặt + ẩn file.
- Mục tiêu chính: **không lộ code/token khi mở file**, và **không ai tắt/dừng script
  mà không có OTP qua Telegram** (xem README gốc).
