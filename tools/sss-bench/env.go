package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/EskimoDev/stremio-smart-search/tools/sss-bench/probes/probekit"
)

// describeEnvironment records the machine without identifying its owner.
func describeEnvironment() Environment {
	cpu, version, memory := systemInfo()
	env := Environment{OS: runtime.GOOS, Arch: runtime.GOARCH, OSVersion: version, CPUModel: cpu, LogicalCPUs: runtime.NumCPU(), MemoryTotalBytes: memory, ClockResolutionNS: probekit.Resolution().Nanoseconds()}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%d", env.OS, env.Arch, env.CPUModel, env.LogicalCPUs, env.MemoryTotalBytes)))
	env.HardwareID = hex.EncodeToString(sum[:])[:16]
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		env.CI = &CIInfo{Provider: "github-actions", Hosted: os.Getenv("RUNNER_ENVIRONMENT") == "github-hosted", Runner: os.Getenv("RUNNER_OS") + "/" + os.Getenv("RUNNER_ARCH"), RunID: os.Getenv("GITHUB_RUN_ID")}
	}
	return env
}

// runOutput runs a small system command and returns its trimmed output.
func runOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseMemTotal reads a /proc/meminfo-style value in kibibytes.
func parseKiB(value string) uint64 {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0
	}
	kib, _ := strconv.ParseUint(fields[0], 10, 64)
	return kib << 10
}

// cpuTimes is cumulative CPU time across all logical CPUs since boot.
type cpuTimes struct{ busy, total time.Duration }

// busyShare is the percentage of machine-wide CPU used between two readings.
func busyShare(before, after cpuTimes) (float64, bool) {
	total := after.total - before.total
	if total <= 0 {
		return 0, false
	}
	return 100 * float64(after.busy-before.busy) / float64(total), true
}

// parseProcStat reads the aggregate cpu line of /proc/stat: user, nice, system, idle,
// iowait, irq, softirq, steal. Waiting on a disk counts as idle, not as work.
func parseProcStat(text string) (cpuTimes, bool) {
	line, _, _ := strings.Cut(text, "\n")
	fields := strings.Fields(line)
	if len(fields) < 6 || fields[0] != "cpu" {
		return cpuTimes{}, false
	}
	var times cpuTimes
	for i, field := range fields[1:] {
		if i >= 8 { // later columns (guest time) are already included in user
			break
		}
		ticks, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			return cpuTimes{}, false
		}
		spent := time.Duration(ticks) * time.Second / 100 // USER_HZ is always 100 in /proc
		times.total += spent
		if i != 3 && i != 4 {
			times.busy += spent
		}
	}
	return times, true
}

// installedHelperRunning checks for something on the default port. It only
// connects: it never speaks to a running helper, which stays undisturbed.
func installedHelperRunning() bool {
	connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", defaultAppPort), 300*time.Millisecond)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}

// findRepoRoot walks up to this module's go.mod.
func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module github.com/EskimoDev/stremio-smart-search") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s is not inside the stremio-smart-search repository; run from there or pass --repo", start)
		}
		dir = parent
	}
}
