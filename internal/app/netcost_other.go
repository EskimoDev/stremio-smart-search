//go:build !windows

package app

// networkCost has no reliable, dependency-free source on macOS or Linux, so it
// says so instead of guessing. Automatic updates then follow the schedule.
func networkCost() string { return "unknown" }
