//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

var (
	advapi32           = syscall.NewLazyDLL("advapi32.dll")
	procRegSetValueEx  = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValue = advapi32.NewProc("RegDeleteValueW")
)

func openRunKey(access uint32) (syscall.Handle, error) {
	path, err := syscall.UTF16PtrFromString(runKey)
	if err != nil {
		return 0, err
	}
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, access, &key); err != nil {
		return 0, err
	}
	return key, nil
}

func runValueExists(name string) (bool, error) {
	key, err := openRunKey(syscall.KEY_QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer syscall.RegCloseKey(key)
	value, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	var kind, size uint32
	err = syscall.RegQueryValueEx(key, value, nil, &kind, nil, &size)
	if err == syscall.ERROR_FILE_NOT_FOUND {
		return false, nil
	}
	return err == nil, err
}

func setRunValue(name, command string) error {
	key, err := openRunKey(syscall.KEY_SET_VALUE)
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	value, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	data, err := syscall.UTF16FromString(command)
	if err != nil {
		return err
	}
	const regSZ = 1
	r, _, _ := procRegSetValueEx.Call(uintptr(key), uintptr(unsafe.Pointer(value)), 0, regSZ, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func deleteRunValue(name string) error {
	key, err := openRunKey(syscall.KEY_SET_VALUE)
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	value, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r, _, _ := procRegDeleteValue.Call(uintptr(key), uintptr(unsafe.Pointer(value)))
	if r != 0 && syscall.Errno(r) != syscall.ERROR_FILE_NOT_FOUND {
		return syscall.Errno(r)
	}
	return nil
}
