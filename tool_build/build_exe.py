#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
build_exe.py — Đóng gói win_monitor.pyw thành WinMonitor.exe độc lập.

File .exe chạy được trên máy CHƯA CÀI PYTHON. Kèm cấu hình để GIẢM BÁO NHẦM
ANTIVIRUS (manifest, version resource, tắt UPX, tùy chọn onedir + ký số).

Cách dùng:
    pip install -r tool_build/requirements.txt
    python tool_build/build_exe.py                # onefile (mặc định)
    python tool_build/build_exe.py --onedir       # KHUYẾN NGHỊ nếu bị AV báo nhầm
    python tool_build/build_exe.py --admin        # yêu cầu quyền Admin (cho /block)
    python tool_build/build_exe.py --sign "C:\\duong\\signtool.exe" --pfx cert.pfx
"""

import os
import sys
import shutil
import subprocess
import argparse

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
SCRIPT = os.path.join(ROOT, "win_monitor.pyw")
MANIFEST = os.path.join(HERE, "manifest.xml")
VERSION_FILE = os.path.join(HERE, "version_info.txt")
ICON = os.path.join(HERE, "icon.ico")


def do_sign(exe_path, signtool, pfx=None, pfx_pass=None, thumbprint=None):
    """Ký số file .exe (nếu có chứng chỉ). Đây là cách HIỆU QUẢ NHẤT chống báo nhầm AV."""
    if not signtool or not os.path.exists(signtool):
        print("!! Khong tim thay signtool, bo qua ky so.")
        return False
    if pfx:
        cmd = [signtool, "sign", "/fd", "SHA256", "/f", pfx, "/tr",
               "http://timestamp.digicert.com", "/td", "SHA256"]
        if pfx_pass:
            cmd += ["/p", pfx_pass]
        cmd += [exe_path]
    elif thumbprint:
        cmd = [signtool, "sign", "/fd", "SHA256", "/sha1", thumbprint, "/tr",
               "http://timestamp.digicert.com", "/td", "SHA256", exe_path]
    else:
        print("!! Thieu --pfx hoac --thumbprint, bo qua ky so.")
        return False
    print("Ky so:", " ".join(cmd))
    return subprocess.call(cmd) == 0


def main():
    ap = argparse.ArgumentParser(description="Build WinMonitor.exe (standalone, it bi AV bao nham).")
    ap.add_argument("--onedir", action="store_true",
                    help="Xuat ra THU MUC thay vi 1 file (GIAM MANH bao nham antivirus)")
    ap.add_argument("--onefile", action="store_true", help="(mac dinh) 1 file duy nhat")
    ap.add_argument("--name", default="WinMonitor")
    ap.add_argument("--admin", action="store_true", help="Yeu cau quyen Admin (cho /block)")
    ap.add_argument("--sign", metavar="SIGNTOOL", help="Duong dan signtool.exe de ky so")
    ap.add_argument("--pfx", help="File chung chi .pfx")
    ap.add_argument("--pfx-pass", help="Mat khau .pfx")
    ap.add_argument("--thumbprint", help="Thumbprint chung chi trong store")
    args = ap.parse_args()

    if not os.path.exists(SCRIPT):
        sys.exit("Khong tim thay: %s" % SCRIPT)
    try:
        import PyInstaller  # noqa
    except Exception:
        sys.exit("Chua cai PyInstaller. Chay: pip install -r tool_build/requirements.txt")

    onedir = args.onedir and not args.onefile

    # PyInstaller de xu ly .py hon .pyw
    tmp_py = os.path.join(HERE, "_build_entry.py")
    shutil.copyfile(SCRIPT, tmp_py)

    cmd = [
        sys.executable, "-m", "PyInstaller",
        "--noconfirm", "--clean",
        "--name", args.name,
        "--distpath", os.path.join(ROOT, "dist"),
        "--workpath", os.path.join(HERE, "build"),
        "--specpath", HERE,
        "--onedir" if onedir else "--onefile",
        "--windowed",                 # khong hien console
        "--noupx",                    # TAT UPX: UPX la nguyen nhan pho bien bi AV bao nham
        "--hidden-import", "ctypes",
        "--hidden-import", "winreg",
    ]
    if os.path.exists(MANIFEST):
        cmd += ["--manifest", MANIFEST]
    if os.path.exists(VERSION_FILE):
        cmd += ["--version-file", VERSION_FILE]
    if os.path.exists(ICON):
        cmd += ["--icon", ICON]
    if args.admin:
        cmd += ["--uac-admin"]
    cmd.append(tmp_py)

    print("Dang build:", " ".join(cmd))
    rc = subprocess.call(cmd)
    try:
        os.remove(tmp_py)
    except Exception:
        pass
    if rc != 0:
        sys.exit("Build that bai (ma %d)" % rc)

    exe = os.path.join(ROOT, "dist", args.name + (".exe" if os.name == "nt" else ""))

    # Ky so neu duoc yeu cau
    if args.sign:
        do_sign(exe, args.sign, args.pfx, args.pfx_pass, args.thumbprint)

    print("\n=== XONG ===")
    print("File .exe:", exe)
    print("May CHUA CAI PYTHON chi can chay file nay.")
    if not onedir:
        print("Neu bi antivirus bao nham: build lai voi --onedir va/hoac ky so (--sign).")


if __name__ == "__main__":
    main()
