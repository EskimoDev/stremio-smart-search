//go:build windows

package app

import (
	"os"
	"syscall"
	"unsafe"
)

var procMessageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

// outputHidden: a windowsgui program started from Explorer or a shortcut has no
// standard error handle at all. A console or redirected run has one.
func outputHidden() bool {
	_, err := os.Stderr.Stat()
	return err != nil
}

func messageBox(title, message string) {
	t, err1 := syscall.UTF16PtrFromString(title)
	m, err2 := syscall.UTF16PtrFromString(message)
	if err1 != nil || err2 != nil {
		return
	}
	const mbOK, mbIconError = 0x0, 0x10
	_, _, _ = procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbOK|mbIconError)
}
