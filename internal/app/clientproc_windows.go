//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

// ClientRunning reports whether a Stremio app is running for any user on this
// computer. One Toolhelp snapshot lists process names without opening any
// process, so it needs no extra rights.
func (a *App) ClientRunning() bool {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(snapshot)
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		if isClientProcess(syscall.UTF16ToString(entry.ExeFile[:])) {
			return true
		}
	}
	return false
}
