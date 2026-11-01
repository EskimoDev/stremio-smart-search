//go:build !linux && !darwin && !windows

package app

func diskFree(string) (free, size uint64) { return 0, 0 }
