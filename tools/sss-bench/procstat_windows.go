//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const processQueryLimited = 0x1000

var processMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

type memoryCountersEx struct {
	Size             uint32
	Faults           uint32
	PeakWorkingSet   uintptr
	WorkingSet       uintptr
	PeakPagedPool    uintptr
	PagedPool        uintptr
	PeakNonPagedPool uintptr
	NonPagedPool     uintptr
	Pagefile         uintptr
	PeakPagefile     uintptr
	Private          uintptr
}

func memoryCounters() (resident, private, peak string) {
	return "windows.WorkingSetSize", "windows.PrivateUsage", "windows.PeakWorkingSetSize"
}

func readProcess(pid int) (procSample, error) {
	handle, err := syscall.OpenProcess(processQueryLimited, false, uint32(pid))
	if err != nil {
		return procSample{}, err
	}
	defer syscall.CloseHandle(handle)
	var memory memoryCountersEx
	memory.Size = uint32(unsafe.Sizeof(memory))
	if ok, _, callErr := processMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&memory)), uintptr(memory.Size)); ok == 0 {
		return procSample{}, fmt.Errorf("GetProcessMemoryInfo: %w", callErr)
	}
	sample := procSample{Resident: uint64(memory.WorkingSet), Private: uint64(memory.Private), Peak: uint64(memory.PeakWorkingSet)}
	var creation, exit, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(handle, &creation, &exit, &kernel, &user) == nil {
		ticks := func(t syscall.Filetime) int64 { return int64(t.HighDateTime)<<32 | int64(t.LowDateTime) }
		sample.CPU, sample.HasCPU = time.Duration((ticks(kernel)+ticks(user))*100), true
	}
	return sample, nil
}

// peakAtExit adds nothing on Windows: the working-set peak is read while running.
func peakAtExit(*os.ProcessState) uint64 { return 0 }
