package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutostartLinuxEntryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Autostart{Exe: "/opt/Smart Search/stremio-smart-search", Args: []string{"--data-dir=/home/me/50% $data"}, goos: "linux", dir: dir}
	if s.Status().Enabled {
		t.Fatal("off until the person turns it on")
	}
	if err := s.Set(true); err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(dir, "stremio-smart-search.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	want := `Exec="/opt/Smart Search/stremio-smart-search" "--no-browser" "--launch=false" "--data-dir=/home/me/50%% \\$data"`
	if !strings.Contains(string(entry), want+"\n") || !strings.Contains(string(entry), "NoDisplay=true") {
		t.Fatalf("desktop entry:\n%s\nwant line %s", entry, want)
	}
	if !s.Status().Enabled {
		t.Fatal("status should report the entry")
	}
	if err := s.Set(false); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(false); err != nil {
		t.Fatal("turning off twice must not fail:", err)
	}
	if s.Status().Enabled {
		t.Fatal("entry should be gone")
	}
}

func TestAutostartMacLaunchAgent(t *testing.T) {
	dir := t.TempDir()
	s := &Autostart{Exe: "/Applications/Stremio + Smart Search.app/Contents/MacOS/Stremio Smart Search Launcher", goos: "darwin", dir: dir}
	if err := s.Set(true); err != nil {
		t.Fatal(err)
	}
	plist, err := os.ReadFile(filepath.Join(dir, "community.eskimodev.smartsearch.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<string>/Applications/Stremio + Smart Search.app/Contents/MacOS/Stremio Smart Search Launcher</string>", "<string>--launch=false</string>", "<key>RunAtLoad</key>\n\t<true/>"} {
		if !strings.Contains(string(plist), want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
	if !strings.Contains(launchAgent([]string{`a&b<"c">`}), "<string>a&amp;b&lt;&quot;c&quot;&gt;</string>") {
		t.Fatal("arguments must be XML-escaped")
	}
}

func TestWindowsCommandLineQuoting(t *testing.T) {
	got := windowsCommandLine([]string{`C:\Users\me\AppData\Local\Programs\Stremio Smart Search\StremioSmartSearch.exe`, "--no-browser", `--data-dir=D:\My Data\`, `say "hi"`})
	want := `"C:\Users\me\AppData\Local\Programs\Stremio Smart Search\StremioSmartSearch.exe" --no-browser "--data-dir=D:\My Data\\" "say \"hi\""`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestAutostartAPI(t *testing.T) {
	a, err := New(context.Background(), t.TempDir(), "http://127.0.0.1:11479", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, body string) (int, AutostartStatus) {
		r := httptest.NewRequest(method, "http://127.0.0.1:11479/api/autostart", strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:32100"
		r.Header.Set("X-Smart-Search-Key", a.Token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		var status AutostartStatus
		_ = json.Unmarshal(w.Body.Bytes(), &status)
		return w.Code, status
	}
	if code, status := call("GET", ""); code != 200 || status.Supported {
		t.Fatalf("no autostart configured: %d %+v", code, status)
	}
	if code, _ := call("POST", `{"enabled":true}`); code != 501 {
		t.Fatalf("unsupported systems refuse: %d", code)
	}
	a.Autostart = &Autostart{Exe: "/usr/bin/stremio-smart-search", goos: "linux", dir: t.TempDir()}
	if code, status := call("GET", ""); code != 200 || !status.Supported || status.Enabled {
		t.Fatalf("starts off: %d %+v", code, status)
	}
	if code, _ := call("POST", `{}`); code != 400 {
		t.Fatalf("a missing choice is rejected: %d", code)
	}
	if code, status := call("POST", `{"enabled":true}`); code != 200 || !status.Enabled || status.Path != "/usr/bin/stremio-smart-search" {
		t.Fatalf("turn on: %d %+v", code, status)
	}
	if code, status := call("POST", `{"enabled":false}`); code != 200 || status.Enabled {
		t.Fatalf("turn off: %d %+v", code, status)
	}
	r := httptest.NewRequest("GET", "http://192.168.1.20:11479/api/autostart", nil)
	r.RemoteAddr = "192.168.1.30:40000"
	r.Header.Set("X-Smart-Search-Key", a.Token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("LAN clients cannot change sign-in settings: %d", w.Code)
	}
}

func TestSearchExplainsMissingCatalogue(t *testing.T) {
	a, err := New(context.Background(), t.TempDir(), "http://127.0.0.1:11479", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	w := request(a, "GET", "/catalog/movie/smart-search/search=dune.json", "https://web.stremio.com", "")
	var body struct {
		Metas []struct{ ID, Type, Name, Description, Poster string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body.Metas) != 1 || body.Metas[0].ID != SetupPlaceholderID || body.Metas[0].Type != "movie" || !strings.Contains(body.Metas[0].Description, "Prepare catalogue") || body.Metas[0].Poster != "http://127.0.0.1:11479/addon-logo.png" {
		t.Fatalf("setup placeholder: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("the placeholder must not be cached past setup")
	}
	if err := a.Load("../../testdata/catalog.json"); err != nil {
		t.Fatal(err)
	}
	w = request(a, "GET", "/catalog/movie/smart-search/search=interstelar.json", "https://web.stremio.com", "")
	if strings.Contains(w.Body.String(), SetupPlaceholderID) {
		t.Fatal("no placeholder once a catalogue is loaded")
	}
}

func TestStartupErrorSentence(t *testing.T) {
	if got := sentence("another program is using port 11479"); got != "Another program is using port 11479." {
		t.Fatal(got)
	}
	if got := sentence(" done. "); got != "Done." {
		t.Fatal(got)
	}
}
