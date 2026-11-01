//go:build linux || darwin

package app

import "syscall"

// diskFree reports the space available to this user on the drive holding path.
func diskFree(path string) (free, size uint64) {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0, 0
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
}
