//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// clockTicksPerSecond is USER_HZ, which Linux fixes at 100 for /proc on every
// architecture Go supports.
const clockTicksPerSecond = 100

func memoryCounters() (resident, private, peak string) {
	return "linux.VmRSS", "linux.RssAnon", "linux.VmHWM"
}

func readProcess(pid int) (procSample, error) {
	var sample procSample
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return sample, err
	}
	for _, line := range strings.Split(string(status), "\n") {
		key, value, ok := strings.Cut(line, ":")
		fields := strings.Fields(value)
		if !ok || len(fields) == 0 {
			continue
		}
		kib, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "VmRSS":
			sample.Resident = kib << 10
		case "VmHWM":
			sample.Peak = kib << 10
		case "RssAnon":
			sample.Private = kib << 10
		}
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil {
		// The command name is in parentheses and may contain spaces; utime and
		// stime are the 12th and 13th fields after it.
		if i := bytes.LastIndexByte(stat, ')'); i >= 0 {
			if fields := strings.Fields(string(stat[i+1:])); len(fields) > 12 {
				user, userErr := strconv.ParseInt(fields[11], 10, 64)
				system, systemErr := strconv.ParseInt(fields[12], 10, 64)
				if userErr == nil && systemErr == nil {
					sample.CPU, sample.HasCPU = time.Duration(user+system)*time.Second/clockTicksPerSecond, true
				}
			}
		}
	}
	return sample, nil
}

// peakAtExit adds nothing on Linux: VmHWM is read while running, and ru_maxrss can
// report the parent's size because of how a child is started.
func peakAtExit(*os.ProcessState) uint64 { return 0 }
