package app

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func installRequest(a *App, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://127.0.0.1:11479/api/install", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:32100"
	r.Header.Set("X-Smart-Search-Key", a.Token)
	r.Header.Set("Origin", "http://127.0.0.1:11479")
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func TestWebInstallerPreservesLocalHTTPAndPort(t *testing.T) {
	a := testApp(t)
	u, err := url.Parse(a.Status().WebInstall)
	if err != nil {
		t.Fatal(err)
	}
	_, query, ok := strings.Cut(u.Fragment, "?")
	if !ok {
		t.Fatal("missing install route")
	}
	parameters, err := url.ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	if got := parameters.Get("addon"); got != "http://127.0.0.1:11479/manifest.json" {
		t.Fatalf("local transport was changed: %s", got)
	}
}

func TestInstallTargetsAndLauncherPreference(t *testing.T) {
	for _, id := range []string{"stremio", "dxbi", "enhanced"} {
		t.Run(id, func(t *testing.T) {
			a := testApp(t)
			launched := ""
			a.LaunchClient = func(client string) error { launched = client; return nil }
			w := installRequest(a, `{"client":"`+id+`"}`)
			if w.Code != 200 || launched != id {
				t.Fatalf("wrong app launch: %d %s", w.Code, launched)
			}
			restored, err := New(context.Background(), a.dataDir, a.BaseURL, "0.1.0")
			if err != nil || restored.Status().PreferredClient != id {
				t.Fatal("chosen app was not remembered")
			}
			w = request(a, "POST", "/api/launch", "", a.Token)
			if w.Code != 200 || launched != id {
				t.Fatal("launcher ignored chosen app")
			}
		})
	}
}

func TestInstallCannotLaunchArbitraryCommand(t *testing.T) {
	a := testApp(t)
	called := false
	a.LaunchClient = func(string) error { called = true; return nil }
	for _, body := range []string{`{"client":"cmd.exe"}`, `{"client":"dxbi","path":"untrusted.exe"}`, `invalid`} {
		if w := installRequest(a, body); w.Code != 400 {
			t.Fatalf("unsafe launch accepted: %d", w.Code)
		}
	}
	if called {
		t.Fatal("invalid choice reached launcher")
	}
	a.LaunchClient = func(string) error { return ErrClientNotFound }
	if w := installRequest(a, `{"client":"enhanced"}`); w.Code != 404 {
		t.Fatal("missing app should be reported")
	}
	if a.Status().PreferredClient != "system" {
		t.Fatal("missing app should not become preferred")
	}
	a.LaunchClient = func(string) error { return errors.New("failed") }
	if w := installRequest(a, `{"client":"dxbi"}`); w.Code != 500 {
		t.Fatal("launch error was hidden")
	}
}

func TestWindowsClientPathsKeepAppsSeparate(t *testing.T) {
	for _, tc := range []struct{ id, folder, file string }{{"stremio", "Stremio", "stremio-shell-ng.exe"}, {"dxbi", "dxbi", "dx.bi.exe"}, {"enhanced", "stremio-enhanced", "Stremio Enhanced.exe"}} {
		paths := clientPaths("windows", tc.id, "", "Local Data", "Program Files", "")
		want := filepath.Join("Local Data", "Programs", tc.folder, tc.file)
		if len(paths) == 0 || paths[0] != want {
			t.Fatalf("%s discovery: %+v", tc.id, paths)
		}
	}
}
