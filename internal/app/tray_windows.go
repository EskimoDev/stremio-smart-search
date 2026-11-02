//go:build windows

package app

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// StartTray puts the app's icon in the Windows notification area. Double-click,
// Enter or the menu's first item calls open; the menu's last item calls quit.
// The icon lives on one locked OS thread with its own message loop, so starting
// it never delays the server. If the taskbar is missing (a service session) or
// slow to appear at sign-in, the icon is added when Explorer announces it.
// stop removes the icon and waits briefly for that, so no stale icon is left
// behind after the process exits.
func StartTray(open, quit func()) (stop func()) {
	ready := make(chan syscall.Handle, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		trayLoop(open, quit, ready)
	}()
	hwnd := <-ready
	if hwnd == 0 {
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			procPostMessage.Call(uintptr(hwnd), wmClose, 0, 0)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}
		})
	}
}

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procRegisterWindowMsg   = user32.NewProc("RegisterWindowMessageW")
	procLoadImage           = user32.NewProc("LoadImageW")
	procDestroyIcon         = user32.NewProc("DestroyIcon")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procSetMenuDefaultItem  = user32.NewProc("SetMenuDefaultItem")
	procTrackPopupMenuEx    = user32.NewProc("TrackPopupMenuEx")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")
	procSetThreadDpiContext = user32.NewProc("SetThreadDpiAwarenessContext")
	procGetModuleHandle     = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	procShellNotifyIcon     = syscall.NewLazyDLL("shell32.dll").NewProc("Shell_NotifyIconW")
)

const (
	wmNull        = 0x0000
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmContextMenu = 0x007B
	wmTimer       = 0x0113
	wmLButtonDbl  = 0x0203
	wmApp         = 0x8000
	trayCallback  = wmApp + 1
	ninKeySelect  = 0x0401 // WM_USER+1: Enter or Space on the focused icon

	nimAdd        = 0
	nimDelete     = 2
	nimSetVersion = 4
	nifMessage    = 0x01
	nifIcon       = 0x02
	nifTip        = 0x04
	nifShowTip    = 0x80
	notifyIconV4  = 4

	imageIcon       = 1
	smCxSmIcon      = 49
	smCySmIcon      = 50
	smMenuDropAlign = 40
	mfString        = 0x0
	mfSeparator     = 0x800
	tpmRightAlign   = 0x0008
	tpmNoNotify     = 0x0080
	tpmReturnCmd    = 0x0100
	tpmRightButton  = 0x0002

	menuOpen    = 1
	menuQuit    = 2
	retryTimer  = 1
	retryEvery  = 2000 // ms
	retryTimes  = 15   // about 30 s after sign-in for a slow taskbar
	appIconID   = 1    // RT_GROUP_ICON written by tools/icons
	trayIconID  = 1
	perMonitor2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
)

// notifyIconData is NOTIFYICONDATAW. Go lays these fields out as C does on both
// 64-bit (976 bytes) and 32-bit Windows; tray_windows_test.go checks the size.
type notifyIconData struct {
	Size            uint32
	Wnd             syscall.Handle
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            syscall.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        guid
	BalloonIcon     syscall.Handle
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

type winMsg struct {
	Wnd     syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

// One tray per process; its state belongs to the tray thread only.
var tray struct {
	data           notifyIconData
	open, quit     func()
	taskbarCreated uintptr
	retries        int
	lastOpen       time.Time
}

var trayWndProc = syscall.NewCallback(func(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch {
	case msg == trayCallback:
		switch uint32(lParam & 0xFFFF) {
		case wmLButtonDbl, ninKeySelect:
			trayOpen()
		case wmContextMenu:
			// Version 4 sends the anchor point in wParam as two signed 16-bit
			// values; they are negative on a monitor left of or above the main one.
			showTrayMenu(hwnd, int32(int16(wParam&0xFFFF)), int32(int16(wParam>>16&0xFFFF)))
		}
		return 0
	case msg == wmTimer && wParam == retryTimer:
		if addTrayIcon() || tray.retries >= retryTimes {
			procKillTimer.Call(uintptr(hwnd), retryTimer)
		}
		tray.retries++
		return 0
	case msg == wmClose:
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case msg == wmDestroy:
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&tray.data)))
		if tray.data.Icon != 0 {
			procDestroyIcon.Call(uintptr(tray.data.Icon))
		}
		procPostQuitMessage.Call(0)
		return 0
	case tray.taskbarCreated != 0 && uintptr(msg) == tray.taskbarCreated:
		// Explorer restarted, or the taskbar appeared after this process did.
		addTrayIcon()
		return 0
	}
	r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
})

func trayLoop(open, quit func(), ready chan<- syscall.Handle) {
	tray.open, tray.quit = open, quit
	// A crisp icon on scaled displays: only this hidden window's thread becomes
	// DPI aware (Windows 10 1607+); the rest of the process is unchanged.
	if procSetThreadDpiContext.Find() == nil {
		procSetThreadDpiContext.Call(perMonitor2)
	}
	instance, _, _ := procGetModuleHandle.Call(0)
	className, _ := syscall.UTF16PtrFromString("StremioSmartSearchTray")
	class := wndClassEx{WndProc: trayWndProc, Instance: syscall.Handle(instance), ClassName: className}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		ready <- 0
		return
	}
	title, _ := syscall.UTF16PtrFromString(Product)
	// A hidden top-level window rather than a message-only one: only top-level
	// windows receive the TaskbarCreated broadcast. It is never shown.
	hwnd, _, _ := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		ready <- 0
		return
	}
	if name, err := syscall.UTF16PtrFromString("TaskbarCreated"); err == nil {
		tray.taskbarCreated, _, _ = procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(name)))
	}
	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	icon, _, _ := procLoadImage.Call(instance, appIconID, imageIcon, cx, cy, 0)
	tray.data = notifyIconData{Wnd: syscall.Handle(hwnd), ID: trayIconID, Flags: nifMessage | nifIcon | nifTip | nifShowTip, CallbackMessage: trayCallback, Icon: syscall.Handle(icon), Version: notifyIconV4}
	tray.data.Size = uint32(unsafe.Sizeof(tray.data))
	tip, _ := syscall.UTF16FromString(Product)
	copy(tray.data.Tip[:len(tray.data.Tip)-1], tip)
	if !addTrayIcon() {
		procSetTimer.Call(hwnd, retryTimer, retryEvery, 0)
	}
	ready <- syscall.Handle(hwnd)

	var m winMsg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // WM_QUIT, or an invalid call
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func addTrayIcon() bool {
	if ok, _, _ := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&tray.data))); ok == 0 {
		return false
	}
	// Version 4 gives keyboard selection and a WM_CONTEXTMENU for Shift+F10 and
	// the menu key, so the icon works without a mouse.
	procShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&tray.data)))
	return true
}

// trayOpen ignores repeats within a second: one double-click or Enter press
// can arrive as two events, and each would open another browser tab.
func trayOpen() {
	if now := time.Now(); now.Sub(tray.lastOpen) > time.Second {
		tray.lastOpen = now
		go tray.open()
	}
}

func showTrayMenu(hwnd syscall.Handle, x, y int32) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	for _, item := range []struct {
		id    uintptr
		flags uintptr
		text  string
	}{{menuOpen, mfString, "&Open dashboard"}, {0, mfSeparator, ""}, {menuQuit, mfString, "&Quit Smart Search"}} {
		text, _ := syscall.UTF16PtrFromString(item.text)
		procAppendMenu.Call(menu, item.flags, item.id, uintptr(unsafe.Pointer(text)))
	}
	procSetMenuDefaultItem.Call(menu, menuOpen, 0) // bold, as double-click does it
	flags := uintptr(tpmReturnCmd | tpmNoNotify | tpmRightButton)
	if right, _, _ := procGetSystemMetrics.Call(smMenuDropAlign); right != 0 {
		flags |= tpmRightAlign
	}
	// Without the foreground call the menu stays open after clicking elsewhere;
	// the WM_NULL afterwards is the documented fix for the menu closing early.
	procSetForegroundWindow.Call(uintptr(hwnd))
	cmd, _, _ := procTrackPopupMenuEx.Call(menu, flags, uintptr(x), uintptr(y), uintptr(hwnd), 0)
	procPostMessage.Call(uintptr(hwnd), wmNull, 0, 0)
	switch cmd {
	case menuOpen:
		trayOpen()
	case menuQuit:
		go tray.quit()
	}
}
