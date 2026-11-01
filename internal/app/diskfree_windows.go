//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// diskFree reports the space available to this user on the drive holding path.
func diskFree(path string) (free, size uint64) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	var available, total, totalFree uint64
	ok, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree)))
	if ok == 0 {
		return 0, 0
	}
	return available, total
}
