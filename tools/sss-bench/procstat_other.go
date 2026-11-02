//go:build !windows && !linux && !darwin

package main

import (
	"errors"
	"os"
)

func memoryCounters() (resident, private, peak string) { return "", "", "" }

func readProcess(int) (procSample, error) {
	return procSample{}, errors.New("process counters are not implemented on this platform")
}

func peakAtExit(*os.ProcessState) uint64 { return 0 }
