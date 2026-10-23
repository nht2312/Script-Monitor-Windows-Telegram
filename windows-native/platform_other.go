//go:build !windows

package main

// Stub cho môi trường không phải Windows (chỉ để biên dịch/test logic).
// Bản thực tế build cho Windows trong platform_windows.go.

var AutoUnblockNotify func()

func hideConsole()                             {}
func setDpiAwareness()                         {}
func isRemoteSession() bool                    { return false }
func isElevated() bool                         { return false }
func foregroundTitle() string                  { return "" }
func takeScreenshot(path string) bool          { return false }
func lockWorkstation() bool                    { return false }
func keyDown(vk int) bool                      { return false }
func hideFile(path string)                     {}
func restrictToCurrentUser(path string)        {}
func moveFileOnReboot(path string)             {}
func acquireSingleInstance() bool              { return true }
func isInputBlocked() bool                     { return false }
func StartBlock(auto int) (bool, bool)         { return false, false }
func StopBlock() bool                          { return false }
func startKeyboardHook(sink func(string)) bool { return false }

type Proc struct {
	PID  string
	Name string
}

func listProcesses() []Proc                                       { return nil }
func getClipboard() string                                        { return "" }
func shutdownMachine() bool                                       { return false }
func restartMachine() bool                                        { return false }
func createShortcut(l, t, a, w string) bool                       { return false }
func runHidden(name string, args ...string) error                 { return nil }
func runHiddenOutput(name string, args ...string) (string, error) { return "", nil }

func releaseSingleInstance()                {}
func relaunchCanonical(args []string) error { return nil }
