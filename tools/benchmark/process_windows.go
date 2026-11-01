//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var currentProcess = kernel.NewProc("GetCurrentProcess")
var processTimes = kernel.NewProc("GetProcessTimes")
var processMemory = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
var performanceCounter = kernel.NewProc("QueryPerformanceCounter")
var performanceFrequency = kernel.NewProc("QueryPerformanceFrequency")

func clockMS() float64 {
	var counter, frequency int64
	performanceCounter.Call(uintptr(unsafe.Pointer(&counter)))
	performanceFrequency.Call(uintptr(unsafe.Pointer(&frequency)))
	return float64(counter) * 1000 / float64(frequency)
}

type memoryCounters struct {
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

func readProcess() processStats {
	handle, _, _ := currentProcess.Call()
	var created, exited, kernelTime, userTime syscall.Filetime
	processTimes.Call(handle, uintptr(unsafe.Pointer(&created)), uintptr(unsafe.Pointer(&exited)), uintptr(unsafe.Pointer(&kernelTime)), uintptr(unsafe.Pointer(&userTime)))
	ticks := func(t syscall.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
	var memory memoryCounters
	memory.Size = uint32(unsafe.Sizeof(memory))
	processMemory.Call(handle, uintptr(unsafe.Pointer(&memory)), uintptr(memory.Size))
	return processStats{CPUSeconds: float64(ticks(kernelTime)+ticks(userTime)) / 1e7, RSSBytes: uint64(memory.WorkingSet), PeakRSSBytes: uint64(memory.PeakWorkingSet), PrivateBytes: uint64(memory.Private)}
}
