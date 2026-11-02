//go:build windows

package app

import (
	"testing"
	"unsafe"
)

// Shell_NotifyIconW rejects a structure whose size it does not recognise.
func TestNotifyIconDataMatchesWindowsLayout(t *testing.T) {
	want := uintptr(976) // sizeof(NOTIFYICONDATAW) on 64-bit Windows
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 956
	}
	if got := unsafe.Sizeof(notifyIconData{}); got != want {
		t.Fatalf("NOTIFYICONDATAW is %d bytes, want %d", got, want)
	}
}

// The tray must start and stop cleanly even where there is no taskbar to show
// it in, such as a CI service session: stop removes it and returns.
func TestTrayStartsAndStops(t *testing.T) {
	stop := StartTray(func() {}, func() {})
	stop()
	stop() // a second stop is harmless
}
