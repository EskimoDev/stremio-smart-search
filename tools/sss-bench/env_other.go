//go:build !windows && !linux

package main

import "strconv"

// systemCPU has no equivalent here without cgo, so the metric reports itself as
// unavailable rather than guessing.
func systemCPU() (cpuTimes, bool) { return cpuTimes{}, false }

// systemInfo uses macOS's sysctl and sw_vers; elsewhere the fields stay empty and
// the result records that it does not know.
func systemInfo() (cpu, version string, memory uint64) {
	cpu = runOutput("sysctl", "-n", "machdep.cpu.brand_string")
	version = runOutput("sw_vers", "-productVersion")
	memory, _ = strconv.ParseUint(runOutput("sysctl", "-n", "hw.memsize"), 10, 64)
	return cpu, version, memory
}
