//go:build darwin

package app

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ClientRunning reports whether a Stremio app is running. macOS has no /proc,
// so this lists process names with ps, which is part of the system.
func (a *App) ClientRunning() bool {
	out, err := exec.Command("/bin/ps", "-axco", "comm=").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if isClientProcess(filepath.Base(strings.TrimSpace(line))) {
			return true
		}
	}
	return false
}
