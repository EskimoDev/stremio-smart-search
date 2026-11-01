package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// TestRealDataAutomaticUpdate runs the whole automatic-update path on real
// IMDb files from a local folder, served over loopback HTTP with real
// validators, in an isolated data folder: first preparation, a routine
// update that carries alternative titles, a weekly update that downloads
// them, and an unchanged check. Nothing is fetched from the internet.
// Opt in with SMART_SEARCH_REAL_DATA=<folder with title.*.tsv.gz>.
func TestRealDataAutomaticUpdate(t *testing.T) {
	dir := os.Getenv("SMART_SEARCH_REAL_DATA")
	if dir == "" {
		t.Skip("set SMART_SEARCH_REAL_DATA to a folder with the three IMDb .tsv.gz files")
	}
	// Match the helper's own runtime settings (cmd/smart-search).
	runtime.GOMAXPROCS(min(2, runtime.NumCPU()))
	debug.SetMemoryLimit(256 << 20)
	var bodyBytes, gets, heads int64
	generation := "g1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"`+generation+"-"+name+`"`)
		if r.Method == http.MethodHead {
			heads++
		} else {
			gets++
			bodyBytes += info.Size()
		}
		// ServeContent handles If-None-Match, If-Modified-Since and HEAD.
		f, _ := os.Open(path)
		defer f.Close()
		http.ServeContent(w, r, name, info.ModTime(), f)
	}))
	defer server.Close()

	c := &clock{now: time.Now().UTC()}
	data := t.TempDir()
	a, err := New(context.Background(), data, "http://127.0.0.1:11479", "e2e")
	if err != nil {
		t.Fatal(err)
	}
	source := catalog.Source{BaseURL: server.URL, Client: server.Client()}
	a.DownloadCatalog, a.DownloadAliases, a.CheckSource = source.DownloadProfile, source.DownloadAliases, source.Check
	a.NetworkCost = func() string { return "unmetered" }
	a.now = c.Now
	profile, _ := catalog.ProfileByID("standard")

	step := func(name string, run func()) {
		gets, heads, bodyBytes = 0, 0, 0
		_ = os.WriteFile("/proc/self/clear_refs", []byte("5"), 0) // reset peak RSS (Linux)
		start := time.Now()
		run()
		s := waitBuildFor(t, a, 20*time.Minute)
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		t.Logf("%-28s %6.1fs  HEAD %d  GET %d  %4d MB  titles %d  aliases %d  error %q  peak RSS %s",
			name, time.Since(start).Seconds(), heads, gets, bodyBytes/1e6, s.Movies+s.Series, s.Aliases, s.Error+s.Updates.LastError, peakRSS())
	}
	findsFrench := func() bool {
		w := request(a, "GET", "/catalog/movie/smart-search/search=la%20reine%20des%20neiges.json", "", "")
		return strings.Contains(w.Body.String(), "tt2294629")
	}

	step("first preparation (fr)", func() { a.startBuild(profile, catalog.AliasOptions{Language: "fr"}) })
	if !a.Status().Ready || !findsFrench() {
		t.Fatal("first preparation failed")
	}
	aliases := a.Status().Aliases

	c.Advance(49 * time.Hour)
	step("due, files unchanged", func() { a.UpdateTick(context.Background()) })
	if gets != 0 || a.Status().Updates.CheckResult != "unchanged" {
		t.Fatalf("unchanged files downloaded: %d GETs", gets)
	}

	generation = "g2" // IMDb published again
	c.Advance(7 * time.Hour)
	step("routine update (carry)", func() { a.UpdateTick(context.Background()) })
	if gets != 2 || a.Status().Aliases != aliases || !findsFrench() {
		t.Fatalf("routine update: %d GETs, %d aliases (was %d)", gets, a.Status().Aliases, aliases)
	}

	generation = "g3"
	c.Advance(6 * 24 * time.Hour)
	step("weekly update (with akas)", func() { a.UpdateTick(context.Background()) })
	if gets != 3 || !findsFrench() {
		t.Fatalf("weekly update: %d GETs", gets)
	}
	for i := 0; i < 5; i++ { // later ticks before the next interval do nothing
		c.Advance(30 * time.Minute)
		step("tick, not due", func() { a.UpdateTick(context.Background()) })
		if heads+gets != 0 {
			t.Fatal("requests while not due")
		}
	}
}

func waitBuildFor(t *testing.T, a *App, limit time.Duration) Status {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if s := a.Status(); !s.Building {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("build did not finish")
	return Status{}
}

func peakRSS() string {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "n/a"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "VmHWM:"))
		}
	}
	return "n/a"
}
