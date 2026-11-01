package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceCleanupRemovesOnlyItsOwnFolder(t *testing.T) {
	parent := t.TempDir()
	neighbour := filepath.Join(parent, "keep-me.txt")
	if err := os.WriteFile(neighbour, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := newWorkspace(parent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(w.root), workspacePrefix) {
		t.Fatalf("workspace %s", w.root)
	}
	if err := os.WriteFile(filepath.Join(w.home, "temporary"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.root); !os.IsNotExist(err) {
		t.Fatal("workspace was not removed")
	}
	if _, err := os.Stat(neighbour); err != nil {
		t.Fatal("a neighbouring file was touched")
	}
}

func TestCleanupRefusesAFolderItDoesNotOwn(t *testing.T) {
	for name, prepare := range map[string]func(w *workspace){
		"marker removed":   func(w *workspace) { os.Remove(filepath.Join(w.root, workspaceMarker)) },
		"marker replaced":  func(w *workspace) { os.WriteFile(filepath.Join(w.root, workspaceMarker), []byte("other"), 0o600) },
		"different run id": func(w *workspace) { w.id = "another-run" },
	} {
		t.Run(name, func(t *testing.T) {
			w, err := newWorkspace(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			prepare(w)
			if err := w.cleanup(); err == nil {
				t.Fatal("cleanup should have refused")
			}
			if _, err := os.Stat(w.root); err != nil {
				t.Fatal("the folder should still exist")
			}
		})
	}
	// A folder carrying this run's marker but the wrong name is also left alone.
	root := filepath.Join(t.TempDir(), "important")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, workspaceMarker), []byte("id"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&workspace{root: root, id: "id"}).cleanup(); err == nil {
		t.Fatal("cleanup should have refused a differently named folder")
	}
}

func TestInstalledDataFolderIsRefused(t *testing.T) {
	config := t.TempDir()
	t.Setenv("APPDATA", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", config)
	installed := installedDataDir()
	if installed == "" {
		t.Skip("no user config folder on this platform")
	}
	for _, path := range []string{installed, filepath.Join(installed, "child"), filepath.Dir(installed)} {
		if _, err := newWorkspace(path); err == nil {
			t.Errorf("%s was accepted as a work folder", path)
		}
	}
	if err := refuseProtected(t.TempDir()); err != nil {
		t.Errorf("an unrelated folder was refused: %v", err)
	}
}

func TestChildEnvironmentCannotReachTheRealInstallation(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://real-proxy.example:3128")
	t.Setenv("GOGC", "off")
	t.Setenv("GOMEMLIMIT", "1GiB")
	w, err := newWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env, removed := childEnv(w)
	values := map[string]string{}
	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		if _, seen := values[strings.ToUpper(name)]; seen {
			t.Fatalf("%s is set twice", name)
		}
		values[strings.ToUpper(name)] = value
	}
	for _, name := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "TMP", "TEMP", "TMPDIR"} {
		if !sameOrInside(values[name], w.root) {
			t.Errorf("%s points outside the workspace: %s", name, values[name])
		}
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		if values[name] != "http://127.0.0.1:9" {
			t.Errorf("%s = %s", name, values[name])
		}
	}
	if _, ok := values["GOGC"]; ok {
		t.Error("GOGC reached the child")
	}
	if strings.Join(removed, ",") == "" {
		t.Error("removed tuning variables should be reported")
	}
}

func TestSameOrInside(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "data", "app")
	for path, expected := range map[string]bool{
		base:                               true,
		filepath.Join(base, "catalog"):     true,
		base + "-other":                    false,
		filepath.Dir(base):                 false,
		filepath.Join(base, "..", "other"): false,
	} {
		if got := sameOrInside(path, base); got != expected {
			t.Errorf("%s: %v", path, got)
		}
	}
}

func TestLockIsAdvisoryAndOnlyReleasedByItsOwner(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("TMP", os.Getenv("TMPDIR"))
	t.Setenv("TEMP", os.Getenv("TMPDIR"))
	first, err := newWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := newWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if warning := first.acquireLock(); warning != "" {
		t.Fatalf("first run warned: %s", warning)
	}
	if warning := second.acquireLock(); warning == "" {
		t.Fatal("second run should be warned")
	}
	second.releaseLock()
	if warning := second.acquireLock(); warning == "" {
		t.Fatal("the second run must not have released the first run's lock")
	}
	first.releaseLock()
	if warning := second.acquireLock(); warning != "" {
		t.Fatalf("after release: %s", warning)
	}
	second.releaseLock()
}

func TestOutputsRefuseToReplaceASavedResult(t *testing.T) {
	dir := t.TempDir()
	files := outputsFor(filepath.Join(dir, "nested", "before"))
	if err := files.claim(false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.markdown, []byte("saved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := files.claim(false); err == nil {
		t.Fatal("an existing report must not be replaced silently")
	}
	if err := files.claim(true); err != nil {
		t.Fatal(err)
	}
}
