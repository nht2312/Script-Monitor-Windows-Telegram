#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# ===========================================================================
#  FILE DA MA HOA (ma hoa boi tool_mahoa/ma_hoa.py). KHONG chinh sua tay.
#  Khi chay se hoi MAT KHAU roi giai ma trong bo nho va thuc thi.
# ===========================================================================
import base64, hashlib, hmac, marshal, zlib, sys, os

_HEADER = "WINMON-ENC-v1 s /ljqmVlx5ZDUGCcD9IPp/g== DYao/rJVSjHgtz08y6Uzbw== U2/wWoT2PZIObTQSZngmXw=="
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
