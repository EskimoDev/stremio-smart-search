package app

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageReportsDataFolderUsage(t *testing.T) {
	a := testApp(t)
	write := func(name string, size int) {
		if err := os.WriteFile(filepath.Join(a.dataDir, name), make([]byte, size), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("catalog.json.gz", 1000)
	write("catalog.json.gz.previous", 400)
	w := request(a, "GET", "/api/storage", "", a.Token)
	if w.Code != 200 {
		t.Fatalf("storage status %d: %s", w.Code, w.Body)
	}
	var s Storage
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	// control.key and updates.json are created by New and count as other data.
	settings, err := os.Stat(filepath.Join(a.dataDir, updateSettingsFile))
	if err != nil {
		t.Fatal(err)
	}
	other := 64 + settings.Size()
	if s.Catalogue != 1000 || s.Backup != 400 || s.Other != other || s.Total != 1400+other || s.Custom || s.Memory == 0 || !s.CanOpen || !filepath.IsAbs(s.Path) {
		t.Fatalf("unexpected storage report: %+v", s)
	}
}

func TestOpenFolderOnlyForLocalSettingsPage(t *testing.T) {
	a := testApp(t)
	var opened []string
	a.OpenFolder = func(path string) error { opened = append(opened, path); return nil }
	// Wrong key, foreign origin, and a LAN client with the right key are all refused.
	if got := request(a, "POST", "/api/storage/open", "", "wrong"); got.Code != 403 {
		t.Fatalf("wrong key accepted: %d", got.Code)
	}
	if got := request(a, "POST", "/api/storage/open", "https://unrelated.example", a.Token); got.Code != 403 {
		t.Fatalf("foreign origin accepted: %d", got.Code)
	}
	for _, path := range []string{"/api/storage/open", "/api/storage"} {
		method := "POST"
		if path == "/api/storage" {
			method = "GET"
		}
		r := httptest.NewRequest(method, "http://127.0.0.1:11479"+path, nil)
		r.RemoteAddr = "192.168.1.4:5000"
		r.Header.Set("X-Smart-Search-Key", a.Token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("LAN request to %s accepted: %d", path, w.Code)
		}
	}
	if len(opened) != 0 {
		t.Fatal("folder opened for a refused request")
	}
	if got := request(a, "GET", "/api/storage/open", "", a.Token); got.Code != 404 {
		t.Fatalf("GET should not open the folder: %d", got.Code)
	}
	if got := request(a, "POST", "/api/storage/open", "http://127.0.0.1:11479", a.Token); got.Code != 200 {
		t.Fatalf("local open failed: %d %s", got.Code, got.Body)
	}
	abs, _ := filepath.Abs(a.dataDir)
	if len(opened) != 1 || opened[0] != abs {
		t.Fatalf("opened %v, want only the data folder", opened)
	}
}

func postJSON(a *App, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://127.0.0.1:11479"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:32100"
	r.Header.Set("Origin", "http://127.0.0.1:11479")
	r.Header.Set("X-Smart-Search-Key", a.Token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func TestMoveCatalogueAndBack(t *testing.T) {
	a := testApp(t)
	settings := absPath(a.dataDir)
	for name, size := range map[string]int{"catalog.json.gz": 1000, "catalog.json.gz.previous": 400, "preferences.json": 10} {
		if err := os.WriteFile(filepath.Join(settings, name), make([]byte, size), 0600); err != nil {
			t.Fatal(err)
		}
	}
	target := t.TempDir()
	if w := postJSON(a, "/api/storage/move", `{"path":"relative/folder"}`); w.Code != 400 {
		t.Fatalf("relative path accepted: %d", w.Code)
	}
	w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(target)+`}`)
	if w.Code != 200 {
		t.Fatalf("move failed: %d %s", w.Code, w.Body)
	}
	dest := filepath.Join(target, MovedFolderName)
	for _, name := range catalogFiles {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Fatalf("%s not moved: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(settings, name)); !os.IsNotExist(err) {
			t.Fatalf("%s left behind in the settings folder", name)
		}
	}
	for _, name := range []string{"control.key", "preferences.json"} {
		if _, err := os.Stat(filepath.Join(settings, name)); err != nil {
			t.Fatalf("settings file %s must stay put: %v", name, err)
		}
	}
	s := a.Storage()
	if s.Path != dest || !s.Custom || s.Catalogue != 1000 || s.Backup != 400 || s.Other <= 0 {
		t.Fatalf("unexpected storage after move: %+v", s)
	}
	// A restart reads the recorded folder and finds the catalogue there.
	if err := os.WriteFile(filepath.Join(dest, "catalog.json.gz"), mustRead(t, "../../testdata/catalog.json"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := New(a.ctx, a.dataDir, a.BaseURL, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if b.CatalogDir() != dest || b.Restore() != nil || !b.Status().Ready {
		t.Fatalf("restart did not use the moved catalogue: %s", b.CatalogDir())
	}
	// Empty path moves it back to the default folder and removes the empty custom folder.
	if w := postJSON(a, "/api/storage/move", `{"path":""}`); w.Code != 200 {
		t.Fatalf("move back failed: %d %s", w.Code, w.Body)
	}
	if a.Storage().Custom || a.CatalogDir() != settings {
		t.Fatal("did not return to the default folder")
	}
	if _, err := os.Stat(filepath.Join(settings, locationFile)); !os.IsNotExist(err) {
		t.Fatal("location file should be removed for the default folder")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("empty custom folder should be removed")
	}
}

func TestMoveRefusedDuringBuild(t *testing.T) {
	a := testApp(t)
	a.mu.Lock()
	a.status.Building = true
	a.mu.Unlock()
	if w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(t.TempDir())+`}`); w.Code != 409 {
		t.Fatalf("move during build: %d", w.Code)
	}
}

func TestChooseFolder(t *testing.T) {
	a := testApp(t)
	a.ChooseFolder = func(start string) (string, error) {
		if start != a.CatalogDir() {
			t.Errorf("picker should start in the current folder, got %q", start)
		}
		return "/picked", nil
	}
	if w := postJSON(a, "/api/storage/choose", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"/picked"`) {
		t.Fatalf("choose: %d %s", w.Code, w.Body)
	}
	a.ChooseFolder = func(string) (string, error) { return "", nil }
	if w := postJSON(a, "/api/storage/choose", ""); !strings.Contains(w.Body.String(), `"cancelled":true`) {
		t.Fatalf("cancel: %s", w.Body)
	}
	a.ChooseFolder = func(string) (string, error) { return "", ErrPickerMissing }
	if w := postJSON(a, "/api/storage/choose", ""); w.Code != 501 {
		t.Fatalf("missing picker: %d", w.Code)
	}
	a.ChooseFolder = func(string) (string, error) { return "", errors.New("boom") }
	if w := postJSON(a, "/api/storage/choose", ""); w.Code != 500 {
		t.Fatalf("picker error: %d", w.Code)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:11479/api/storage/choose", nil)
	r.RemoteAddr = "192.168.1.4:5000"
	r.Header.Set("X-Smart-Search-Key", a.Token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("LAN client opened a picker: %d", w.Code)
	}
}

func mustJSON(v string) string { b, _ := json.Marshal(v); return string(b) }
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A destination folder another account can write may already hold a symlink
// where the copy's temporary file would go. The move must never write through
// it into the file it points at.
func TestMoveCatalogueNeverWritesThroughPlantedLinks(t *testing.T) {
	a := testApp(t)
	if err := os.WriteFile(filepath.Join(a.dataDir, catalogFile), []byte("catalogue"), 0600); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	dest := filepath.Join(target, MovedFolderName)
	if err := os.MkdirAll(dest, 0700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{catalogFile + ".moving", catalogFile, catalogFile + ".previous"} {
		if err := os.Symlink(victim, filepath.Join(dest, name)); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
	}
	if _, err := a.MoveCatalog(target); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(victim); string(data) != "keep me" {
		t.Fatalf("move wrote through a planted link: %q", data)
	}
	info, err := os.Lstat(filepath.Join(dest, catalogFile))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("catalogue is not a regular file after the move: %v %v", info, err)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, catalogFile)); string(data) != "catalogue" {
		t.Fatalf("catalogue content %q", data)
	}
	// No temporary files are left behind (the planted link is not ours).
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".moving") && e.Name() != catalogFile+".moving" {
			t.Fatalf("left %s behind", e.Name())
		}
	}
}
