//go:build windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

// enableVirtualTerminalProcessing lets a Windows console interpret ANSI colors.
const enableVirtualTerminalProcessing = 0x0004

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleMode        = kernel32.NewProc("SetConsoleMode")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

// OwnsConsoleWindow reports whether Windows opened a console window just for
// this program, as it does when the .exe is double-clicked. That window
// closes as soon as the program exits. When started from a terminal, the
// terminal's shell shares the console too, so there are at least two processes.
func OwnsConsoleWindow() bool {
	pids := make([]uint32, 2)
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}

// enableVT switches on ANSI color support for a Windows console and reports
// whether colors can be used.
func enableVT(f *os.File) bool {
	h := syscall.Handle(f.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	ok, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
	return ok != 0
}
