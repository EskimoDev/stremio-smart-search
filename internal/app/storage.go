package app

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Storage describes the helper's data folder. The search index itself is
// rebuilt in memory from the catalogue file at start-up, so Memory reports
// the helper's current RAM footprint alongside the files on disk.
type Storage struct {
	Path      string `json:"path"`
	Total     int64  `json:"total"`
	Catalogue int64  `json:"catalogue"`
	Backup    int64  `json:"backup"`
	Other     int64  `json:"other"`
	Files     int    `json:"files"`
	Memory    uint64 `json:"memory"`
	DiskFree  uint64 `json:"diskFree"`
	DiskSize  uint64 `json:"diskSize"`
	CanOpen   bool   `json:"canOpen"`
}

func (a *App) Storage() Storage {
	s := Storage{Path: a.dataDir, CanOpen: a.OpenFolder != nil}
	if abs, err := filepath.Abs(a.dataDir); err == nil {
		s.Path = abs
	}
	catalogue := filepath.Join(a.dataDir, "catalog.json.gz")
	// The folder holds a handful of files, so walking it on request is cheap
	// and needs no background bookkeeping.
	_ = filepath.WalkDir(a.dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		size := info.Size()
		s.Total += size
		s.Files++
		switch path {
		case catalogue:
			s.Catalogue += size
		case catalogue + ".previous":
			s.Backup += size
		default:
			s.Other += size
		}
		return nil
	})
	s.DiskFree, s.DiskSize = diskFree(s.Path)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.Memory = m.Sys - m.HeapReleased
	return s
}

// OpenFolder shows a folder in the system file manager. The folder is always
// the helper's own data directory, never a path taken from a request.
func OpenFolder(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	prepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
