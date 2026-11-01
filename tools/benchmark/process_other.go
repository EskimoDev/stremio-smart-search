//go:build !windows && !linux

package main

import "time"

// The reproducible working-set/CPU comparison currently targets Windows.
// Runtime heap and wall-time measurements remain available on other platforms.
func readProcess() processStats { return processStats{} }

func clockMS() float64 { return float64(time.Now().UnixNano()) / 1e6 }
