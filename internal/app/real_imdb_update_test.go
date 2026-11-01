package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// countingTransport counts requests and response body bytes, and refuses
// any file not in allowed, so the test can never fetch title.akas.
type countingTransport struct {
	allowed map[string]bool
	mu      sync.Mutex
	log     []string
	bytes   atomic.Int64
}

type countedBody struct {
	io.ReadCloser
	n *atomic.Int64
}

func (b countedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n.Add(int64(n))
	return n, err
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	file := filepath.Base(r.URL.Path)
	if !c.allowed[file] {
		return nil, fmt.Errorf("test refused to request %s", file)
	}
	response, err := http.DefaultTransport.RoundTrip(r)
	c.mu.Lock()
	if err == nil {
		c.log = append(c.log, fmt.Sprintf("%s %s -> %d (ETag %s, Last-Modified %s, %d bytes)", r.Method, file, response.StatusCode, response.Header.Get("ETag"), response.Header.Get("Last-Modified"), response.ContentLength))
		response.Body = countedBody{response.Body, &c.bytes}
	} else {
		c.log = append(c.log, fmt.Sprintf("%s %s -> error %v", r.Method, file, err))
	}
	c.mu.Unlock()
	return response, err
}

func (c *countingTransport) take() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.log
	c.log = nil
	return out
}

// TestRealIMDbAutomaticUpdate runs one real automatic update against IMDb's
// server in an isolated temporary folder (removed afterwards): freshness
// check, ratings and titles download (about 238 MB), build, validation,
// activation, persisted state, restart and the following quiet period. It
// never requests title.akas. Opt in with SMART_SEARCH_REAL_IMDB_UPDATE=1.
//
// The test catalogue is the small repository fixture, recorded as built
// three days ago from IMDb files dated 29 October; the current IMDb files are
// genuinely newer, so the schedule's own check decides whether to download.
func TestRealIMDbAutomaticUpdate(t *testing.T) {
	if os.Getenv("SMART_SEARCH_REAL_IMDB_UPDATE") != "1" {
		t.Skip("set SMART_SEARCH_REAL_IMDB_UPDATE=1 to download about 238 MB from IMDb")
	}
	runtime.GOMAXPROCS(min(2, runtime.NumCPU())) // as cmd/smart-search
	debug.SetMemoryLimit(256 << 20)

	dir := t.TempDir()
	t.Logf("isolated data folder: %s", dir)
	transport := &countingTransport{allowed: map[string]bool{catalog.RatingsFile: true, catalog.BasicsFile: true}}
	source := catalog.Source{BaseURL: catalog.IMDb.BaseURL, Client: &http.Client{Transport: transport, Timeout: 30 * time.Minute}}
	c := &clock{now: time.Now().UTC()}
	open := func() *App {
		a, err := New(context.Background(), dir, "http://127.0.0.1:11479", "e2e")
		if err != nil {
			t.Fatal(err)
		}
		a.DownloadCatalog, a.DownloadAliases, a.CheckSource = source.DownloadProfile, source.DownloadAliases, source.Check
		a.now = c.Now
		if err := a.Restore(); err != nil {
			t.Fatal(err)
		}
		return a
	}

	// Seed: fixture catalogue, automatic updates on, last update 3 days ago.
	seed, err := catalog.Load("../../testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	seed.Profile = "compact"
	seed.BuiltAt = c.Now().Add(-72 * time.Hour)
	if err := catalog.Save(filepath.Join(dir, catalogFile), seed); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(dir, updateSettingsFile), UpdateSettings{Auto: true, IntervalHours: 48}); err != nil {
		t.Fatal(err)
	}
	old := "Wed, 29 Oct 2026 00:45:00 GMT"
	if err := writeJSONFile(filepath.Join(dir, updateStateFile), updateState{LastSuccess: seed.BuiltAt, Sources: []catalog.SourceFile{
		{File: catalog.RatingsFile, ETag: `"seed-ratings"`, LastModified: old}, {File: catalog.BasicsFile, ETag: `"seed-basics"`, LastModified: old}}}); err != nil {
		t.Fatal(err)
	}
	a := open()
	a.NetworkCost = networkCost
	before := a.Status()
	t.Logf("before: ready %v, %d titles, profile %s, next check %v (now %v), network %s", before.Ready, before.Movies+before.Series, before.Profile, before.Updates.NextCheck.Format(time.RFC3339), c.Now().Format(time.RFC3339), networkCost())

	// Search keeps answering throughout.
	var searches, misses atomic.Int64
	stop := make(chan struct{})
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
			w := request(a, "GET", "/catalog/series/smart-search/search=house%20of%20the%20dragon.json", "", "")
			searches.Add(1)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "tt11198330") {
				misses.Add(1)
			}
		}
	}()

	start := time.Now()
	a.UpdateTick(context.Background())
	checked := time.Since(start)
	for _, line := range transport.take() {
		t.Logf("check: %s", line)
	}
	s := a.Status()
	t.Logf("check result %q after %v; building %v", s.Updates.CheckResult, checked.Round(time.Millisecond), s.Building)
	if !s.Building {
		close(stop)
		group.Wait()
		t.Logf("no download was needed or started: state %s, error %q", s.Updates.State, s.Updates.LastError)
		if s.Updates.CheckResult == string(catalog.Unchanged) {
			t.Skip("IMDb reported the files unchanged; nothing was downloaded")
		}
		t.Fatalf("update did not start: %+v", s.Updates)
	}
	s = waitBuildFor(t, a, 40*time.Minute)
	elapsed := time.Since(start)
	close(stop)
	group.Wait()
	for _, line := range transport.take() {
		t.Logf("download: %s", line)
	}
	t.Logf("update finished in %v; %d body bytes received; %d searches during it, %d missed", elapsed.Round(time.Millisecond), transport.bytes.Load(), searches.Load(), misses.Load())
	if s.Error != "" || s.Updates.LastError != "" || s.Updates.Failures != 0 {
		t.Fatalf("update failed: %q %q", s.Error, s.Updates.LastError)
	}
	if misses.Load() != 0 {
		t.Fatalf("%d searches missed during the update", misses.Load())
	}
	t.Logf("after: %d movies + %d series, profile %s, built %v", s.Movies, s.Series, s.Profile, s.BuiltAt.Format(time.RFC3339))

	// Integrity: the saved snapshot reloads, validates and matches.
	saved, err := catalog.Load(filepath.Join(dir, catalogFile))
	if err != nil {
		t.Fatalf("saved catalogue does not load: %v", err)
	}
	if len(saved.Titles) != s.Movies+s.Series || saved.Profile != "compact" || saved.AliasLanguage != "" {
		t.Fatalf("saved catalogue mismatch: %d titles, profile %q", len(saved.Titles), saved.Profile)
	}
	if _, err := os.Stat(filepath.Join(dir, catalogFile+".previous")); !os.IsNotExist(err) {
		t.Fatal("recovery copy left behind after success")
	}
	w := request(a, "GET", "/catalog/movie/smart-search/search=interstelar.json", "", "")
	if !strings.Contains(w.Body.String(), "tt0816692") {
		t.Fatalf("new catalogue does not find Interstellar: %s", w.Body)
	}

	// Freshness: what was downloaded is what IMDb serves now.
	state := readState(t, dir)
	t.Logf("state: lastSuccess %v, failures %d, attempt %v, sources %+v", state.LastSuccess.Format(time.RFC3339), state.Failures, state.Attempt, state.Sources)
	if state.Failures != 0 || !state.Attempt.IsZero() || len(state.Sources) != 2 {
		t.Fatalf("state not persisted correctly: %+v", state)
	}
	for _, f := range state.Sources {
		now, err := source.Head(context.Background(), f.File)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("freshness %s: downloaded ETag %s / %s, server now %s / %s", f.File, f.ETag, f.LastModified, now.ETag, now.LastModified)
		if catalog.Older(now, f) {
			t.Fatalf("downloaded %s is older than what the server now has", f.File)
		}
	}
	transport.take()

	// Restart: same folder, loads the new catalogue, not due, no requests.
	b := open()
	b.NetworkCost = func() string { return "unmetered" }
	if r := b.Status(); !r.Ready || r.Movies != s.Movies || r.Series != s.Series {
		t.Fatalf("restart: %+v", r)
	}
	bytesBefore := transport.bytes.Load()
	for i := 0; i < 94; i++ { // 47 hours of half-hourly ticks
		c.Advance(30 * time.Minute)
		b.UpdateTick(context.Background())
	}
	if lines := transport.take(); len(lines) != 0 {
		t.Fatalf("requests within the interval after a restart: %v", lines)
	}
	// Past the interval: one check. Unless IMDb published meanwhile, it is
	// unchanged and nothing is downloaded; either way at most one update.
	c.Advance(2 * time.Hour)
	b.UpdateTick(context.Background())
	if r := b.Status(); r.Building {
		t.Log("IMDb published new files during the test; letting that one update finish")
		waitBuildFor(t, b, 40*time.Minute)
	}
	lines := transport.take()
	for _, line := range lines {
		t.Logf("next check: %s", line)
	}
	t.Logf("next check result %q, extra body bytes %d", b.Status().Updates.CheckResult, transport.bytes.Load()-bytesBefore)
	for i := 0; i < 10; i++ {
		c.Advance(30 * time.Minute)
		b.UpdateTick(context.Background())
	}
	if more := transport.take(); len(more) != 0 {
		t.Fatalf("further requests after the check: %v", more)
	}
}
