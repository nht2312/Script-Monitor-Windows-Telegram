#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
ma_hoa.py — Công cụ mã hóa file script Python thành file .pyw tự chạy.

- File đầu ra (vd: win_monitor_mahoa.pyw) là file ĐÃ MÃ HÓA: mở bằng Notepad chỉ
  thấy dữ liệu mã hóa, KHÔNG thấy code gốc / token.
- Khi chạy, file sẽ hỏi MẬT KHẨU (hoặc đọc biến môi trường WINMON_KEY), giải mã
  trong bộ nhớ rồi thực thi y như file gốc.
- Dùng thuần thư viện chuẩn (không cần cài thêm gì).

Cách dùng:
    python ma_hoa.py win_monitor.pyw
        -> tạo win_monitor_mahoa.pyw (mặc định: chế độ source, chạy được mọi Python 3)

    python ma_hoa.py win_monitor.pyw --compile
        -> nhúng code đã biên dịch (khó dịch ngược hơn, nhưng phải CÙNG phiên bản Python)

    python ma_hoa.py win_monitor.pyw -o ten_khac.pyw -p "matkhau"
    python ma_hoa.py --selftest
"""

import os
import sys
import base64
import hashlib
import hmac
import zlib
import marshal
import argparse
import getpass

MAGIC = "WINMON-ENC-v1"
DEFAULT_ITER = 200000


# ---------------------------------------------------------------------------
#  Lõi mã hóa (dùng chung cho cả lúc mã hóa lẫn lúc giải mã)
# ---------------------------------------------------------------------------
def derive_key(password, salt, iterations):
    return hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt, iterations, 32)


def keystream(key, nonce, length):
    """Sinh dòng khóa giả ngẫu nhiên từ HMAC-SHA256 (counter mode)."""
    out = bytearray()
    counter = 0
    while len(out) < length:
        block = hmac.new(key, nonce + counter.to_bytes(8, "big"), hashlib.sha256).digest()
        out += block
        counter += 1
    return bytes(out[:length])


def xor_bytes(data, stream):
    return bytes(a ^ b for a, b in zip(data, stream))


def encrypt_bytes(plaintext, password, mode, iterations=DEFAULT_ITER):
    salt = os.urandom(16)
    nonce = os.urandom(16)
    key = derive_key(password, salt, iterations)
    ks = keystream(key, nonce, len(plaintext))
    cipher = xor_bytes(plaintext, ks)
    tag = hmac.new(key, nonce + cipher, hashlib.sha256).digest()[:16]
    return salt, nonce, tag, cipher


def decrypt_bytes(cipher, password, salt, nonce, tag, iterations=DEFAULT_ITER):
    key = derive_key(password, salt, iterations)
    calc = hmac.new(key, nonce + cipher, hashlib.sha256).digest()[:16]
    if not hmac.compare_digest(calc, tag):
        raise ValueError("Sai mật khẩu hoặc file đã bị chỉnh sửa.")
    ks = keystream(key, nonce, len(cipher))
    return xor_bytes(cipher, ks)


# ---------------------------------------------------------------------------
#  Mẫu loader được nhúng vào file đầu ra
# ---------------------------------------------------------------------------
LOADER_TEMPLATE = '''#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# ===========================================================================
#  FILE DA MA HOA (ma hoa boi tool_mahoa/ma_hoa.py). KHONG chinh sua tay.
#  Khi chay se hoi MAT KHAU roi giai ma trong bo nho va thuc thi.
# ===========================================================================
import base64, hashlib, hmac, marshal, zlib, sys, os

_HEADER = "{header}"
_BODY = (
{body}
)
_ITER = {iterations}
_MODE = "{mode}"


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
                         "hoac tao file .env (WINMON_KEY=...) canh script.\\n")
        sys.exit(1)
    import getpass
    try:
        return getpass.getpass("Mat khau: ")
    except Exception:
        return input("Mat khau: ")


def _run():
    parts = _HEADER.split()
    if len(parts) < 5 or parts[0] != "WINMON-ENC-v1":
        sys.stderr.write("File khong hop le.\\n")
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
        sys.stderr.write("Sai mat khau hoac file da bi chinh sua.\\n")
        sys.exit(1)
    ks = _keystream(key, nonce, len(cipher))
    plain = bytes(a ^ b for a, b in zip(cipher, ks))
    plain = zlib.decompress(plain)
    if mode == "c":
        code = marshal.loads(plain)
        g = {{"__name__": "__main__", "__file__": os.path.abspath(__file__)}}
        exec(code, g)
    else:
        src = plain.decode("utf-8")
        g = {{"__name__": "__main__", "__file__": os.path.abspath(__file__)}}
        exec(compile(src, os.path.abspath(__file__), "exec"), g)


if __name__ == "__main__":
    _run()
'''


def _wrap_body(b64, width=96):
    chunks = [b64[i:i + width] for i in range(0, len(b64), width)]
    return "\n".join('    "%s"' % c for c in chunks)


def build_encrypted_script(source_bytes, password, mode="s", iterations=DEFAULT_ITER):
    """
    mode = "s" (source, chạy mọi Python 3) hoặc "c" (compiled, cùng phiên bản Python).
    Trả về nội dung file .pyw đã mã hóa (str).
    """
    if mode == "c":
        # biên dịch + marshal (phải cùng phiên bản Python khi chạy)
        code = compile(source_bytes, "<encrypted>", "exec")
        payload = marshal.dumps(code)
    else:
        payload = source_bytes

    payload = zlib.compress(payload, 9)
    salt, nonce, tag, cipher = encrypt_bytes(payload, password, mode, iterations)

    header = "%s %s %s %s %s" % (
        MAGIC, mode,
        base64.b64encode(salt).decode(),
        base64.b64encode(nonce).decode(),
        base64.b64encode(tag).decode(),
    )
    body = _wrap_body(base64.b64encode(cipher).decode())
    return LOADER_TEMPLATE.format(header=header, body=body, iterations=iterations, mode=mode)


# ---------------------------------------------------------------------------
#  CLI
# ---------------------------------------------------------------------------
def default_output(inp):
    root, ext = os.path.splitext(inp)
    return root + "_mahoa" + (ext or ".pyw")


def selftest():
    sample = b"print('xin chao tu file da ma hoa')\nX = 1 + 2\n"
    for mode in ("s", "c"):
        out = build_encrypted_script(sample, "matkhau-test", mode=mode, iterations=1000)
        # trích ngược header/body rồi giải mã để kiểm tra round-trip
        header = out.split('_HEADER = "', 1)[1].split('"', 1)[0]
        parts = header.split()
        salt = base64.b64decode(parts[2]); nonce = base64.b64decode(parts[3]); tag = base64.b64decode(parts[4])
        body_block = out.split("_BODY = (", 1)[1].split(")", 1)[0]
        b64 = "".join(seg.strip().strip('"') for seg in body_block.strip().splitlines())
        cipher = base64.b64decode(b64)
        plain = zlib.decompress(decrypt_bytes(cipher, "matkhau-test", salt, nonce, tag, iterations=1000))
        if mode == "s":
            assert plain == sample, "source round-trip loi"
        else:
            assert marshal.loads(plain) is not None
        # sai mat khau phai that bai
        try:
            decrypt_bytes(cipher, "sai", salt, nonce, tag, iterations=1000)
            raise AssertionError("sai mat khau van giai duoc?!")
        except ValueError:
            pass
        print("  [OK] mode=%s round-trip + kiem tra mat khau" % mode)
    print("SELFTEST PASSED")


def main():
    ap = argparse.ArgumentParser(description="Ma hoa file script Python thanh .pyw tu chay.")
    ap.add_argument("input", nargs="?", help="File .py/.pyw goc can ma hoa")
    ap.add_argument("-o", "--output", help="File dau ra (mac dinh: <ten>_mahoa.pyw)")
    ap.add_argument("-p", "--password", help="Mat khau (mac dinh: hoi khi chay)")
    ap.add_argument("--compile", action="store_true",
                    help="Che do compiled (kho dich nguoc, phai cung phien ban Python)")
    ap.add_argument("--iter", type=int, default=DEFAULT_ITER, help="So vong PBKDF2")
    ap.add_argument("--selftest", action="store_true", help="Chay kiem tra noi bo")
    ap.add_argument("--force", action="store_true", help="Bo qua canh bao mat khau yeu")
    args = ap.parse_args()

    if args.selftest:
        selftest()
        return

    if not args.input:
        ap.error("Thieu file dau vao (hoac dung --selftest)")

    with open(args.input, "rb") as f:
        source_bytes = f.read()

    password = args.password or os.environ.get("WINMON_KEY")
    if not password:
        pw1 = getpass.getpass("Nhap mat khau: ")
        pw2 = getpass.getpass("Nhap lai mat khau: ")
        if pw1 != pw2:
            sys.stderr.write("Mat khau khong khop.\n")
            sys.exit(1)
        if not pw1:
            sys.stderr.write("Mat khau rong khong duoc phep.\n")
            sys.exit(1)
        password = pw1

    # Canh bao mat khau yeu / mac dinh cong khai (issue #1)
    weak = {"doi-mat-khau-nay", "doi_mat_khau_nay_123", "matkhau", "password", "123456", "admin"}
    if len(password) < 8 or password.lower() in weak:
        sys.stderr.write("CANH BAO: mat khau yeu hoac la mat khau MAC DINH cong khai.\n"
                         "  -> Ban nen dung mat khau >= 12 ky tu, KHONG trung vi du/README.\n")
        if not args.force:
            sys.stderr.write("  (Dung --force de bo qua canh bao nay.)\n")
            sys.exit(2)

    mode = "c" if args.compile else "s"
    out_text = build_encrypted_script(source_bytes, password, mode=mode, iterations=args.iter)

    output = args.output or default_output(args.input)
    with open(output, "w", encoding="utf-8") as f:
        f.write(out_text)

    # che do compiled: canh bao phien ban
    if mode == "c":
        sys.stderr.write("LUU Y: che do --compile chi chay dung tren Python %s.%s (cung phien ban khi ma hoa).\n"
                         % (sys.version_info.major, sys.version_info.minor))

    print("Da tao file ma hoa: %s (%d bytes)" % (output, len(out_text)))


if __name__ == "__main__":
    main()
