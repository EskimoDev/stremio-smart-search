package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// Automatic-update tests use a controllable clock and a local stand-in for
// IMDb's dataset server that counts every request and body byte, so "no
// download" is measured, not assumed.

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) Advance(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }
func (c *clock) Set(t time.Time)         { c.mu.Lock(); c.now = t; c.mu.Unlock() }

type fakeIMDb struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	files    map[string][]byte
	etag     string // "" sends no ETag
	modified string // "" sends no Last-Modified
	// Fault injection.
	headStatus        int           // reply to HEAD with this status
	ignoreConditional bool          // never answer 304
	getStatus         int           // reply to GET with this status
	truncate          bool          // send half the body under the full Content-Length
	garbage           bool          // send bytes that are not gzip
	block             chan struct{} // GETs wait for this to close
	heads, gets       atomic.Int64
	bodyBytes         atomic.Int64
}

func gzipFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	_, _ = z.Write(raw)
	_ = z.Close()
	return buf.Bytes()
}

func newFakeIMDb(t *testing.T) *fakeIMDb {
	f := &fakeIMDb{t: t, etag: `"v1"`, modified: "Sat, 01 Nov 2026 03:00:00 GMT", files: map[string][]byte{
		catalog.RatingsFile: gzipFile(t, "../../testdata/aliases/title.ratings.tsv"),
		catalog.BasicsFile:  gzipFile(t, "../../testdata/aliases/title.basics.tsv"),
		catalog.AkasFile:    gzipFile(t, "../../testdata/aliases/title.akas.tsv"),
	}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeIMDb) set(change func(*fakeIMDb)) { f.mu.Lock(); change(f); f.mu.Unlock() }

func (f *fakeIMDb) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	body, ok := f.files[strings.TrimPrefix(r.URL.Path, "/")]
	etag, modified, headStatus, getStatus, truncate, garbage, block, plain := f.etag, f.modified, f.headStatus, f.getStatus, f.truncate, f.garbage, f.block, f.ignoreConditional
	f.mu.Unlock()
	if r.Method == http.MethodHead {
		f.heads.Add(1)
	} else {
		f.gets.Add(1)
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodHead && headStatus != 0 {
		w.WriteHeader(headStatus)
		return
	}
	if r.Method == http.MethodGet && getStatus != 0 {
		w.WriteHeader(getStatus)
		return
	}
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	if modified != "" {
		w.Header().Set("Last-Modified", modified)
	}
	// Same rules as S3/CloudFront: If-None-Match wins over If-Modified-Since.
	if plain {
		// A server or proxy that ignores conditional requests.
	} else if match := r.Header.Get("If-None-Match"); match != "" && etag != "" {
		if match == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	} else if since := r.Header.Get("If-Modified-Since"); since != "" && modified != "" && since == modified {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if garbage {
		body = bytes.Repeat([]byte("not gzip "), 50)
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	if block != nil {
		select {
		case <-block:
		case <-r.Context().Done():
			return
		}
	}
	if truncate {
		body = body[:len(body)/2]
	}
	n, _ := w.Write(body)
	f.bodyBytes.Add(int64(n))
	if truncate {
		// Drop the connection so the client sees a short body.
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				conn.Close()
			}
		}
	}
}

func (f *fakeIMDb) requests() (heads, gets int64) { return f.heads.Load(), f.gets.Load() }

// updateApp is a helper on dataDir wired to the fake server and clock.
func updateApp(t *testing.T, ctx context.Context, dataDir string, f *fakeIMDb, c *clock) *App {
	t.Helper()
	a, err := New(ctx, dataDir, "http://127.0.0.1:11479", "test")
	if err != nil {
		t.Fatal(err)
	}
	source := catalog.Source{BaseURL: f.server.URL, Client: f.server.Client()}
	a.DownloadCatalog = source.DownloadProfile
	a.DownloadAliases = source.DownloadAliases
	a.CheckSource = source.Check
	a.NetworkCost = func() string { return "unmetered" }
	a.now = c.Now
	return a
}

func waitBuild(t *testing.T, a *App) Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := a.Status(); !s.Building {
			return s
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("build did not finish")
	return Status{}
}

// installed returns a helper with a catalogue built from the fake server at
// the clock's current time, as after a first-run preparation.
func installed(t *testing.T, f *fakeIMDb, c *clock) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	a := updateApp(t, context.Background(), dir, f, c)
	profile, _ := catalog.ProfileByID("compact")
	if !a.startBuild(profile, catalog.AliasOptions{}) {
		t.Fatal("first build refused")
	}
	if s := waitBuild(t, a); !s.Ready || s.Error != "" {
		t.Fatalf("first build failed: %+v", s)
	}
	return a, dir
}

func assertServingFixture(t *testing.T, a *App) {
	t.Helper()
	w := request(a, "GET", "/catalog/movie/smart-search/search=frozen.json", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "tt2294629") {
		t.Fatalf("search stopped answering: %d %s", w.Code, w.Body)
	}
}

func startTime() time.Time { return time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC) }

func TestFreshCatalogueMakesNoRequests(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, _ := installed(t, f, c)
	heads, gets := f.requests()
	for _, step := range []time.Duration{time.Minute, time.Hour, 47 * time.Hour} {
		c.Set(startTime().Add(step))
		a.UpdateTick(context.Background())
	}
	if h, g := f.requests(); h != heads || g != gets {
		t.Fatalf("fresh catalogue sent %d HEAD and %d GET requests", h-heads, g-gets)
	}
}

func TestOverdueCatalogueUpdatesOnceAcrossRestarts(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	_, dir := installed(t, f, c)
	f.set(func(f *fakeIMDb) { f.etag = `"v2"` })
	c.Advance(49 * time.Hour)
	heads, gets := f.requests()
	// Ten launches over the next day: only the first one downloads.
	for launch := 0; launch < 10; launch++ {
		a := updateApp(t, context.Background(), dir, f, c)
		if err := a.Restore(); err != nil {
			t.Fatal(err)
		}
		a.UpdateTick(context.Background())
		waitBuild(t, a)
		c.Advance(2 * time.Hour)
	}
	h, g := f.requests()
	if g-gets != 2 || h-heads != 2 {
		t.Fatalf("ten launches made %d HEAD and %d GET requests; want one check (2 HEAD) and one download (2 GET)", h-heads, g-gets)
	}
	state := readState(t, dir)
	if state.Failures != 0 || state.LastSuccess.IsZero() || len(state.Sources) != 2 || state.Sources[0].ETag != `"v2"` {
		t.Fatalf("state after update: %+v", state)
	}
}

func readState(t *testing.T, dir string) updateState {
	t.Helper()
	var s updateState
	data, err := os.ReadFile(filepath.Join(dir, updateStateFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUnchangedSourceIsNotDownloaded(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, dir := installed(t, f, c)
	before := mustRead(t, a.catalogPath())
	bytesBefore := f.bodyBytes.Load()
	c.Advance(49 * time.Hour)
	_, gets := f.requests()
	a.UpdateTick(context.Background())
	if _, g := f.requests(); g != gets || f.bodyBytes.Load() != bytesBefore {
		t.Fatal("unchanged files were downloaded")
	}
	if s := a.Status().Updates; s.CheckResult != "unchanged" || s.Failures != 0 {
		t.Fatalf("status after unchanged check: %+v", s)
	}
	if !bytes.Equal(before, mustRead(t, a.catalogPath())) {
		t.Fatal("catalogue rewritten without new data")
	}
	// Re-checks are spaced, even across a restart.
	heads, _ := f.requests()
	c.Advance(5 * time.Hour)
	b := updateApp(t, context.Background(), dir, f, c)
	_ = b.Restore()
	b.UpdateTick(context.Background())
	if h, _ := f.requests(); h != heads {
		t.Fatal("re-checked sooner than six hours after an unchanged result")
	}
	c.Advance(time.Hour + time.Minute)
	b.UpdateTick(context.Background())
	if h, _ := f.requests(); h != heads+2 {
		t.Fatal("did not re-check after six hours")
	}
}

func TestServerIgnoringConditionalHeadersStillCompares(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, _ := installed(t, f, c)
	source := catalog.Source{BaseURL: f.server.URL, Client: f.server.Client()}
	known := readState(t, a.dataDir).Sources
	f.set(func(f *fakeIMDb) { f.ignoreConditional = true })
	if got, err := source.Check(context.Background(), catalog.DatasetFiles(catalog.AliasOptions{}), []catalog.SourceFile{{File: catalog.RatingsFile, ETag: `"v1"`}, {File: catalog.BasicsFile, LastModified: known[1].LastModified}}); err != nil || got != catalog.Unchanged {
		t.Fatalf("matching validators from a 200 reply: %v %v", got, err)
	}
	f.set(func(f *fakeIMDb) { f.etag = `"v2"` })
	if got, _ := source.Check(context.Background(), catalog.DatasetFiles(catalog.AliasOptions{}), known); got != catalog.Changed {
		t.Fatalf("new ETag reported %s", got)
	}
	f.set(func(f *fakeIMDb) { f.etag = ""; f.modified = "Sun, 02 Nov 2026 03:00:00 GMT" })
	if got, _ := source.Check(context.Background(), catalog.DatasetFiles(catalog.AliasOptions{}), []catalog.SourceFile{{File: catalog.RatingsFile, LastModified: known[0].LastModified}, {File: catalog.BasicsFile, LastModified: known[1].LastModified}}); got != catalog.Changed {
		t.Fatalf("new Last-Modified reported %s", got)
	}
}

// With no validators to compare, or HEAD refused, the check cannot tell; the
// time-based schedule alone decides, so it downloads once per interval and
// never more often.
func TestNoValidatorsFallsBackToInterval(t *testing.T) {
	for name, change := range map[string]func(*fakeIMDb){
		"no validators": func(f *fakeIMDb) { f.etag, f.modified = "", "" },
		"HEAD refused":  func(f *fakeIMDb) { f.headStatus = http.StatusMethodNotAllowed },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeIMDb(t)
			f.set(change)
			c := &clock{now: startTime()}
			a, _ := installed(t, f, c)
			_, gets := f.requests()
			for hour := 1; hour <= 96; hour++ {
				c.Advance(time.Hour)
				a.UpdateTick(context.Background())
				waitBuild(t, a)
			}
			if _, g := f.requests(); g-gets != 2*2 {
				t.Fatalf("96 hours at a 48 hour interval downloaded %d files; want 4 (2 updates)", g-gets)
			}
			if s := a.Status().Updates; s.Failures != 0 {
				t.Fatalf("status %+v", s)
			}
		})
	}
}

func TestOfflineStartupBacksOffAndPersists(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	_, dir := installed(t, f, c)
	f.server.Close() // offline from here on
	c.Advance(49 * time.Hour)
	a := updateApp(t, context.Background(), dir, f, c)
	if err := a.Restore(); err != nil || !a.Status().Ready {
		t.Fatalf("offline start-up must still serve: %v", err)
	}
	a.UpdateTick(context.Background())
	assertServingFixture(t, a)
	s := a.Status().Updates
	if s.Failures != 1 || s.State != "retrying" || !strings.Contains(s.LastError, "Could not reach IMDb") || !s.NextCheck.Equal(c.Now().Add(time.Hour)) {
		t.Fatalf("after offline check: %+v", s)
	}
	// A restart ten minutes later does not retry.
	c.Advance(10 * time.Minute)
	b := updateApp(t, context.Background(), dir, f, c)
	_ = b.Restore()
	b.UpdateTick(context.Background())
	if got := b.Status().Updates.Failures; got != 1 {
		t.Fatalf("restart retried during backoff: failures %d", got)
	}
}

func TestConsecutiveFailuresBackOffBounded(t *testing.T) {
	want := []time.Duration{1, 2, 4, 8, 16, 24, 24, 24}
	for i, hours := range want {
		if got := backoff(i + 1); got != hours*time.Hour {
			t.Fatalf("failure %d waits %v, want %vh", i+1, got, hours)
		}
	}
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, _ := installed(t, f, c)
	f.set(func(f *fakeIMDb) { f.etag = `"v2"`; f.getStatus = http.StatusServiceUnavailable })
	c.Advance(49 * time.Hour)
	_, gets := f.requests()
	// Tick every 30 minutes for three days.
	for i := 0; i < 144; i++ {
		a.UpdateTick(context.Background())
		waitBuild(t, a)
		c.Advance(30 * time.Minute)
	}
	// Attempts at 0,1,3,7,15,31,55 hours: 7 attempts, one GET each (ratings fails first).
	if _, g := f.requests(); g-gets != 7 {
		t.Fatalf("three days of failures made %d download attempts; want 7", g-gets)
	}
	assertServingFixture(t, a)
	if s := a.Status(); s.Error != "" || !s.Ready {
		t.Fatalf("automatic failures must not put an error on the page: %+v", s)
	}
}

func TestInterruptedAndCorruptDownloadsKeepCatalogue(t *testing.T) {
	for name, change := range map[string]func(*fakeIMDb){
		"truncated":   func(f *fakeIMDb) { f.truncate = true },
		"not gzip":    func(f *fakeIMDb) { f.garbage = true },
		"server 500":  func(f *fakeIMDb) { f.getStatus = 500 },
		"missing col": func(f *fakeIMDb) { f.files[catalog.BasicsFile] = gzipBytes([]byte("tconst\ttitleType\nTT1\tmovie\n")) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeIMDb(t)
			c := &clock{now: startTime()}
			a, _ := installed(t, f, c)
			before := mustRead(t, a.catalogPath())
			f.set(func(f *fakeIMDb) { f.etag = `"v2"` })
			f.set(change)
			c.Advance(49 * time.Hour)
			a.UpdateTick(context.Background())
			s := waitBuild(t, a)
			if !s.Ready || s.Updates.Failures != 1 {
				t.Fatalf("after %s: %+v", name, s.Updates)
			}
			assertServingFixture(t, a)
			assertRestartable(t, a, before)
		})
	}
}

func gzipBytes(raw []byte) []byte {
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	_, _ = z.Write(raw)
	_ = z.Close()
	return buf.Bytes()
}

// A valid but implausibly small replacement (here: a ratings file with one
// title) is refused by an automatic update and kept out of the snapshot.
func TestAutomaticUpdateRefusesShrunkenCatalogue(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, _ := installed(t, f, c)
	before := mustRead(t, a.catalogPath())
	f.set(func(f *fakeIMDb) {
		f.etag = `"v2"`
		f.files[catalog.RatingsFile] = gzipBytes([]byte("tconst\taverageRating\tnumVotes\ntt2294629\t7.5\t700000\n"))
	})
	c.Advance(49 * time.Hour)
	a.UpdateTick(context.Background())
	s := waitBuild(t, a)
	if s.Updates.Failures != 1 || !strings.Contains(s.Updates.LastError, "kept the current catalogue") {
		t.Fatalf("shrunken catalogue: %+v", s.Updates)
	}
	assertRestartable(t, a, before)
}

func TestCrashDuringDownloadCountsAsFailure(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	_, dir := installed(t, f, c)
	state := readState(t, dir)
	state.Attempt = c.Now()
	if err := writeJSONFile(filepath.Join(dir, updateStateFile), state); err != nil {
		t.Fatal(err)
	}
	c.Advance(49 * time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := updateApp(t, ctx, dir, f, c)
	_ = a.Restore()
	a.StartUpdates(ctx)
	s := a.Status().Updates
	if s.Failures != 1 || !strings.Contains(s.LastError, "interrupted") {
		t.Fatalf("crash not recorded: %+v", s)
	}
	// Backoff counts from the interrupted attempt, which is long past.
	if !s.NextCheck.Equal(c.Now()) && s.NextCheck.After(c.Now()) {
		t.Fatalf("an old interrupted attempt should not delay a due update: next %v now %v", s.NextCheck, c.Now())
	}
}

func TestShutdownDuringUpdateStopsCleanly(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	_, dir := installed(t, f, c)
	block := make(chan struct{})
	defer close(block)
	f.set(func(f *fakeIMDb) { f.etag = `"v2"`; f.block = block })
	c.Advance(49 * time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	a := updateApp(t, ctx, dir, f, c)
	if err := a.Restore(); err != nil {
		t.Fatal(err)
	}
	before := mustRead(t, a.catalogPath())
	a.UpdateTick(ctx)
	if !a.Status().Building {
		t.Fatal("update did not start")
	}
	cancel() // Quit or OS shutdown
	s := waitBuild(t, a)
	if !s.Ready || s.Updates.Failures != 1 || !strings.Contains(s.Updates.LastError, "closed") {
		t.Fatalf("after shutdown: %+v", s.Updates)
	}
	if st := readState(t, dir); !st.Attempt.IsZero() {
		t.Fatal("clean shutdown left an attempt marker")
	}
	assertRestartable(t, a, before)
}

func TestClockChanges(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, dir := installed(t, f, c)
	_, gets := f.requests()
	// Set back a year: the stored update is "in the future". The schedule
	// restarts from now instead of waiting a year.
	c.Set(startTime().AddDate(-1, 0, 0))
	b := updateApp(t, context.Background(), dir, f, c)
	_ = b.Restore()
	if next := b.Status().Updates.NextCheck; !next.Equal(c.Now().Add(48 * time.Hour)) {
		t.Fatalf("after clock set back, next check %v; want %v", next, c.Now().Add(48*time.Hour))
	}
	b.UpdateTick(context.Background())
	if _, g := f.requests(); g != gets {
		t.Fatal("clock set back triggered a download")
	}
	// A retry time far ahead (clock was ahead when it failed) is capped.
	b.mu.Lock()
	b.updates.state.Failures, b.updates.state.RetryAfter = 1, c.Now().AddDate(1, 0, 0)
	next := b.nextUpdateLocked(c.Now())
	b.mu.Unlock()
	if next.After(c.Now().Add(backoffMax + 48*time.Hour)) {
		t.Fatalf("future retry not capped: %v", next)
	}
	// Set forward a year: one update, not one per missed interval.
	b.mu.Lock()
	b.updates.state.Failures, b.updates.state.RetryAfter = 0, time.Time{}
	b.mu.Unlock()
	f.set(func(f *fakeIMDb) { f.etag = `"v2"` })
	c.Set(startTime().AddDate(1, 0, 0))
	for i := 0; i < 5; i++ {
		b.UpdateTick(context.Background())
		waitBuild(t, b)
		c.Advance(30 * time.Minute)
	}
	if _, g := f.requests(); g-gets != 2 {
		t.Fatalf("clock set forward made %d GETs; want one update", g-gets)
	}
	_ = a
}

func TestManualRefreshDuringAutomaticUpdate(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, _ := installed(t, f, c)
	block := make(chan struct{})
	f.set(func(f *fakeIMDb) { f.etag = `"v2"`; f.block = block })
	c.Advance(49 * time.Hour)
	_, gets := f.requests()
	a.UpdateTick(context.Background())
	if s := a.Status(); !s.Building || !s.Updates.Automatic {
		t.Fatalf("automatic update not running: %+v", s.Updates)
	}
	// Same selection: the click joins the running update.
	if w := postJSON(a, "/api/build", `{"profile":"compact"}`); w.Code != 202 {
		t.Fatalf("Refresh now during an automatic update: %d %s", w.Code, w.Body)
	}
	if a.Status().Updates.Automatic {
		t.Fatal("joined update still marked automatic")
	}
	// A different size still waits its turn.
	if w := postJSON(a, "/api/build", `{"profile":"expanded"}`); w.Code != 409 {
		t.Fatalf("different size during an update: %d", w.Code)
	}
	close(block)
	waitBuild(t, a)
	if _, g := f.requests(); g != gets+2 { // ratings and basics once each
		t.Fatalf("a second download started: %d extra GETs", g-gets)
	}
	// And a manual refresh resets the schedule like an automatic one.
	if s := a.Status().Updates; !s.LastSuccess.Equal(c.Now()) || s.Failures != 0 {
		t.Fatalf("manual refresh not recorded: %+v", s)
	}
}

func TestMeteredConnectionSkipsAutomaticUpdates(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, _ := installed(t, f, c)
	a.NetworkCost = func() string { return "metered" }
	f.set(func(f *fakeIMDb) { f.etag = `"v2"` })
	c.Advance(49 * time.Hour)
	heads, gets := f.requests()
	a.UpdateTick(context.Background())
	if h, g := f.requests(); h != heads || g != gets {
		t.Fatal("metered connection made requests")
	}
	if s := a.Status().Updates; s.State != "metered" || s.Failures != 0 {
		t.Fatalf("metered status: %+v", s)
	}
	a.NetworkCost = func() string { return "unmetered" }
	a.UpdateTick(context.Background())
	waitBuild(t, a)
	if _, g := f.requests(); g != gets+2 {
		t.Fatal("did not update once unmetered")
	}
}

func TestSearchDuringAutomaticSwap(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, _ := installed(t, f, c)
	f.set(func(f *fakeIMDb) { f.etag = `"v2"` })
	c.Advance(49 * time.Hour)
	stop := make(chan struct{})
	var failures atomic.Int64
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				w := request(a, "GET", "/catalog/movie/smart-search/search=frozen.json", "", "")
				// A busy slot answers with an empty list; anything else
				// must still find the title.
				if w.Code != 200 || (!strings.Contains(w.Body.String(), "tt2294629") && !strings.Contains(w.Body.String(), `"metas":[]`)) {
					failures.Add(1)
				}
			}
		}()
	}
	for i := 0; i < 3; i++ {
		a.UpdateTick(context.Background())
		waitBuild(t, a)
		f.set(func(f *fakeIMDb) { f.etag = fmt.Sprintf(`"v%d"`, i+3) })
		c.Advance(49 * time.Hour)
	}
	close(stop)
	group.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d searches failed during updates", failures.Load())
	}
}

func TestUpdateSettingsMigration(t *testing.T) {
	// New installation: on by default, every 48 hours, written down.
	fresh := t.TempDir()
	a, err := New(context.Background(), fresh, "http://127.0.0.1:11479", "test")
	if err != nil {
		t.Fatal(err)
	}
	if s := a.Status().Updates; !s.Auto || s.IntervalHours != 48 || s.State != "waiting" {
		t.Fatalf("new installation: %+v", s)
	}
	if _, err := os.Stat(filepath.Join(fresh, updateSettingsFile)); err != nil {
		t.Fatal("settings not written:", err)
	}
	// Existing installation (a catalogue, no settings file): stays manual.
	existing := t.TempDir()
	if err := os.WriteFile(filepath.Join(existing, catalogFile), gzipBytes([]byte(`{}`)), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := New(context.Background(), existing, "http://127.0.0.1:11479", "test")
	if s := b.Status().Updates; s.Auto || s.State != "off" {
		t.Fatalf("existing installation turned on without asking: %+v", s)
	}
	// The decision sticks after the catalogue changes.
	_ = os.Remove(filepath.Join(existing, catalogFile))
	b, _ = New(context.Background(), existing, "http://127.0.0.1:11479", "test")
	if b.Status().Updates.Auto {
		t.Fatal("setting flipped on a later start")
	}
	// A hand-edited invalid interval falls back to the default.
	if err := os.WriteFile(filepath.Join(existing, updateSettingsFile), []byte(`{"auto":true,"intervalHours":5}`), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ = New(context.Background(), existing, "http://127.0.0.1:11479", "test")
	if s := b.Status().Updates; !s.Auto || s.IntervalHours != 48 {
		t.Fatalf("invalid interval: %+v", s)
	}
	// Garbage state is ignored, not fatal.
	if err := os.WriteFile(filepath.Join(existing, updateStateFile), []byte(`{nope`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(context.Background(), existing, "http://127.0.0.1:11479", "test"); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateSettingsAPI(t *testing.T) {
	a := testApp(t)
	for body, code := range map[string]int{
		`{"auto":false}`:        200,
		`{"intervalHours":168}`: 200,
		`{"intervalHours":5}`:   400,
		`{"auto":"yes"}`:        400,
		`{"other":1}`:           400,
	} {
		if w := postJSON(a, "/api/updates", body); w.Code != code {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	s := a.Status().Updates
	if s.Auto || s.IntervalHours != 168 {
		t.Fatalf("settings not applied: %+v", s)
	}
	data, _ := os.ReadFile(filepath.Join(a.dataDir, updateSettingsFile))
	var saved UpdateSettings
	if json.Unmarshal(data, &saved) != nil || saved.Auto || saved.IntervalHours != 168 {
		t.Fatalf("settings not saved: %s", data)
	}
	if w := request(a, "POST", "/api/updates", "", ""); w.Code != 403 {
		t.Fatal("update settings accepted without the control key")
	}
}

func TestAutomaticUpdatesOffMakesNoRequests(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a, _ := installed(t, f, c)
	off := false
	if err := a.SetUpdates(&off, nil); err != nil {
		t.Fatal(err)
	}
	heads, gets := f.requests()
	c.Advance(30 * 24 * time.Hour)
	a.UpdateTick(context.Background())
	if h, g := f.requests(); h != heads || g != gets {
		t.Fatal("updates switched off still made requests")
	}
}

func TestAliasesFileIsCheckedWhenSelected(t *testing.T) {
	f := newFakeIMDb(t)
	c := &clock{now: startTime()}
	a := updateApp(t, context.Background(), t.TempDir(), f, c)
	profile, _ := catalog.ProfileByID("compact")
	a.startBuild(profile, catalog.AliasOptions{Language: "fr"})
	if s := waitBuild(t, a); s.Language != "fr" || s.Error != "" {
		t.Fatalf("alias build: %+v", s)
	}
	if got := len(readState(t, a.dataDir).Sources); got != 3 {
		t.Fatalf("recorded %d source files; want ratings, basics and akas", got)
	}
	if b := a.Status().Updates.DownloadBytes; b != int64(len(f.files[catalog.RatingsFile])+len(f.files[catalog.BasicsFile])+len(f.files[catalog.AkasFile])) {
		t.Fatalf("estimate %d does not use recorded sizes", b)
	}
	heads, _ := f.requests()
	c.Advance(49 * time.Hour)
	a.UpdateTick(context.Background())
	if h, _ := f.requests(); h != heads+3 {
		t.Fatalf("checked %d files; want 3", h-heads)
	}
}

// BenchmarkUpdateTickNotDue is the cost of the half-hourly wake-up when no
// update is due, which is almost every wake-up.
func BenchmarkUpdateTickNotDue(b *testing.B) {
	a, err := New(context.Background(), b.TempDir(), "http://127.0.0.1:11479", "test")
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Load("../../testdata/catalog.json"); err != nil {
		b.Fatal(err)
	}
	a.mu.Lock()
	a.updates.state.LastSuccess = time.Now()
	a.mu.Unlock()
	b.ReportAllocs()
	for b.Loop() {
		a.UpdateTick(context.Background())
	}
}

// BenchmarkLoadUpdates is the start-up cost: two small JSON files.
func BenchmarkLoadUpdates(b *testing.B) {
	dir := b.TempDir()
	a, err := New(context.Background(), dir, "http://127.0.0.1:11479", "test")
	if err != nil {
		b.Fatal(err)
	}
	a.mu.Lock()
	a.updates.state = updateState{LastSuccess: time.Now(), Sources: []catalog.SourceFile{{File: catalog.RatingsFile, ETag: `"abc"`, Size: 1}, {File: catalog.BasicsFile, ETag: `"def"`, Size: 1}}}
	a.saveUpdateStateLocked()
	a.mu.Unlock()
	b.ReportAllocs()
	for b.Loop() {
		a.loadUpdates()
	}
}
