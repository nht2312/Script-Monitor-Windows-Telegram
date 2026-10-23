# WinMonitor — Giám sát toàn bộ máy Windows qua Telegram

Công cụ giám sát & điều khiển máy Windows từ xa qua **Telegram**: chụp màn hình định kỳ,
theo dõi cửa sổ/tiến trình/clipboard/keylogger, tắt máy, khóa màn hình, ngắt/kết nối lại
bàn phím–chuột… Mọi thao tác nguy hiểm đều phải xác nhận bằng **mật khẩu một lần (OTP)**
gửi qua Telegram. Script **tự khởi động cùng Windows và tự phục hồi** nếu bị tắt.

> Có **2 phiên bản** cùng tính năng:
> - **Go native (`WinMonitor.exe`)** ⭐ — máy đích **KHÔNG cần Python**, ít bị antivirus báo nhầm. **Khuyến nghị.**
> - **Python (`win_monitor.pyw`)** — để đọc/sửa code, chạy được mọi Python 3.

---

## Mục lục
1. [Cấu trúc repo](#1-cấu-trúc-repo)
2. [Hai phiên bản: Python vs Go](#2-hai-phiên-bản-python-vs-go)
3. [Kiến trúc & luồng hoạt động (FLOW)](#3-kiến-trúc--luồng-hoạt-động-flow)
4. [Luồng xử lý một lệnh Telegram](#4-luồng-xử-lý-một-lệnh-telegram)
5. [Máy trạng thái: OTP & BlockInput](#5-máy-trạng-thái-otp--blockinput)
6. [Cài đặt & chạy](#6-cài-đặt--chạy)
7. [Danh sách lệnh Telegram](#7-danh-sách-lệnh-telegram)
8. [Cấu hình (và cách sửa)](#8-cấu-hình-và-cách-sửa)
9. [Cơ chế tự khởi động & tự phục hồi](#9-cơ-chế-tự-khởi-động--tự-phục-hồi)
10. [Bảo mật & mô hình đe dọa](#10-bảo-mật--mô-hình-đe-dọa)
11. [Build & phát hành](#11-build--phát-hành)
12. [Khắc phục sự cố](#12-khắc-phục-sự-cố)
13. [Kiểm thử](#13-kiểm-thử)
14. [Mục đích sử dụng & pháp lý](#14-mục-đích-sử-dụng--pháp-lý)

---

## 1. Cấu trúc repo

```
.
├── windows-native/          ⭐ Bản Go native (WinMonitor.exe) — không cần Python
│   ├── main.go              # Toàn bộ logic (config, telegram, monitors, commands, persistence)
│   ├── platform_windows.go  # Syscall Win32: GDI screenshot, BlockInput, keyboard hook, ACL...
│   ├── platform_other.go    # Stub để build/test trên Linux
│   ├── main_test.go         # Unit test (go test)
│   ├── build.sh / build.bat # Build amd64 + arm64 + checksum
│   ├── WinMonitor.exe       # Bản build sẵn (amd64)
│   └── WinMonitor-arm64.exe # Bản build sẵn (arm64)
│
├── win_monitor.pyw          # Bản Python (1 file, không cần thư viện ngoài)
├── tool_mahoa/              # Mã hoá file .pyw thành bản chạy được (mật khẩu)
├── tool_build/              # Đóng gói bản Python thành .exe (PyInstaller) + manifest/sign
├── tests/                   # Test tự động bản Python (unittest)
├── docs/                    # QA checklist, tài liệu phụ
├── .github/workflows/       # CI build .exe (Go native + Python)
├── winmonitor.config.example.json  # Mẫu cấu hình đóng gói cạnh .exe
├── LICENSE                  # MIT + điều khoản sử dụng hợp pháp
└── README.md
```

---

## 2. Hai phiên bản: Python vs Go

| Tiêu chí | Python (`win_monitor.pyw`) | **Go native (`WinMonitor.exe`)** ⭐ |
|---|---|---|
| Máy đích cần Python? | **Có** (hoặc đóng gói PyInstaller) | **KHÔNG** — chạy ngay |
| Kích thước | ~40KB (script) | ~6MB (đã gồm mọi thứ) |
| Antivirus báo nhầm | Cao (nếu đóng gói PyInstaller) | **Rất thấp** (PE native, không UPX/packer) |
| Runtime | Python 3 | Không (static binary) |
| Chụp màn hình | GDI qua ctypes (+ dự phòng PowerShell) | GDI trực tiếp |
| Keylogger | Polling `GetAsyncKeyState` | **Low-level hook** (`WH_KEYBOARD_LL`), dự phòng polling |
| Đa kiến trúc | — | amd64 + arm64 |
| Dùng khi nào | Muốn đọc/sửa code, máy có Python | **Triển khai thực tế (khuyến nghị)** |

> Hai bản dùng **cùng schema cấu hình**, cùng bộ lệnh Telegram, cùng cơ chế OTP/persistence.

---

## 3. Kiến trúc & luồng hoạt động (FLOW)

### 3.1. Sơ đồ tổng quát

```
                          ┌────────────────────────────┐
                          │        TELEGRAM BOT         │
                          │  (chủ máy nhắn lệnh / nhận  │
                          │   ảnh, cảnh báo, OTP)       │
                          └────────────▲───────┬────────┘
                                       │       │  HTTPS (api.telegram.org)
                            gửi tin/ảnh│       │getUpdates (long-poll 25s)
                                       │       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                          WinMonitor (1 tiến trình)                     │
│                                                                        │
│  [main] khởi động:                                                    │
│    hide console → load config → single-instance mutex                 │
│    → getMe (kiểm token) → start keyboard hook → ensure_persistence    │
│    → spawn các goroutine/thread:                                       │
│                                                                        │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌───────────┐ │
│   │ pollLoop     │  │ senderLoop   │  │screenshotLoop│  │ monitors  │ │
│   │ (nhận lệnh)  │  │ (gửi queue)  │  │ (định kỳ)    │  │ window/   │ │
│   └──────┬───────┘  └──────▲───────┘  └──────┬───────┘  │ process/  │ │
│          │ handleUpdate     │ notify()        │ chụp     │ clipboard/│ │
│          ▼                  │ (queue)         ▼          │ keylog    │ │
│   handleCommand ───────────┘            takeScreenshot   └───────────┘ │
│   (set/block/OTP/...)                     (GDI)                        │
│                                                                        │
│   ┌────────────────────┐                                               │
│   │ selfhealLoop       │  mỗi 5 phút: ensure_persistence + dọn ảnh cũ  │
│   └────────────────────┘                                               │
└──────────────────────────────────────────────────────────────────────┘
        │ ghi file                          │ đăng ký tự khởi động
        ▼                                   ▼
 %APPDATA%\WinMonitor\            Registry Run + 2 Scheduled Task
   config.json (ẩn, ACL)          + shortcut ẩn trong Startup
   monitor.log, shot_*.png
```

### 3.2. Trình tự khởi động (startup sequence)

1. **Ẩn console** (`-H windowsgui` / `hideConsole`).
2. **DPI awareness** — để chụp đúng trên đa màn hình DPI hỗn hợp.
3. **Nạp cấu hình** theo thứ tự ưu tiên:
   `%APPDATA%\WinMonitor\config.json` → nếu chưa có thì `winmonitor.config.json` cạnh `.exe`
   → rồi đè bằng `--config`, `WINMON_TOKEN`, `--token`, `--chat`.
4. **Single-instance mutex** — nếu đã có bản chạy thì thoát (tránh 2 tiến trình tranh `getUpdates`).
5. **`getMe`** — kiểm tra token hợp lệ; sai token (401) → dừng; lỗi mạng → vẫn tiếp tục.
6. **Cài keyboard hook** (Go) để keylogger chính xác.
7. **`ensure_persistence`** — copy bản thân vào `%APPDATA%` (ẩn), tạo shortcut ẩn trong Startup,
   đăng ký Registry `Run` + 2 Scheduled Task.
8. **Spawn các luồng** (xem sơ đồ): sender, screenshot, monitors, heartbeat, selfheal, poll.
9. Gửi tin "đã khởi động", rồi **chờ tín hiệu thoát** (main block ở `select`/vòng poll).

### 3.3. Vòng đời các luồng

| Luồng | Chu kỳ | Việc làm |
|---|---|---|
| `pollLoop` | liên tục (long-poll 25s) | `getUpdates` → `handleUpdate` → `handleCommand` |
| `senderLoop` | ~1.05s/tin | Lấy từ **queue** → gửi; retry khi lỗi mạng/429 (có backoff, tối đa 20 lần) |
| `screenshotLoop` | mỗi `screenshot_interval` giây | Chụp → đẩy ảnh vào queue (bỏ qua nếu queue đầy) |
| `windowMonitor` | 2s | Báo khi tiêu đề cửa sổ foreground đổi |
| `processMonitor` | 5s | Báo tiến trình mới (giới hạn 15/lần) |
| `clipboardMonitor` | 3s | Báo khi clipboard đổi (nếu bật) |
| `keyloggerLoop` | hook/30ms | Gom phím gõ, gửi mỗi `keylog_flush` giây |
| `heartbeatLoop` | `heartbeat_interval` | Báo "vẫn đang chạy" |
| `selfhealLoop` | 300s | Đăng ký lại persistence + dọn ảnh tạm cũ |

### 3.4. Luồng dữ liệu gửi tin (send pipeline)

Mọi thứ muốn gửi Telegram đều đi qua **queue có giới hạn** (`notify`/`notifyPhoto`):

```
notify(text) ─┐
notifyPhoto() ─┼─► SEND_QUEUE (tối đa ~300) ─► senderLoop ─► tgCall(sendMessage/sendPhoto)
               │      │ (đầy thì bỏ item CŨ nhất, xoá ảnh cũ)        │
               │      │                                               ├─ lỗi mạng → backoff rồi thử lại (≤20 lần)
               └──────┘                                               ├─ 429 → đọc retry_after, ngủ rồi gửi lại
                                                                       └─ ảnh > 9MB → gửi dạng sendDocument
```

Vì sao dùng queue: tách **người tạo sự kiện** khỏi **người gửi mạng** → giám sát không bị
block khi mạng chậm, và tuân thủ giới hạn ~1 tin/giây của Telegram.

---

## 4. Luồng xử lý một lệnh Telegram

```
Bạn gõ "/shot"
   │
   ▼
Telegram server
   │ getUpdates (pollLoop) trả về message
   ▼
handleUpdate(upd)
   ├─ nếu là callback_query (bấm nút inline) → answerCallbackQuery + coi data như lệnh
   └─ lấy chat_id, text
        │
        ▼
   isAuthorized(chat_id)? ── KHÔNG ─► log "Unauthorized", bỏ qua
        │ CÓ (đúng chat chủ, hoặc auto-bind lần đầu)
        ▼
   handleCommand(text, chat_id)
        ├─ "/shot"  → notify("⏳...") + go doScreenshotSend()
        ├─ "/set"   → applySet → cfgUpdate (validate clamp) → save_config
        ├─ "/block" → StartBlock(auto, tối đa 30') ...
        ├─ "/shutdown" → requestOtp("shutdown") → gửi mã
        └─ "/confirm <mã>" → nếu khớp & chưa hết hạn → executeAction()
   │
   ▼
notify()/notifyPhoto() → SEND_QUEUE → senderLoop → Telegram → bạn nhận kết quả
```

**Điểm quan trọng:** mọi kiểm tra quyền (chat_id) diễn ra **trước** khi thực thi lệnh; OTP
là lớp thứ hai cho thao tác nguy hiểm.

---

## 5. Máy trạng thái: OTP & BlockInput

### 5.1. OTP (mật khẩu một lần)

```
   /shutdown /restart /stop
        │
        ▼
   requestOtp(action)  → sinh mã 6 số (CSPRNG) → lưu {code, expires = now+120s}
        │                                         → gửi mã qua Telegram
        ▼
   chờ /confirm <mã>
        ├─ mã SAI  → tăng đếm lỗi; đủ 5 lần → hủy toàn bộ OTP
        ├─ mã ĐÚNG & còn hạn → executeAction(action)  (tắt máy / khởi động lại / dừng)
        └─ quá 120s → hết hạn, phải yêu cầu lại
```

Mã chỉ hiển thị trên Telegram của **chủ máy** → kẻ khác (kể cả AI agent chiếm quyền chat)
không thể tắt máy/dừng giám sát nếu không có mã.

### 5.2. BlockInput (ngắt bàn phím & chuột)

```
   /block [phút]
        │
        ▼
   auto = phút (hoặc config; nếu 0/>30 → ép = 30)   ← VAN AN TOÀN CỨNG
        │
        ▼
   StartBlock(auto):  giữ 1 OS thread, gọi BlockInput(TRUE)
        │              + luôn có defer BlockInput(FALSE)
        ├─ /unblock  → đóng kênh stop → BlockInput(FALSE)
        ├─ hết auto phút → tự BlockInput(FALSE) + báo Telegram
        └─ tiến trình thoát (kể cả /stop) → release trong defer / graceful exit
```

Van an toàn 30 phút đảm bảo **không bao giờ khóa cứng** máy kể cả khi mất mạng/mất Telegram.

---

## 6. Cài đặt & chạy

### 6.1. Cách A — Go native (khuyến nghị, không cần Python)
1. Tải `WinMonitor.exe` (trong `windows-native/` hoặc Releases).
2. Cấu hình (chọn 1):
   - `WinMonitor.exe --token 123:ABC --chat 987654321`
   - hoặc đặt `winmonitor.config.json` cạnh `.exe` (xem file `.example`).
3. Double-click `.exe` → tự cài khởi động ẩn + tự phục hồi.
4. Telegram → `/start`.

### 6.2. Cách B — Python
1. Cài Python 3 (tích "Add to PATH").
2. Tạo bot: **@BotFather** → `/newbot` → lấy token; lấy chat id qua **@userinfobot**.
3. `pythonw win_monitor.pyw --token ... --chat ...` (hoặc sửa `DEFAULT_BOT_TOKEN`/`DEFAULT_CHAT_ID`).
4. Telegram → `/start`.

### 6.3. Chạy quyền Administrator (khuyến nghị)
`/block` (ngắt bàn phím–chuột) cần quyền Admin để chắc chắn hoạt động. Chuột phải → *Run as administrator* một lần.

---

## 6.5. Vòng đời: cài, xóa file tải về, cập nhật, gỡ (TL;DR)

### Đúng — chỉ cần tải file và chạy, KHÔNG cần "setup" kiểu cài đặt
File `WinMonitor.exe` (Go native) là **standalone** — không cần cài Python, không cần
installer. Chỉ cần cấu hình token/chat (1 lần) rồi chạy.

### "Chạy 1 lần rồi xóa file .exe tải về" — ĐƯỢC ✅
Lần chạy đầu tiên, app tự **cài & bàn giao**:
1. Copy chính nó (ẩn) vào `%APPDATA%\WinMonitor\WinMonitor.exe`.
2. Đăng ký tự khởi động (Registry Run + 2 Scheduled Task + shortcut ẩn trong Startup)
   — tất cả trỏ tới **bản trong `%APPDATA%`**.
3. Chuyển sang chạy **bản trong `%APPDATA%`** rồi tiến trình tải về tự thoát.
→ File `.exe` ở Downloads **không còn bị khóa** → **xóa được ngay**. Xóa nó KHÔNG ảnh
hưởng gì: app vẫn chạy từ `%APPDATA%`, cấu hình vẫn còn trong
`%APPDATA%\WinMonitor\config.json`.

> Vì sao cần cơ chế này: Windows **khóa file .exe đang chạy**, không cho xóa. Nếu không
> bàn giao, bạn phải tắt app mới xóa được (mà app tự khởi động lại). Cơ chế trên giải
> quyết triệt để.

### Cập nhật lên version mới — làm gì?
1. (Tùy chọn) Gửi `/stop` + `/confirm` trên Telegram để tắt bản cũ & gỡ khỏi startup,
   hoặc chỉ cần **Task Manager → End task** `WinMonitor.exe`.
2. Tải `.exe` mới, chạy nó → nó tự **ghi đè** bản trong `%APPDATA%` và chạy.
   (Nếu bản cũ còn chạy, single-instance sẽ chặn; hãy End task bản cũ trước.)
3. Cấu hình (`config.json`) **giữ nguyên** qua các lần cập nhật.

### Đổi cấu hình — 3 cách (không cần cài lại)
- **Telegram** (tiện nhất): `/config` xem, `/set <khóa> <giá trị>`, `/interval`, `/screen on/off`… → **áp dụng ngay**.
- **File**: sửa `%APPDATA%\WinMonitor\config.json` rồi gửi `/reload_config`.
- **Bootstrap**: đặt `winmonitor.config.json` cạnh `.exe` trước lần chạy đầu (tự nạp rồi xóa).

### Gỡ chương trình hoàn toàn — làm gì?
Cách sạch nhất: gửi **`/stop`** trên Telegram → nhập **`/confirm <mã>`**. App sẽ:
- Gỡ Registry Run + 2 Scheduled Task + shortcut Startup;
- Xóa file trong `%APPDATA%\WinMonitor\` và tự xóa khi khởi động lại.
Sau đó xóa nốt thư mục `%APPDATA%\WinMonitor\` nếu muốn (chứa log/cấu hình còn lại).

> Gỡ thủ công (nếu không dùng được Telegram): xóa Task `WinMonitorGuard`/`WinMonitorLogon`
> trong Task Scheduler, xóa giá trị `WinMonitor` trong `HKCU\...\Run`, xóa shortcut
> `WinMonitor.lnk` trong Startup, xóa thư mục `%APPDATA%\WinMonitor\`.

---

## 7. Danh sách lệnh Telegram

> Menu lệnh (nút ☰) **tự cập nhật** mỗi lần khởi động; gõ `/reload` để cập nhật lại.

### Giám sát
| Lệnh | Tác dụng |
|---|---|
| `/status` | Trạng thái (uptime, chụp, monitors, quyền Admin, RDP, hàng đợi…) |
| `/shot` | Chụp màn hình ngay |
| `/interval <giây>` | Đổi chu kỳ chụp (5–86400) |
| `/screen on\|off` | Bật/tắt chụp định kỳ |
| `/activity on\|off` | Báo đổi cửa sổ |
| `/process on\|off` | Báo tiến trình mới |
| `/clip on\|off` | Theo dõi clipboard |
| `/keylog on\|off` | Ghi phím gõ (có cảnh báo quyền riêng tư) |

### Điều khiển
| Lệnh | Tác dụng |
|---|---|
| `/block [phút]` | Ngắt bàn phím & chuột (tự mở sau tối đa 30 phút) |
| `/unblock` | Kết nối lại bàn phím & chuột |
| `/lock` | Khóa màn hình |

### Nguy hiểm — cần OTP
| Lệnh | Tác dụng |
|---|---|
| `/shutdown` | Tắt máy |
| `/restart` | Khởi động lại |
| `/stop` | Dừng giám sát & gỡ khỏi khởi động cùng Windows |
| `/confirm <mã>` | Xác nhận OTP |
| `/cancel` | Hủy yêu cầu |

### Cấu hình
| Lệnh | Tác dụng |
|---|---|
| `/config` | Xem toàn bộ (token bị ẩn) |
| `/get <khóa>` | Xem 1 khóa |
| `/set <khóa> <giá trị>` | Đổi (tự validate/clamp) |
| `/reset` | Về mặc định (giữ token/chat_id) |
| `/reload_config` | Nạp lại từ file (không cần restart) |
| `/logs [n]` | Xem log gần đây |
| `/reload` | Cập nhật lại menu lệnh |

---

## 8. Cấu hình (và cách sửa)

### 8.1. Sửa config — 3 cách
| Cách | Làm thế nào | Hiệu lực |
|---|---|---|
| **Telegram** (tiện nhất) | `/set <khóa> <giá trị>`, `/interval`, `/screen on/off`… | **Ngay lập tức**, không cần restart |
| **File** `%APPDATA%\WinMonitor\config.json` | Sửa trực tiếp | Gõ `/reload_config` (hoặc restart) |
| **Bootstrap cạnh .exe** `winmonitor.config.json` | Đặt trước khi chạy lần đầu | Chỉ nạp lần đầu, sau đó xoá & chuyển vào `%APPDATA%` |

### 8.2. Các khóa
| Khóa | Mặc định | Ý nghĩa | Khoảng |
|---|---|---|---|
| `bot_token` | — | Token bot (sửa qua CLI/env/file) | — |
| `chat_id` | — | Chat ID chủ máy | — |
| `screenshot_interval` | `300` | Chu kỳ chụp (giây) | 5–86400 |
| `screenshots_enabled` | `true` | Bật chụp định kỳ | bool |
| `activity_monitor` | `true` | Báo đổi cửa sổ | bool |
| `process_monitor` | `true` | Báo tiến trình mới | bool |
| `clipboard_monitor` | `false` | Theo dõi clipboard | bool |
| `keylogger` | `false` | Ghi phím gõ | bool |
| `keylog_flush` | `60` | Giây/lần gửi phím gõ | 10–3600 |
| `block_safety_minutes` | `0` | Tự mở khóa sau N phút (0 = dùng van 30') | 0–1440 |
| `heartbeat_interval` | `3600` | Giây/lần báo "vẫn chạy" | 60–86400 hoặc 0 |
| `allow_autobind` | `true` | Cho tự liên kết chat đầu tiên | bool |

> Giá trị ngoài khoảng sẽ được **tự clamp** (vd `screenshot_interval = 1` → 5).

---

## 9. Cơ chế tự khởi động & tự phục hồi

Script đăng ký **nhiều lớp** để khó bị tắt:
1. **Registry `HKCU\...\Run`** → chạy khi đăng nhập.
2. **Scheduled Task `WinMonitorLogon`** → chạy khi đăng nhập.
3. **Scheduled Task `WinMonitorGuard`** → chạy **mỗi 5 phút**, tự bật lại nếu bị kill.
4. **Shortcut ẩn** trong thư mục Startup.
5. **`selfhealLoop`** → cứ 5 phút tự đăng ký lại các lớp trên.

Chỉ lệnh **`/stop`** (đã xác thực OTP) mới gỡ toàn bộ và thoát.

> Giới hạn trung thực: người có **quyền Administrator cục bộ** vẫn có thể gỡ thủ công
> (Task Scheduler, Registry). Không phần mềm nào chống được admin cục bộ tuyệt đối.

---

## 10. Bảo mật & mô hình đe dọa

| Lớp | Cơ chế |
|---|---|
| Chỉ chủ máy điều khiển | Kiểm tra `chat_id` trước mọi lệnh; có thể tắt auto-bind |
| Thao tác nguy hiểm | OTP 6 số (CSPRNG), hết hạn 120s, tối đa 5 lần thử |
| Token | Hỗ trợ `WINMON_TOKEN` (env), che `--token` khỏi argv; config đặt **ẩn + ACL** chỉ user hiện tại |
| Mã hoá source (tuỳ chọn) | `tool_mahoa/ma_hoa.py` — PBKDF2 + HMAC; từ chối mật khẩu yếu/mặc định |
| Kênh | HTTPS tới `api.telegram.org` |

**Mô hình đe dọa — điều KHÔNG chống được:**
- Admin cục bộ đọc bộ nhớ tiến trình lúc giải mã / gỡ Scheduled Task.
- Ai đọc được `config.json` / biến môi trường token.
- Đây là công cụ giám sát **máy của chính bạn** — xem mục 14.

---

## 11. Build & phát hành

### Go native (khuyến nghị)
```bash
cd windows-native && ./build.sh     # Linux/macOS cross-compile → amd64 + arm64 + SHA256SUMS
# hoặc build.bat trên Windows
```
CI `.github/workflows/build-go-exe.yml`: chạy `go test`, build amd64+arm64, checksum, ký số
(nếu có Secrets), upload artifact & gắn vào Release khi tạo tag `v*`.

### Python → .exe (PyInstaller)
```bash
pip install -r tool_build/requirements.txt
python tool_build/build_exe.py [--onedir] [--sign ...]
```
Xem `tool_build/README.md` để chống antivirus báo nhầm (onedir, ký số, FP report…).

### Mã hoá source Python
```bash
python tool_mahoa/ma_hoa.py win_monitor.pyw -p "MatKhauManh123"
```

> ⚠️ GitHub Actions cần **billing/Actions** hoạt động (repo private). Nếu CI báo lỗi
> "spending limit", xem issue #21 — có thể build tay hoặc chuyển repo sang Public.

---

## 12. Khắc phục sự cố

| Hiện tượng | Cách xử lý |
|---|---|
| Bot không trả lời | Kiểm token/chat id; xem `%APPDATA%\WinMonitor\monitor.log` hoặc `/logs` |
| Không chụp được màn hình | Thử `/shot`; kiểm tra phiên RDP/lock screen; xem log |
| `/block` báo không ngắt được | Chạy bằng **Administrator** |
| Không tự khởi động lại | Kiểm Task Scheduler (`WinMonitorGuard`); có thể bị GPO chặn |
| 409 Conflict | Token đang dùng ở máy/tiến trình khác — chỉ 1 nơi dùng 1 token |
| Antivirus báo nhầm | Dùng bản Go native; nếu vẫn bị → xem `tool_build/README.md` |
| Máy không có mạng | App vẫn chạy, tự gửi bù khi có mạng; bật `heartbeat` để biết còn sống |

---

## 13. Kiểm thử

```bash
# Go
cd windows-native && go test ./...

# Python
python -m unittest discover -s tests -v

# Kiểm thử thủ công trên Windows thật: xem docs/QA-checklist.md
```

---

## 14. Mục đích sử dụng & pháp lý

Công cụ này dành cho việc **giám sát máy tính của CHÍNH BẠN** (hoặc máy bạn có quyền quản
trị hợp pháp và được sự đồng ý của người dùng). **Không** dùng để theo dõi người khác trái
phép. Tính năng keylogger có thể ghi cả mật khẩu — chỉ bật khi thật cần. Xem `LICENSE`.
