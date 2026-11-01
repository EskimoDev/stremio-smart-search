package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const catalogFile = "catalog.json.gz"

// catalogFiles are the only files a location change copies or removes.
var catalogFiles = []string{catalogFile, catalogFile + ".previous"}

// locationFile in the settings folder records a custom catalogue folder.
// Settings and the control key always stay in the settings folder, so other
// launches and --quit keep finding the running helper.
const locationFile = "storage.json"

// MovedFolderName is created inside the folder a user picks, so choosing a
// drive root or a busy folder never scatters catalogue files there.
const MovedFolderName = "Stremio Smart Search"

var (
	ErrMoveBusy        = errors.New("a catalogue update or move is already running")
	ErrPickerMissing   = errors.New("no folder picker is available on this computer")
	errNotEnoughSpace  = errors.New("not enough free space on that drive")
	errNotAbsolutePath = errors.New("choose a full folder path")
)

// Storage describes where the helper keeps its files and how big they are.
// The search index itself is rebuilt in memory from the catalogue file at
// start-up, so Memory reports the helper's current RAM footprint.
type Storage struct {
	Path        string `json:"path"`
	DefaultPath string `json:"defaultPath"`
	Custom      bool   `json:"custom"`
	Total       int64  `json:"total"`
	Catalogue   int64  `json:"catalogue"`
	Backup      int64  `json:"backup"`
	Other       int64  `json:"other"`
	Memory      uint64 `json:"memory"`
	DiskFree    uint64 `json:"diskFree"`
	DiskSize    uint64 `json:"diskSize"`
	CanOpen     bool   `json:"canOpen"`
	CanChoose   bool   `json:"canChoose"`
	Moving      bool   `json:"moving"`
}

func absPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// loadCatalogDir returns the catalogue folder recorded in the settings folder,
// or the settings folder itself.
func loadCatalogDir(dataDir string) string {
	var location struct {
		CatalogDir string `json:"catalogDir"`
	}
	data, err := os.ReadFile(filepath.Join(dataDir, locationFile))
	if err == nil && json.Unmarshal(data, &location) == nil && filepath.IsAbs(location.CatalogDir) {
		if info, err := os.Stat(location.CatalogDir); err == nil && info.IsDir() {
			return filepath.Clean(location.CatalogDir)
		}
	}
	return absPath(dataDir)
}

func (a *App) CatalogDir() string  { a.mu.Lock(); defer a.mu.Unlock(); return a.catalogDir }
func (a *App) catalogPath() string { return filepath.Join(a.CatalogDir(), catalogFile) }

func (a *App) Storage() Storage {
	a.mu.Lock()
	dir, moving := a.catalogDir, a.moving
	a.mu.Unlock()
	settings := absPath(a.dataDir)
	s := Storage{Path: dir, DefaultPath: settings, Custom: dir != settings, CanOpen: a.OpenFolder != nil, CanChoose: a.ChooseFolder != nil, Moving: moving}
	for i, name := range catalogFiles {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Mode().IsRegular() {
			if i == 0 {
				s.Catalogue = info.Size()
			} else {
				s.Backup = info.Size()
			}
		}
	}
	// The settings folder holds a handful of small files, so walking it on
	// request is cheap and needs no background bookkeeping.
	_ = filepath.WalkDir(settings, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if dir == settings && (path == filepath.Join(dir, catalogFiles[0]) || path == filepath.Join(dir, catalogFiles[1])) {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
			s.Other += info.Size()
		}
		return nil
	})
	s.Total = s.Catalogue + s.Backup + s.Other
	s.DiskFree, s.DiskSize = diskFree(dir)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.Memory = m.Sys - m.HeapReleased
	return s
}

// MoveCatalog moves the catalogue files into a "Stremio Smart Search" folder
// inside target, or back to the settings folder when target is empty. Search
// keeps working throughout because the index lives in memory.
func (a *App) MoveCatalog(target string) (string, error) {
	settings := absPath(a.dataDir)
	dest := settings
	if target != "" {
		if !filepath.IsAbs(target) {
			return "", errNotAbsolutePath
		}
		dest = filepath.Clean(target)
		if filepath.Base(dest) != MovedFolderName && dest != settings {
			dest = filepath.Join(dest, MovedFolderName)
		}
	}
	a.mu.Lock()
	if a.status.Building || a.moving {
		a.mu.Unlock()
		return "", ErrMoveBusy
	}
	source := a.catalogDir
	if source == dest {
		a.mu.Unlock()
		return dest, nil
	}
	a.moving = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.moving = false; a.mu.Unlock() }()

	if err := os.MkdirAll(dest, 0700); err != nil {
		return "", fmt.Errorf("could not create the folder: %w", err)
	}
	var needed uint64
	for _, name := range catalogFiles {
		if info, err := os.Stat(filepath.Join(source, name)); err == nil {
			needed += uint64(info.Size())
		}
	}
	if free, size := diskFree(dest); size > 0 && free < needed+64<<20 {
		return "", errNotEnoughSpace
	}
	for _, name := range catalogFiles {
		if err := copyFile(filepath.Join(source, name), filepath.Join(dest, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("could not copy the catalogue: %w", err)
		}
	}
	if err := saveCatalogDir(settings, dest); err != nil {
		for _, name := range catalogFiles {
			_ = os.Remove(filepath.Join(dest, name))
		}
		return "", fmt.Errorf("could not save the new location: %w", err)
	}
	a.mu.Lock()
	a.catalogDir = dest
	a.mu.Unlock()
	// Only the catalogue files this helper wrote are removed from the old folder.
	for _, name := range catalogFiles {
		_ = os.Remove(filepath.Join(source, name))
	}
	if source != settings {
		_ = os.Remove(source) // only succeeds if the folder is now empty
	}
	return dest, nil
}

func saveCatalogDir(settings, dir string) error {
	path := filepath.Join(settings, locationFile)
	if dir == settings {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, _ := json.Marshal(map[string]string{"catalogDir": dir})
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// copyFile writes to a temporary name and renames it into place, so an
// interrupted copy never leaves a truncated catalogue behind.
func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	temp := to + ".moving"
	out, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if syncErr := out.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		_ = os.Remove(to) // Windows cannot rename over an existing file
		err = os.Rename(temp, to)
	}
	if err != nil {
		_ = os.Remove(temp)
	}
	return err
}

// OpenFolder shows a folder in the system file manager. The folder is always
// the helper's own catalogue folder, never a path taken from a request.
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
	// Unlike the other launchers, never call prepareCommand here: on Windows it
	// sets HideWindow, and Explorer honours that by opening its window hidden.
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
