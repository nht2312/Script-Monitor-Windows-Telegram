#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# ===========================================================================
#  FILE DA MA HOA (ma hoa boi tool_mahoa/ma_hoa.py). KHONG chinh sua tay.
#  Khi chay se hoi MAT KHAU roi giai ma trong bo nho va thuc thi.
# ===========================================================================
import base64, hashlib, hmac, marshal, zlib, sys, os

_HEADER = "WINMON-ENC-v1 s ohlsvhFbm/9yrNScH2h4hg== toufH1zffV5wzA9SZBtC6w== sUsfRIgC45nV/hpp0oL+gQ=="
_BODY = (
    "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"
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
