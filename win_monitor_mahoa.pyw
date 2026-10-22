#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# ===========================================================================
#  FILE DA MA HOA (ma hoa boi tool_mahoa/ma_hoa.py). KHONG chinh sua tay.
#  Khi chay se hoi MAT KHAU roi giai ma trong bo nho va thuc thi.
# ===========================================================================
import base64, hashlib, hmac, marshal, zlib, sys, os

_HEADER = "WINMON-ENC-v1 s Cb0oVU+CMjKvGDtdXH2/pQ== AC7Mvebi/fGDr15NGoY1Sw== RdD+aC8Ljat10esXLQ3jMw=="
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
