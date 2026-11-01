//go:build windows

package app

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// ChooseFolderNative shows the Explorer-style folder picker (IFileOpenDialog
// with FOS_PICKFOLDERS) on this computer, starting in start. An empty result
// with no error means the user cancelled. It calls the shell's COM API
// directly, so no helper process or script is involved.
func ChooseFolderNative(start string) (string, error) {
	// COM dialogs need a single-threaded apartment on one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr := hresult(procCoInitializeEx.Call(0, coinitApartmentThreaded)); hr < 0 {
		return "", fmt.Errorf("CoInitializeEx: %#x", uint32(hr))
	}
	defer procCoUninitialize.Call()

	var dialog *comObject
	if hr := hresult(procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInprocServer, uintptr(unsafe.Pointer(&iidFileOpenDialog)), uintptr(unsafe.Pointer(&dialog)))); hr < 0 || dialog == nil {
		return "", ErrPickerMissing
	}
	defer dialog.call(vtRelease)

	var options uint32
	dialog.call(vtGetOptions, uintptr(unsafe.Pointer(&options)))
	dialog.call(vtSetOptions, uintptr(options|fosPickFolders|fosForceFileSystem|fosPathMustExist))
	if title, err := syscall.UTF16PtrFromString("Choose where Stremio Smart Search keeps its catalogue"); err == nil {
		dialog.call(vtSetTitle, uintptr(unsafe.Pointer(title)))
	}
	if label, err := syscall.UTF16PtrFromString("Use this folder"); err == nil {
		dialog.call(vtSetOkButtonLabel, uintptr(unsafe.Pointer(label)))
	}
	if path, err := syscall.UTF16PtrFromString(start); err == nil && start != "" {
		var folder *comObject
		if hr := hresult(procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&folder)))); hr >= 0 && folder != nil {
			dialog.call(vtSetFolder, uintptr(unsafe.Pointer(folder)))
			folder.call(vtRelease)
		}
	}

	// Own the dialog by the window in front (the settings page's browser), so it
	// opens on top of it rather than behind.
	owner, _, _ := procGetForegroundWindow.Call()
	hr := dialog.call(vtShow, owner)
	if hr == errCancelled {
		return "", nil
	}
	if hr < 0 {
		return "", fmt.Errorf("folder picker: %#x", uint32(hr))
	}
	var item *comObject
	if hr := dialog.call(vtGetResult, uintptr(unsafe.Pointer(&item))); hr < 0 || item == nil {
		return "", fmt.Errorf("folder picker result: %#x", uint32(hr))
	}
	defer item.call(vtRelease)
	var name *uint16
	if hr := item.call(vtGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); hr < 0 || name == nil {
		return "", fmt.Errorf("folder path: %#x", uint32(hr))
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(name)))
	return utf16PtrToString(name), nil
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidFileOpenDialog = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidFileOpenDialog   = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidShellItem        = guid{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}

	ole32                           = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx              = ole32.NewProc("CoInitializeEx")
	procCoUninitialize              = ole32.NewProc("CoUninitialize")
	procCoCreateInstance            = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree               = ole32.NewProc("CoTaskMemFree")
	procSHCreateItemFromParsingName = syscall.NewLazyDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
	procGetForegroundWindow         = syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow")
)

const (
	coinitApartmentThreaded = 0x2
	clsctxInprocServer      = 0x1
	fosPickFolders          = 0x20
	fosForceFileSystem      = 0x40
	fosPathMustExist        = 0x800
	sigdnFileSysPath        = 0x80058000
	errCancelled            = int32(-2147023673) // HRESULT_FROM_WIN32(ERROR_CANCELLED), 0x800704C7
)

// Vtable slots: IUnknown, then IModalWindow::Show, then IFileDialog in order.
// IShellItem::GetDisplayName is slot 5 of its own vtable.
const (
	vtRelease          = 2
	vtShow             = 3
	vtSetOptions       = 9
	vtGetOptions       = 10
	vtSetFolder        = 12
	vtSetTitle         = 17
	vtSetOkButtonLabel = 18
	vtGetResult        = 20
	vtGetDisplayName   = 5
)

type comObject struct{ vtable *[32]uintptr }

// uintptrescapes moves any pointer converted to uintptr at a call site to the
// heap and keeps it alive for the call, as syscall.Proc.Call does.
//
//go:uintptrescapes
func (o *comObject) call(slot int, args ...uintptr) int32 {
	r, _, _ := syscall.SyscallN(o.vtable[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return int32(r)
}

func hresult(r uintptr, _ uintptr, _ error) int32 { return int32(r) }

func utf16PtrToString(p *uint16) string {
	var chars []uint16
	for ptr := unsafe.Pointer(p); ; ptr = unsafe.Add(ptr, 2) {
		c := *(*uint16)(ptr)
		if c == 0 {
			break
		}
		chars = append(chars, c)
	}
	return syscall.UTF16ToString(chars)
}
