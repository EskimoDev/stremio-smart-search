package app

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	// control.key is created by New and counts as other data.
	if s.Catalogue != 1000 || s.Backup != 400 || s.Other != 64 || s.Total != 1464 || s.Files != 3 || s.Memory == 0 || !s.CanOpen || !filepath.IsAbs(s.Path) {
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
