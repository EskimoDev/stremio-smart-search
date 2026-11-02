//go:build !windows

package app

import "os"

// outputHidden reports whether standard error goes somewhere the person will
// not see: /dev/null (Finder, launchd), a log file or the system journal. A
// terminal or a pipe means someone, or a script, is reading it.
func outputHidden() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return true
	}
	if info.Mode()&os.ModeNamedPipe != 0 {
		return false
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		null, err := os.Stat(os.DevNull)
		return err == nil && os.SameFile(info, null)
	}
	return true
}

func messageBox(string, string) {}
