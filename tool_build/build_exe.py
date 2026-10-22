#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
build_exe.py — Đóng gói win_monitor.pyw thành 1 file WinMonitor.exe độc lập.

File .exe này CHẠY ĐƯỢC TRÊN MÁY CHƯA CÀI PYTHON (Python + mọi thứ được nhúng sẵn).

Cách dùng (chạy trên máy Windows hoặc qua GitHub Actions):
    pip install -r tool_build/requirements.txt
    python tool_build/build_exe.py

Kết quả: dist/WinMonitor.exe

Tùy chọn:
    --onedir      Tạo thư mục thay vì 1 file (khởi động nhanh hơn)
    --no-console  Ẩn hoàn toàn console (mặc định đã ẩn bằng windowed mode)
    --name TÊN    Đặt tên file .exe khác
"""

import os
import sys
import shutil
import subprocess
import argparse

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
SCRIPT = os.path.join(ROOT, "win_monitor.pyw")


def main():
    ap = argparse.ArgumentParser(description="Build WinMonitor.exe (standalone).")
    ap.add_argument("--onedir", action="store_true", help="Dung --onedir thay vi --onefile")
    ap.add_argument("--name", default="WinMonitor", help="Ten file .exe")
    args = ap.parse_args()

    if not os.path.exists(SCRIPT):
        sys.exit("Khong tim thay: %s" % SCRIPT)

    try:
        import PyInstaller  # noqa
    except Exception:
        sys.exit("Chua cai PyInstaller. Chay: pip install -r tool_build/requirements.txt")

    # Copy script sang .py tam (PyInstaller xu ly .py de hon .pyw)
    tmp_py = os.path.join(HERE, "_build_entry.py")
    shutil.copyfile(SCRIPT, tmp_py)

    icon = os.path.join(HERE, "icon.ico")
    cmd = [
        sys.executable, "-m", "PyInstaller",
        "--noconfirm", "--clean",
        "--name", args.name,
        "--distpath", os.path.join(ROOT, "dist"),
        "--workpath", os.path.join(HERE, "build"),
        "--specpath", HERE,
        "--onefile" if not args.onedir else "--onedir",
        "--windowed",                 # khong hien console (giong pythonw)
        "--hidden-import", "ctypes",
        "--hidden-import", "winreg",
        "--hidden-import", "tkinter",
    ]
    if os.path.exists(icon):
        cmd += ["--icon", icon]
    cmd.append(tmp_py)

    print("Dang build:", " ".join(cmd))
    rc = subprocess.call(cmd)
    try:
        os.remove(tmp_py)
    except Exception:
        pass

    if rc != 0:
        sys.exit("Build that bai (ma %d)" % rc)

    out = os.path.join(ROOT, "dist", args.name + (".exe" if os.name == "nt" else ""))
    print("\n=== XONG ===")
    print("File .exe:", out)
    print("Tren may CHUA CAI PYTHON, chi can chay file .exe nay la duoc.")


if __name__ == "__main__":
    main()
