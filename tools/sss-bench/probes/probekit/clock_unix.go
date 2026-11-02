//go:build !windows

package probekit

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var started = time.Now()

// Nanos is a monotonic timestamp.
func Nanos() int64 { return time.Since(started).Nanoseconds() }

// ProcessCPU is the user and system time this process has used.
func ProcessCPU() time.Duration {
	var usage syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) != nil {
		return 0
	}
	return time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
}

// PeakResident is this process's peak resident memory: VmHWM on Linux, whose
// ru_maxrss can report the parent's size, and ru_maxrss elsewhere.
func PeakResident() uint64 {
	if runtime.GOOS == "linux" {
		status, err := os.ReadFile("/proc/self/status")
		if err != nil {
			return 0
		}
		for _, line := range strings.Split(string(status), "\n") {
			if rest, ok := strings.CutPrefix(line, "VmHWM:"); ok {
				if fields := strings.Fields(rest); len(fields) > 0 {
					kib, _ := strconv.ParseUint(fields[0], 10, 64)
					return kib << 10
				}
			}
		}
		return 0
	}
	var usage syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) != nil {
		return 0
	}
	return uint64(usage.Maxrss)
}
