#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# ===========================================================================
#  FILE DA MA HOA (ma hoa boi tool_mahoa/ma_hoa.py). KHONG chinh sua tay.
#  Khi chay se hoi MAT KHAU roi giai ma trong bo nho va thuc thi.
# ===========================================================================
import base64, hashlib, hmac, marshal, zlib, sys, os

_HEADER = "WINMON-ENC-v1 s OBd5g7Zyz/ofbnhOUHKBoQ== hPmKTkBUCa/a/3eIKubzvg== 0ib0VbUDr8zYfp6G/ABzQA=="
_BODY = (
    "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"
    "3hQoto9OTMSJ74PHoAF4AvLdDko="
)
_ITER = 200000
_MODE = "s"


def _keystream(key, nonce, length):
    out = bytearray()
    counter = 0
    while len(out) < length:
        out += hmac.new(key, nonce + counter.to_bytes(8, "big"), hashlib.sha256).digest()
        counter += 1
    return bytes(out[:length])


def _read_env_password():
    """Doc mat khau tu file .env (WINMON_KEY=...) o cac vi tri uu tien."""
    here = os.path.dirname(os.path.abspath(__file__))
    cands = [
        os.path.join(here, ".env"),
        os.path.join(os.getcwd(), ".env"),
    ]
    appdata = os.environ.get("APPDATA")
    if appdata:
        cands.append(os.path.join(appdata, "WinMonitor", ".env"))
    cands.append(os.path.join(os.path.expanduser("~"), ".win_monitor.env"))
    for path in cands:
        try:
            if os.path.exists(path):
                with open(path, "r", encoding="utf-8", errors="ignore") as f:
                    for line in f:
                        line = line.strip()
                        if not line or line.startswith("#") or "=" not in line:
                            continue
                        k, v = line.split("=", 1)
                        if k.strip() == "WINMON_KEY":
                            return v.strip().strip('"').strip("'")
        except Exception:
            pass
    return None


def _get_password():
    p = os.environ.get("WINMON_KEY")
    if p:
        return p
    p = _read_env_password()
    if p:
        return p
    # Neu chay nen (khong co console) thi khong the hoi mat khau -> bao loi ro rang
    if not sys.stdin or not sys.stdin.isatty():
        sys.stderr.write("Khong tim thay mat khau. Dat bien moi truong WINMON_KEY "
                         "hoac tao file .env (WINMON_KEY=...) canh script.\n")
        sys.exit(1)
    import getpass
    try:
        return getpass.getpass("Mat khau: ")
    except Exception:
        return input("Mat khau: ")


def _run():
    parts = _HEADER.split()
    if len(parts) < 5 or parts[0] != "WINMON-ENC-v1":
        sys.stderr.write("File khong hop le.\n")
        sys.exit(1)
    mode = parts[1]
    salt = base64.b64decode(parts[2])
    nonce = base64.b64decode(parts[3])
    tag = base64.b64decode(parts[4])
    cipher = base64.b64decode(_BODY)
    pw = _get_password()
    key = hashlib.pbkdf2_hmac("sha256", pw.encode("utf-8"), salt, _ITER, 32)
    calc = hmac.new(key, nonce + cipher, hashlib.sha256).digest()[:16]
    if not hmac.compare_digest(calc, tag):
        sys.stderr.write("Sai mat khau hoac file da bi chinh sua.\n")
        sys.exit(1)
    ks = _keystream(key, nonce, len(cipher))
    plain = bytes(a ^ b for a, b in zip(cipher, ks))
    plain = zlib.decompress(plain)
    if mode == "c":
        code = marshal.loads(plain)
        g = {"__name__": "__main__", "__file__": os.path.abspath(__file__)}
        exec(code, g)
    else:
        src = plain.decode("utf-8")
        g = {"__name__": "__main__", "__file__": os.path.abspath(__file__)}
        exec(compile(src, os.path.abspath(__file__), "exec"), g)


if __name__ == "__main__":
    _run()
