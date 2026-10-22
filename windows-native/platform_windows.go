//go:build windows

package main

import (
	"encoding/csv"
	"image"
	"image/png"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetForegroundWindow       = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW            = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW      = user32.NewProc("GetWindowTextLengthW")
	procBlockInput                = user32.NewProc("BlockInput")
	procGetAsyncKeyState          = user32.NewProc("GetAsyncKeyState")
	procLockWorkStation           = user32.NewProc("LockWorkStation")
	procGetDC                     = user32.NewProc("GetDC")
	procReleaseDC                 = user32.NewProc("ReleaseDC")
	procGetSystemMetrics          = user32.NewProc("GetSystemMetrics")
	procShowWindow                = user32.NewProc("ShowWindow")
	procGetConsoleWindow          = kernel32.NewProc("GetConsoleWindow")
	procCreateCompatibleDC        = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap    = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject              = gdi32.NewProc("SelectObject")
	procBitBlt                    = gdi32.NewProc("BitBlt")
	procDeleteObject              = gdi32.NewProc("DeleteObject")
	procDeleteDC                  = gdi32.NewProc("DeleteDC")
	procGetDIBits                 = gdi32.NewProc("GetDIBits")
	procSetFileAttributesW        = kernel32.NewProc("SetFileAttributesW")
	procCreateMutexW              = kernel32.NewProc("CreateMutexW")
	procMoveFileExW               = kernel32.NewProc("MoveFileExW")
	procSetProcessDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware        = user32.NewProc("SetProcessDPIAware")
	procSetWindowsHookEx          = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx            = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx       = user32.NewProc("UnhookWindowsHookEx")
	procGetMessage                = user32.NewProc("GetMessageW")
	procTranslateMessage          = user32.NewProc("TranslateMessage")
	procDispatchMessage           = user32.NewProc("DispatchMessageW")
)

// AutoUnblockNotify được main gán để báo khi bàn phím/chuột tự mở khóa.
var AutoUnblockNotify func()

func hideConsole() {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd != 0 {
		procShowWindow.Call(hwnd, 0) // SW_HIDE
	}
}

// setDpiAwareness giúp chụp màn hình đúng trên cấu hình DPI hỗn hợp (issue #33).
func setDpiAwareness() {
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
	r, _, _ := procSetProcessDpiAwarenessCtx.Call(uintptr(^uintptr(3))) // -4 as unsigned
	if r == 0 {
		procSetProcessDPIAware.Call() // fallback
	}
}

func getSysMetric(idx int) int {
	r, _, _ := procGetSystemMetrics.Call(uintptr(idx))
	return int(int32(r))
}

func isRemoteSession() bool {
	const smRemoteSession = 0x1000
	return getSysMetric(smRemoteSession) != 0
}

// isElevated kiểm tra tiến trình có quyền Administrator không (issue #36).
func isElevated() bool {
	return runHidden("net", "session") == nil
}

func foregroundTitle() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(n+1))
	return syscall.UTF16ToString(buf)
}

func takeScreenshot(path string) bool {
	x := getSysMetric(76)
	y := getSysMetric(77)
	w := getSysMetric(78)
	h := getSysMetric(79)
	if w <= 0 || h <= 0 {
		x, y = 0, 0
		w = getSysMetric(0)
		h = getSysMetric(1)
	}
	if w <= 0 || h <= 0 {
		return false
	}
	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return false
	}
	defer procReleaseDC.Call(0, hdcScreen)
	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return false
	}
	defer procDeleteDC.Call(hdcMem)
	hbm, _, _ := procCreateCompatibleBitmap.Call(hdcScreen, uintptr(w), uintptr(h))
	if hbm == 0 {
		return false
	}
	defer procDeleteObject.Call(hbm)
	procSelectObject.Call(hdcMem, hbm)
	const srcCopy = 0x00CC0020
	ok, _, _ := procBitBlt.Call(hdcMem, 0, 0, uintptr(w), uintptr(h), hdcScreen,
		uintptr(x), uintptr(y), srcCopy)
	if ok == 0 {
		return false
	}
	bmi := make([]byte, 40)
	*(*uint32)(unsafe.Pointer(&bmi[0])) = 40
	*(*int32)(unsafe.Pointer(&bmi[4])) = int32(w)
	*(*int32)(unsafe.Pointer(&bmi[8])) = -int32(h)
	*(*uint16)(unsafe.Pointer(&bmi[12])) = 1
	*(*uint16)(unsafe.Pointer(&bmi[14])) = 32
	bufSize := w * h * 4
	buf := make([]byte, bufSize)
	procGetDIBits.Call(hdcMem, hbm, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bmi[0])), 0)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < bufSize; i += 4 {
		img.Pix[i] = buf[i+2]
		img.Pix[i+1] = buf[i+1]
		img.Pix[i+2] = buf[i]
		img.Pix[i+3] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return png.Encode(f, img) == nil
}

func lockWorkstation() bool {
	r, _, _ := procLockWorkStation.Call()
	return r != 0
}

func keyDown(vk int) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return uint16(r)&0x8000 != 0
}

func hideFile(path string) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	const fileAttrHidden = 0x2
	const fileAttrSystem = 0x4
	procSetFileAttributesW.Call(uintptr(unsafe.Pointer(p)), fileAttrHidden|fileAttrSystem)
}

// restrictToCurrentUser đặt ACL chỉ user hiện tại đọc/ghi (issue #4).
func restrictToCurrentUser(path string) {
	user := os.Getenv("USERNAME")
	if user == "" {
		return
	}
	runHidden("icacls", path, "/inheritance:r", "/grant:r", user+":F")
}

func moveFileOnReboot(path string) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	const movefileDelayUntilReboot = 0x4
	procMoveFileExW.Call(uintptr(unsafe.Pointer(p)), 0, movefileDelayUntilReboot)
}

var mutexHandle uintptr

func acquireSingleInstance() bool {
	name, _ := syscall.UTF16PtrFromString("Global\\WinMonitorSingleton_v1")
	h, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	mutexHandle = h
	if errno, ok := err.(syscall.Errno); ok && errno == 183 {
		return false
	}
	return true
}

// ---- BlockInput (giữ trên 1 OS thread + luôn giải phóng) ----
var (
	blockMu     sync.Mutex
	blockActive bool
	blockStop   chan struct{}
)

func isInputBlocked() bool {
	blockMu.Lock()
	defer blockMu.Unlock()
	return blockActive
}

// StartBlock ngắt bàn phím+chuột. Trả về (started, ok).
// autoMinutes: tự mở sau N phút (đã được main áp hard-cap).
func StartBlock(autoMinutes int) (bool, bool) {
	blockMu.Lock()
	if blockActive {
		blockMu.Unlock()
		return false, false
	}
	blockActive = true
	stop := make(chan struct{})
	blockStop = stop
	blockMu.Unlock()

	res := make(chan bool, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		r, _, _ := procBlockInput.Call(1)
		if r == 0 {
			blockMu.Lock()
			blockActive = false
			blockMu.Unlock()
			res <- false
			return
		}
		res <- true
		// Luôn giải phóng dù thoát kiểu gì (issue #14/#29)
		defer func() {
			procBlockInput.Call(0)
			blockMu.Lock()
			blockActive = false
			blockMu.Unlock()
		}()
		if autoMinutes > 0 {
			select {
			case <-stop:
			case <-time.After(time.Duration(autoMinutes) * time.Minute):
				if AutoUnblockNotify != nil {
					AutoUnblockNotify()
				}
			}
		} else {
			<-stop
		}
	}()
	ok := <-res
	return true, ok
}

func StopBlock() bool {
	blockMu.Lock()
	defer blockMu.Unlock()
	if blockActive && blockStop != nil {
		close(blockStop)
		blockStop = nil
		return true
	}
	return false
}

// ---- process / clipboard / exec ----
type Proc struct {
	PID  string
	Name string
}

func listProcesses() []Proc {
	out, err := runHiddenOutput("tasklist", "/fo", "csv", "/nh")
	if err != nil {
		return nil
	}
	var procs []Proc
	r := csv.NewReader(strings.NewReader(out))
	r.FieldsPerRecord = -1
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) >= 2 {
			procs = append(procs, Proc{PID: strings.TrimSpace(rec[1]), Name: strings.TrimSpace(rec[0])})
		}
	}
	return procs
}

func getClipboard() string {
	out, err := runHiddenOutput("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard -Raw")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func shutdownMachine() bool {
	return runHidden("shutdown", "/s", "/t", "5", "/c", "WinMonitor: remote shutdown") == nil
}

func restartMachine() bool {
	return runHidden("shutdown", "/r", "/t", "5", "/c", "WinMonitor: remote restart") == nil
}

func createShortcut(lnk, target, arguments, workdir string) bool {
	ps := "$W=New-Object -ComObject WScript.Shell;" +
		"$S=$W.CreateShortcut('" + strings.ReplaceAll(lnk, "'", "''") + "');" +
		"$S.TargetPath='" + strings.ReplaceAll(target, "'", "''") + "';" +
		"$S.Arguments='" + strings.ReplaceAll(arguments, "'", "''") + "';" +
		"$S.WorkingDirectory='" + strings.ReplaceAll(workdir, "'", "''") + "';" +
		"$S.WindowStyle=7;$S.Save()"
	err := runHidden("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden", "-Command", ps)
	if err != nil {
		return false
	}
	_, statErr := os.Stat(lnk)
	return statErr == nil
}

// ---- Keyboard hook (WH_KEYBOARD_LL) ----
type kbdLLHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type msgStruct struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	PtX      int32
	PtY      int32
	LPrivate uint32
}

var (
	keyEventSink func(string)
	hookHandle   uintptr
)

func keyboardProc(nCode int, wParam, lParam uintptr) uintptr {
	if nCode == 0 && (wParam == 0x100 || wParam == 0x104) {
		kb := (*kbdLLHookStruct)(unsafe.Pointer(lParam))
		shift := keyDown(0x10)
		if t := vkToText(int(kb.VkCode), shift); t != "" && keyEventSink != nil {
			keyEventSink(t)
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

// startKeyboardHook cài low-level keyboard hook (chính xác, ít tốn CPU - issue #15).
func startKeyboardHook(sink func(string)) bool {
	keyEventSink = sink
	cb := syscall.NewCallback(keyboardProc)
	done := make(chan bool, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		h, _, _ := procSetWindowsHookEx.Call(13, cb, 0, 0) // WH_KEYBOARD_LL=13
		if h == 0 {
			done <- false
			return
		}
		hookHandle = h
		done <- true
		var m msgStruct
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	select {
	case ok := <-done:
		hookOK = ok
	case <-time.After(2 * time.Second):
		hookOK = false
	}
	return hookOK
}

// runHidden chạy lệnh, ẩn cửa sổ.
func runHidden(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run()
}

func runHiddenOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	return string(out), err
}
