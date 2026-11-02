//go:build windows

package probekit

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	performanceCount = kernel32.NewProc("QueryPerformanceCounter")
	performanceFreq  = kernel32.NewProc("QueryPerformanceFrequency")
	ticksPerSecond   = func() int64 {
		var frequency int64
		performanceFreq.Call(uintptr(unsafe.Pointer(&frequency)))
		return frequency
	}()
)

// Nanos is a monotonic timestamp. On Windows time.Now only advances every few
// hundred microseconds, as long as many searches take, so it uses
// QueryPerformanceCounter instead.
func Nanos() int64 {
	var ticks int64
	performanceCount.Call(uintptr(unsafe.Pointer(&ticks)))
	// Split the conversion: ticks*1e9 overflows after minutes of uptime.
	return ticks/ticksPerSecond*1e9 + ticks%ticksPerSecond*1e9/ticksPerSecond
}

// ProcessCPU is the user and kernel time this process has used. Windows updates
// it on the scheduler tick, about every 15 ms.
func ProcessCPU() time.Duration {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var creation, exit, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(handle, &creation, &exit, &kernel, &user) != nil {
		return 0
	}
	ticks := func(t syscall.Filetime) int64 { return int64(t.HighDateTime)<<32 | int64(t.LowDateTime) }
	return time.Duration((ticks(kernel) + ticks(user)) * 100)
}

type memoryCounters struct {
	Size           uint32
	Faults         uint32
	PeakWorkingSet uintptr
	WorkingSet     uintptr
	Pools          [6]uintptr
	Private        uintptr
}

var processMemory = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// PeakResident is this process's peak working set.
func PeakResident() uint64 {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var memory memoryCounters
	memory.Size = uint32(unsafe.Sizeof(memory))
	if ok, _, _ := processMemory.Call(uintptr(handle), uintptr(unsafe.Pointer(&memory)), uintptr(memory.Size)); ok == 0 {
		return 0
	}
	return uint64(memory.PeakWorkingSet)
}
