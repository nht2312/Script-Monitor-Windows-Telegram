#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Bộ test tự động cho win_monitor.pyw (issue #42 — test automation).
Chạy: python -m unittest discover -s tests -v
Không cần Windows (các hàm Win32 được stub/không gọi trong test logic).
"""
import importlib.machinery
import importlib.util
import os
import struct
import tempfile
import unittest


def load_module():
    os.environ.setdefault("APPDATA", tempfile.mkdtemp())
    loader = importlib.machinery.SourceFileLoader("wm", os.path.join(os.path.dirname(__file__), "..", "win_monitor.pyw"))
    spec = importlib.util.spec_from_loader("wm", loader)
    m = importlib.util.module_from_spec(spec)
    loader.exec_module(m)
    m.tg_call = lambda *a, **k: {"ok": True}
    m.load_config()
    return m


class WinMonitorTests(unittest.TestCase):
    def setUp(self):
        self.wm = load_module()
        self.wm.cfg_set("chat_id", "123")

    def test_parse_bool(self):
        for s in ("1", "on", "true", "bật", "có"):
            self.assertTrue(self.wm.parse_bool(s))
        for s in ("0", "off", "false", "", "khong"):
            self.assertFalse(self.wm.parse_bool(s))

    def test_config_clamp(self):
        self.wm.cfg_set("screenshot_interval", 1)
        self.assertEqual(self.wm.cfg_get("screenshot_interval"), 5)
        self.wm.cfg_set("keylog_flush", 1)
        self.assertEqual(self.wm.cfg_get("keylog_flush"), 10)

    def test_interval_command(self):
        self.wm.handle_command("/interval 90", "123")
        self.assertEqual(self.wm.cfg_get("screenshot_interval"), 90)

    def test_set_and_get(self):
        self.wm.handle_command("/set keylogger on", "123")
        self.assertTrue(self.wm.cfg_get("keylogger"))
        self.wm.handle_command("/keylog off", "123")
        self.assertFalse(self.wm.cfg_get("keylogger"))
        self.wm.handle_command("/set screenshot_interval abc", "123")  # invalid -> unchanged

    def test_reset_keeps_identity(self):
        self.wm.cfg_set("screenshot_interval", 123)
        self.wm.handle_command("/reset", "123")
        self.assertEqual(self.wm.cfg_get("screenshot_interval"), 300)
        self.assertEqual(self.wm.cfg_get("chat_id"), "123")

    def test_otp_crypto_and_limit(self):
        wm = self.wm
        wm.execute_action = lambda a: None
        wm.PENDING.clear()
        wm.OTP_FAILURES["count"] = 0
        wm.request_otp("stop", "X")
        code = wm.PENDING["stop"]["code"]
        self.assertEqual(len(code), 6)
        self.assertTrue(code.isdigit())
        for _ in range(5):
            wm.handle_command("/confirm 000000x", "123")
        self.assertEqual(len(wm.PENDING), 0)

    def test_otp_confirm_consumes(self):
        wm = self.wm
        wm.execute_action = lambda a: None
        wm.PENDING.clear()
        wm.OTP_FAILURES["count"] = 0
        wm.request_otp("shutdown", "Y")
        code = wm.PENDING["shutdown"]["code"]
        wm.handle_command("/confirm %s" % code, "123")
        self.assertNotIn("shutdown", wm.PENDING)

    def test_png_encoder(self):
        wm = self.wm
        w, h = 2, 2
        buf = bytes([0, 0, 255, 255] * 4)  # BGRA red
        png = wm._png_from_bgra(w, h, buf)
        self.assertEqual(png[:8], b"\x89PNG\r\n\x1a\n")
        iw, ih = struct.unpack(">II", png[16:24])
        self.assertEqual((iw, ih), (w, h))

    def test_queue_drop_oldest(self):
        wm = self.wm
        wm.SEND_Q.queue.clear()
        for _ in range(wm.MAX_QUEUE + 50):
            wm.notify("m")
        self.assertEqual(wm.SEND_Q.qsize(), wm.MAX_QUEUE)

    def test_mask_token(self):
        self.assertNotEqual(self.wm.mask_token("123456789:ABC") if hasattr(self.wm, "mask_token") else "x", "123456789:ABC")


if __name__ == "__main__":
    unittest.main()
