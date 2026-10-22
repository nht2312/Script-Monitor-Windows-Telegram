# WinMonitor — Giám sát toàn bộ máy Windows qua Telegram

Một file script duy nhất (`win_monitor.pyw`) chạy nền, tự khởi động cùng Windows,
chụp màn hình định kỳ và gửi qua Telegram, đồng thời cho phép điều khiển ngược
lại từ Telegram (tắt máy, ngắt/kết nối bàn phím–chuột, đổi chu kỳ chụp...).
Mọi thao tác nguy hiểm đều phải xác nhận bằng **mật khẩu một lần (OTP)**.

> Chỉ cần **Python 3** cho Windows. Không cần cài thêm thư viện nào.

## Các file trong repo

| File | Mô tả |
|---|---|
| `win_monitor.pyw` | **Script gốc** — toàn bộ code, dùng để đọc/sửa |
| `win_monitor_mahoa.pyw` | **Bản đã mã hóa** của script gốc — dùng để triển khai (không lộ code/token) |
| `tool_mahoa/` | Công cụ mã hóa + tài liệu (xem `tool_mahoa/README.md`) |
| `.gitignore` | Bỏ qua `config.json`, `.env`, log, ảnh chụp... |

---

## 1. Cài đặt (làm 1 lần)

### 1.1. Cài Python 3
- Tải tại https://www.python.org/downloads/windows/ (bản 3.8+).
- Khi cài, **tích** ô **"Add python.exe to PATH"**.

### 1.2. Tạo bot Telegram & lấy Chat ID
1. Mở Telegram, nhắn **@BotFather** → gõ `/newbot` → đặt tên → nhận **BOT TOKEN**
   (dạng `123456789:AAE...`).
2. Lấy **Chat ID** của bạn: nhắn cho **@userinfobot** (hoặc **@getidsbot**) → nó trả
   về `Id: 123456789`.

### 1.3. Điền cấu hình
Mở `win_monitor.pyw` bằng Notepad, sửa 2 dòng gần đầu file:

```python
DEFAULT_BOT_TOKEN = "123456789:AAE...token_cua_ban"
DEFAULT_CHAT_ID   = "123456789"      # chat id cua ban
```

> **Cách nhanh hơn (không cần sửa file):** mở CMD và chạy
> ```
> pythonw win_monitor.pyw --token 123456789:AAE... --chat 123456789
> ```

### 1.4. Chạy
- **Double-click** `win_monitor.pyw` (đuôi `.pyw` nên không hiện cửa sổ đen).
- Lần đầu chạy, script sẽ:
  - Tự copy vào `%APPDATA%\WinMonitor\.win_monitor.pyw` (file **ẩn**)
  - Tự copy vào thư mục **Startup** dưới dạng file **ẩn** (`.win_monitor.pyw`)
  - Đăng ký **Registry Run** + **2 Scheduled Task** (để tự phục hồi nếu bị tắt)
- Mở Telegram, nhắn **`/start`** cho bot để bắt đầu.

### 1.5. (Khuyến nghị) Chạy quyền Administrator
Để tính năng **ngắt bàn phím–chuột** (`BlockInput`) hoạt động ổn định, nên chạy
script bằng quyền Admin. Có thể chuột phải → *Run as administrator* một lần; các
lần khởi động sau vẫn tự chạy nền.

---

## 2. Danh sách lệnh Telegram

### Giám sát
| Lệnh | Tác dụng |
|---|---|
| `/status` | Xem trạng thái hiện tại |
| `/shot` | Chụp màn hình ngay |
| `/interval <giây>` | Đổi chu kỳ chụp (vd `/interval 120` = 2 phút) |
| `/screen on\|off` | Bật/tắt chụp định kỳ |
| `/activity on\|off` | Báo khi cửa sổ đang dùng thay đổi |
| `/process on\|off` | Báo khi có tiến trình mới mở |
| `/clip on\|off` | Theo dõi clipboard |
| `/keylog on\|off` | Ghi lại phím gõ (gửi định kỳ) |

### Điều khiển
| Lệnh | Tác dụng |
|---|---|
| `/block [phút]` | **Ngắt bàn phím & chuột** (tự mở lại sau N phút nếu có) |
| `/unblock` | **Kết nối lại** bàn phím & chuột |
| `/lock` | Khóa màn hình |

### Nguy hiểm — cần OTP
| Lệnh | Tác dụng |
|---|---|
| `/shutdown` | Tắt máy (gửi OTP xác nhận) |
| `/restart` | Khởi động lại (gửi OTP xác nhận) |
| `/stop` | Dừng giám sát & gỡ khỏi khởi động cùng Windows (gửi OTP) |
| `/confirm <mã>` | Xác nhận mã OTP |
| `/cancel` | Hủy yêu cầu đang chờ |

### Khác
| Lệnh | Tác dụng |
|---|---|
| `/config` | Xem cấu hình |
| `/set <khóa> <giá trị>` | Đổi cấu hình trực tiếp |
| `/logs [n]` | Xem log gần đây |

---

## 3. Cơ chế OTP (mật khẩu một lần)

Khi bạn gõ `/shutdown`, `/restart` hoặc `/stop`:
1. Bot gửi một **mã 6 số** dùng **1 lần**, hết hạn sau **120 giây**.
2. Bạn gõ `/confirm <mã>` để thực thi.
3. Sai mã hoặc quá hạn → phải yêu cầu lại.

> Vì vậy dù ai đó (kể cả AI agent) có được quyền nhắn bot, **cũng không thể tắt máy
> hay dừng giám sát** nếu không có mã OTP hiển thị trên Telegram của **chủ máy**.

---

## 4. Tự khởi động & tự phục hồi

Script đăng ký nhiều lớp để "không thể tắt":
- `HKCU\...\Run` → chạy khi đăng nhập.
- Scheduled Task **WinMonitorLogon** → chạy khi đăng nhập.
- Scheduled Task **WinMonitorGuard** → chạy **mỗi 1 phút**, tự bật lại nếu bị kill.
- Bản sao trong thư mục **Startup**.

Chỉ lệnh `/stop` (đã xác thực OTP) mới gỡ bỏ toàn bộ các lớp này.

> Lưu ý: nếu ai đó có quyền Administrator trên máy, họ vẫn có thể gỡ thủ công qua
> Task Scheduler / Registry. Không có phần mềm nào chống được admin cục bộ tuyệt đối.

---

## 5. Cấu hình (file `%APPDATA%\WinMonitor\config.json`)

| Khóa | Mặc định | Ý nghĩa |
|---|---|---|
| `screenshot_interval` | `300` | Chu kỳ chụp (giây) |
| `screenshots_enabled` | `true` | Bật chụp định kỳ |
| `activity_monitor` | `true` | Báo đổi cửa sổ |
| `process_monitor` | `true` | Báo tiến trình mới |
| `clipboard_monitor` | `false` | Theo dõi clipboard |
| `keylogger` | `false` | Ghi phím gõ |
| `keylog_flush` | `60` | Giây/lần gửi phím gõ |
| `block_safety_minutes` | `0` | Tự mở khóa bàn phím/chuột sau N phút (0 = không) |
| `heartbeat_interval` | `3600` | Giây/lần báo "vẫn đang chạy" |

Đổi bất kỳ giá trị nào qua Telegram bằng `/set <khóa> <giá trị>` — script tự cập nhật
ngay, không cần khởi động lại.

---

## 6. Ẩn file & mã hóa script (nâng cao)

### 6.1. Tự động ẩn file
Khi chạy, script tự đặt thuộc tính **Hidden + System** cho:
- Chính file đang chạy
- Bản sao `%APPDATA%\WinMonitor\.win_monitor.pyw`
- Bản sao trong Startup: `.win_monitor.pyw`

Nhờ tên bắt đầu bằng dấu chấm (`.`) và thuộc tính ẩn, file gần như vô hình trong
File Explorer và giống kiểu file cấu hình `.env`.

> Để xem lại file ẩn: trong File Explorer bật *View → Show → Hidden items*, hoặc chạy
> `attrib -h -s "%APPDATA%\WinMonitor\.win_monitor.pyw"` trong CMD.

### 6.2. Mã hóa script thành `win_monitor_mahoa.pyw`
Dùng công cụ trong `tool_mahoa/` để tạo bản mã hóa (mở Notepad không thấy code/token,
nhưng vẫn chạy y như file gốc):

```bash
python tool_mahoa/ma_hoa.py win_monitor.pyw -p "MatKhauCuaBan"
# -> tạo win_monitor_mahoa.pyw (đã mã hóa)
```

Vì file chạy nền từ Startup không có console để hỏi mật khẩu, hãy đặt mật khẩu trong
file `.env` cạnh script (xem `tool_mahoa/.env.example`):

```
WINMON_KEY=MatKhauCuaBan
```

Chi tiết đầy đủ + 2 chế độ (`source`/`compiled`) + cảnh báo bảo mật: xem
**`tool_mahoa/README.md`**.

---

## 7. Gỡ cài đặt hoàn toàn

1. Gửi `/stop` trên Telegram → nhập `/confirm <mã>` → script tự gỡ khỏi Startup,
   Registry và Scheduled Task rồi thoát.
2. (Tùy chọn) Xóa thư mục `%APPDATA%\WinMonitor`.

---

## 8. Bảo mật

- **Token** nằm trong file → nên đặt quyền file chỉ user hiện tại đọc được.
- Chỉ **Chat ID** đã cấu hình mới điều khiển được bot.
- OTP hiển thị **chỉ trên Telegram của chủ máy** → kẻ khác không tắt được máy.
- Dùng bản mã hóa `win_monitor_mahoa.pyw` để không lộ code/token khi mở file.

---

## 9. Khắc phục sự cố

| Hiện tượng | Cách xử lý |
|---|---|
| Bot không trả lời | Kiểm tra token/chat id; chạy `python win_monitor.pyw` để xem lỗi |
| Không chụp được màn hình | Chạy thử PowerShell; đảm bảo có `powershell` trong PATH |
| `/block` báo không ngắt được | Chạy script bằng quyền **Administrator** |
| Muốn xem log | Gửi `/logs` trên Telegram, hoặc mở `%APPDATA%\WinMonitor\monitor.log` |
