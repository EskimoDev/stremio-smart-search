//go:build windows

package main

import (
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// registryString reads one value under HKEY_LOCAL_MACHINE.
func registryString(path, name string) string {
	key, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	var handle syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, key, 0, syscall.KEY_READ, &handle) != nil {
		return ""
	}
	defer syscall.RegCloseKey(handle)
	value, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return ""
	}
	buffer := make([]uint16, 256)
	size := uint32(len(buffer) * 2)
	if syscall.RegQueryValueEx(handle, value, nil, nil, (*byte)(unsafe.Pointer(&buffer[0])), &size) != nil {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(buffer))
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// systemCPU reads GetSystemTimes, whose kernel time includes the idle time.
func systemCPU() (cpuTimes, bool) {
	var idle, kernel, user syscall.Filetime
	if ok, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemTimes").Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); ok == 0 {
		return cpuTimes{}, false
	}
	ticks := func(t syscall.Filetime) time.Duration {
		return time.Duration(int64(t.HighDateTime)<<32|int64(t.LowDateTime)) * 100
	}
	total := ticks(kernel) + ticks(user)
	return cpuTimes{busy: total - ticks(idle), total: total}, true
}

func systemInfo() (cpu, version string, memory uint64) {
	cpu = registryString(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString")
	const current = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	version = strings.TrimSpace(fmt.Sprintf("build %s %s", registryString(current, "CurrentBuild"), registryString(current, "DisplayVersion")))
	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	if ok, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&status))); ok != 0 {
		memory = status.TotalPhys
	}
	return cpu, version, memory
}
