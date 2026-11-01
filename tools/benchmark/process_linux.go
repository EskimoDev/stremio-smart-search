//go:build linux

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// readProcess reports resident memory from /proc and CPU time from getrusage,
// so Linux runs can be compared with each other. Linux RSS is not the same
// counter as the Windows working set; do not mix the two in one table.
func readProcess() processStats {
	var stats processStats
	var usage syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) == nil {
		stats.CPUSeconds = float64(usage.Utime.Nano()+usage.Stime.Nano()) / 1e9
	}
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return stats
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		key, value, ok := strings.Cut(s.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}
		kib, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "VmRSS":
			stats.RSSBytes = kib << 10
		case "VmHWM":
			stats.PeakRSSBytes = kib << 10
		case "RssAnon":
			stats.PrivateBytes = kib << 10
		}
	}
	return stats
}

func clockMS() float64 { return float64(time.Now().UnixNano()) / 1e6 }
