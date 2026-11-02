//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// macOS has no /proc, and reading another task needs cgo, which this project does
// not build with. ps is the supported interface; it costs a process per sample, so
// the harness never polls it during latency passes.
func memoryCounters() (resident, private, peak string) {
	return "darwin.ps_rss", "", "darwin.ru_maxrss"
}

func readProcess(pid int) (procSample, error) {
	out, err := exec.Command("ps", "-o", "rss=,cputime=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return procSample{}, fmt.Errorf("ps: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return procSample{}, fmt.Errorf("ps returned %q", out)
	}
	kib, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return procSample{}, err
	}
	sample := procSample{Resident: kib << 10}
	if cpu, err := parseCPUTime(fields[1]); err == nil {
		sample.CPU, sample.HasCPU = cpu, true
	}
	return sample, nil
}

// parseCPUTime reads ps's [hh:]mm:ss.cc.
func parseCPUTime(text string) (time.Duration, error) {
	parts := strings.Split(text, ":")
	seconds, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil {
		return 0, err
	}
	scale := 60.0
	for i := len(parts) - 2; i >= 0; i-- {
		value, err := strconv.ParseFloat(parts[i], 64)
		if err != nil {
			return 0, err
		}
		seconds += value * scale
		scale *= 60
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// peakAtExit is the only peak macOS offers another process. ru_maxrss is in bytes
// here, not KiB as on Linux.
func peakAtExit(state *os.ProcessState) uint64 {
	if usage, ok := state.SysUsage().(*syscall.Rusage); ok && usage.Maxrss > 0 {
		return uint64(usage.Maxrss)
	}
	return 0
}
