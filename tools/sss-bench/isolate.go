package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	workspacePrefix = "sss-bench-"
	workspaceMarker = ".sss-bench-workspace"
	lockName        = "sss-bench.lock"
	lockMaxAge      = 6 * time.Hour
	installedFolder = "StremioSmartSearch" // cmd/smart-search's default data folder
)

// workspace is the one directory a run may write to. The marker file is what
// makes it removable: cleanup deletes nothing that does not carry this run's id.
type workspace struct {
	root  string
	id    string
	home  string // stands in for the user's home in child processes
	locks string // lock file, if this run holds it
}

func randomID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// sameOrInside reports whether path is base or below it, after cleaning.
func sameOrInside(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// installedDataDir is the installed helper's data folder for this user.
func installedDataDir() string {
	config, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(config, installedFolder)
}

// refuseProtected rejects a directory a run must never write into.
func refuseProtected(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if installed := installedDataDir(); installed != "" && (sameOrInside(absolute, installed) || sameOrInside(installed, absolute)) {
		return fmt.Errorf("%s holds or is inside the installed Smart Search data folder; choose another directory", path)
	}
	return nil
}

// newWorkspace creates a fresh directory under parent, or under the system
// temporary directory if parent is empty. It never reuses an existing directory.
func newWorkspace(parent string) (*workspace, error) {
	if parent != "" {
		if err := refuseProtected(parent); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return nil, err
		}
	}
	root, err := os.MkdirTemp(parent, workspacePrefix+"*")
	if err != nil {
		return nil, err
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	if root, err = filepath.Abs(root); err != nil {
		return nil, err
	}
	w := &workspace{root: root, id: id, home: filepath.Join(root, "home")}
	if err := os.WriteFile(filepath.Join(root, workspaceMarker), []byte(id), 0o600); err != nil {
		return nil, err
	}
	return w, os.MkdirAll(w.home, 0o700)
}

// cleanup removes the workspace only if its marker still names this run.
func (w *workspace) cleanup() error {
	marker, err := os.ReadFile(filepath.Join(w.root, workspaceMarker))
	if err != nil || string(marker) != w.id || !strings.HasPrefix(filepath.Base(w.root), workspacePrefix) {
		return fmt.Errorf("refusing to remove %s: it is not this run's workspace", w.root)
	}
	// Windows keeps an executable locked briefly after its process exits.
	for attempt := 0; ; attempt++ {
		err = os.RemoveAll(w.root)
		if err == nil || attempt == 10 {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// childEnv is the environment of everything the harness runs from the target.
// The home and config variables point into the workspace, so even a code path
// that ignores --data-dir cannot reach the real installation. Any proxy setting
// is replaced by a closed local port, so an accidental download fails at once.
// Go runtime overrides are removed: they would change what is being measured.
func childEnv(w *workspace) ([]string, []string) {
	blocked := []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "TMP", "TEMP", "TMPDIR", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"}
	tuning := []string{"GOGC", "GOMEMLIMIT", "GOMAXPROCS", "GODEBUG", "GOTRACEBACK"}
	var env, removed []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch {
		case containsFold(blocked, name):
		case containsFold(tuning, name):
			removed = append(removed, name)
		default:
			env = append(env, entry)
		}
	}
	for _, name := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "TMP", "TEMP", "TMPDIR"} {
		env = append(env, name+"="+w.home)
	}
	env = append(env, "APPDATA="+filepath.Join(w.home, "AppData", "Roaming"), "LOCALAPPDATA="+filepath.Join(w.home, "AppData", "Local"))
	const closedPort = "http://127.0.0.1:9"
	env = append(env, "HTTP_PROXY="+closedPort, "HTTPS_PROXY="+closedPort, "ALL_PROXY="+closedPort)
	return env, removed
}

func containsFold(list []string, name string) bool {
	for _, item := range list {
		if strings.EqualFold(item, name) {
			return true
		}
	}
	return false
}

// buildEnv is the environment for the Go toolchain: native target, no cgo, no
// workspace file from the surrounding directories, and the user's own caches.
func buildEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !containsFold([]string{"GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOWORK"}, name) {
			env = append(env, entry)
		}
	}
	return append(env, "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
}

// acquireLock notes that a benchmark is running. Two benchmarks at once disturb
// each other's timings but cannot corrupt anything, so a held lock is a warning,
// not an error. A lock left by a crash goes stale.
func (w *workspace) acquireLock() (warning string) {
	path := filepath.Join(os.TempDir(), lockName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		info, statErr := os.Stat(path)
		if statErr == nil && time.Since(info.ModTime()) > lockMaxAge {
			os.Remove(path)
			file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		}
	}
	if err != nil {
		return "another sss-bench run appears to be active on this machine; timings may be disturbed"
	}
	fmt.Fprint(file, w.id+" "+strconv.Itoa(os.Getpid())+" "+runtime.GOOS)
	file.Close()
	w.locks = path
	return ""
}

func (w *workspace) releaseLock() {
	if w.locks == "" {
		return
	}
	if data, err := os.ReadFile(w.locks); err == nil && strings.HasPrefix(string(data), w.id+" ") {
		os.Remove(w.locks)
	}
	w.locks = ""
}
