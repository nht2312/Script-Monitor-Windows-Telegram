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

	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW         = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW   = user32.NewProc("GetWindowTextLengthW")
	procBlockInput             = user32.NewProc("BlockInput")
	procGetAsyncKeyState       = user32.NewProc("GetAsyncKeyState")
	procLockWorkStation        = user32.NewProc("LockWorkStation")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procShowWindow             = user32.NewProc("ShowWindow")
	procGetConsoleWindow       = kernel32.NewProc("GetConsoleWindow")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procSetFileAttributesW     = kernel32.NewProc("SetFileAttributesW")
	procCreateMutexW           = kernel32.NewProc("CreateMutexW")
	procMoveFileExW            = kernel32.NewProc("MoveFileExW")
)

// AutoUnblockNotify được main gán để báo khi bàn phím/chuột tự mở khóa.
var AutoUnblockNotify func()

func hideConsole() {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd != 0 {
		procShowWindow.Call(hwnd, 0) // SW_HIDE
	}
}

func getSysMetric(idx int) int {
	r, _, _ := procGetSystemMetrics.Call(uintptr(idx))
	return int(int32(r))
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
	x := getSysMetric(76) // SM_XVIRTUALSCREEN
	y := getSysMetric(77) // SM_YVIRTUALSCREEN
	w := getSysMetric(78) // SM_CXVIRTUALSCREEN
	h := getSysMetric(79) // SM_CYVIRTUALSCREEN
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
	*(*int32)(unsafe.Pointer(&bmi[8])) = -int32(h) // top-down
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
	if errno, ok := err.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
		return false
	}
	return true
}

// ---- BlockInput (phải giữ trên 1 OS thread) ----
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
		runtime.LockOSThread() // BlockInput gắn với OS thread
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
		procBlockInput.Call(0)
		blockMu.Lock()
		blockActive = false
		blockMu.Unlock()
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

// runHidden chạy lệnh, ẩn cửa sổ.
func runHidden(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd.Run()
}

func runHiddenOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.Output()
	return string(out), err
}
