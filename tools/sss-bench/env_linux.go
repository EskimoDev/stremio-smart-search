//go:build linux

package main

import (
	"os"
	"strings"
)

func systemInfo() (cpu, version string, memory uint64) {
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			key, value, ok := strings.Cut(line, ":")
			if ok && strings.TrimSpace(key) == "model name" {
				cpu = strings.TrimSpace(value)
				break
			}
		}
	}
	if cpu == "" { // ARM kernels often omit the model name
		cpu = runOutput("sh", "-c", "lscpu | sed -n 's/^Model name: *//p' | head -n1")
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if rest, ok := strings.CutPrefix(line, "MemTotal:"); ok {
				memory = parseKiB(rest)
				break
			}
		}
	}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if rest, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				version = strings.Trim(rest, `"`)
				break
			}
		}
	}
	if kernel := runOutput("uname", "-r"); kernel != "" {
		version = strings.TrimSpace(version + " (kernel " + kernel + ")")
	}
	return cpu, version, memory
}
