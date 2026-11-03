//go:build linux

package app

import "os"

// ClientRunning reports whether a Stremio app is running, from the process
// names in /proc. It reads one small file per process and starts nothing.
func (a *App) ClientRunning() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		comm, err := os.ReadFile("/proc/" + name + "/comm")
		if err == nil && isClientProcess(string(comm)) {
			return true
		}
	}
	return false
}
