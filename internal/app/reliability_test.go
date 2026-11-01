package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// Release-candidate failure-path checks: the working catalogue must survive
// every unsuccessful rebuild or move, and search must keep answering.

func savedApp(t *testing.T) (*App, []byte) {
	t.Helper()
	a := testApp(t)
	c, err := catalog.Load("../../testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	c.Profile = "compact"
	if err := catalog.Save(a.catalogPath(), c); err != nil {
		t.Fatal(err)
	}
	return a, mustRead(t, a.catalogPath())
}

func assertServing(t *testing.T, a *App) {
	t.Helper()
	w := request(a, "GET", "/catalog/series/smart-search/search=HOTD.json", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "tt11198330") {
		t.Fatalf("search stopped answering: %d %s", w.Code, w.Body)
	}
}

func assertRestartable(t *testing.T, a *App, want []byte) {
	t.Helper()
	if got, err := os.ReadFile(a.catalogPath()); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("catalogue at %s changed or vanished: %v", a.catalogPath(), err)
	}
	b, err := New(context.Background(), a.dataDir, a.BaseURL, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(); err != nil || !b.Status().Ready {
		t.Fatalf("restart lost the catalogue: %v", err)
	}
}

// Picking the folder the catalogue already lives in, but spelled differently
// (a symlink or junction here; different letter case on Windows and macOS),
// must not copy the files onto themselves and then delete them.
func TestMoveToSameFolderByAnotherNameKeepsCatalogue(t *testing.T) {
	a, before := savedApp(t)
	target := t.TempDir()
	if w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(target)+`}`); w.Code != 200 {
		t.Fatalf("first move failed: %d %s", w.Code, w.Body)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(alias)+`}`); w.Code != 200 {
		t.Fatalf("move to the same folder: %d %s", w.Code, w.Body)
	}
	assertServing(t, a)
	assertRestartable(t, a, before)
}

func TestMoveToInvalidDestinationKeepsCatalogue(t *testing.T) {
	a, before := savedApp(t)
	file := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{file, filepath.Join(file, "below")} {
		w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(target)+`}`)
		if w.Code != 500 || !strings.Contains(w.Body.String(), "still in its previous folder") {
			t.Fatalf("move into %s: %d %s", target, w.Code, w.Body)
		}
		if a.Storage().Custom || a.Storage().Moving {
			t.Fatalf("location changed after a refused move: %+v", a.Storage())
		}
	}
	if _, err := os.Stat(filepath.Join(a.dataDir, locationFile)); !os.IsNotExist(err) {
		t.Fatal("a refused move recorded a location")
	}
	assertServing(t, a)
	assertRestartable(t, a, before)
}

// A copy that fails part-way (unreadable second file) leaves the source in
// use and removes the half-copied files from the destination.
func TestFailedCopyKeepsSourceAndCleansDestination(t *testing.T) {
	a, before := savedApp(t)
	if err := os.Mkdir(filepath.Join(a.CatalogDir(), catalogFile+".previous"), 0700); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(target)+`}`); w.Code != 500 {
		t.Fatalf("broken copy reported %d %s", w.Code, w.Body)
	}
	if a.Storage().Custom {
		t.Fatal("catalogue switched to a half-copied folder")
	}
	entries, _ := os.ReadDir(filepath.Join(target, MovedFolderName))
	if len(entries) != 0 {
		t.Fatalf("half-copied files left behind: %v", entries)
	}
	assertServing(t, a)
	assertRestartable(t, a, before)
}

func TestFailedMoveCanBeRetried(t *testing.T) {
	a, before := savedApp(t)
	file := filepath.Join(t.TempDir(), "blocker")
	_ = os.WriteFile(file, nil, 0600)
	_ = postJSON(a, "/api/storage/move", `{"path":`+mustJSON(file)+`}`)
	target := t.TempDir()
	if w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(target)+`}`); w.Code != 200 {
		t.Fatalf("retry after a failed move: %d %s", w.Code, w.Body)
	}
	assertRestartable(t, a, before)
}

func TestBuildCancelledByShutdownKeepsCatalogue(t *testing.T) {
	a, before := savedApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx = ctx
	entered := make(chan struct{})
	a.DownloadCatalog = func(ctx context.Context, _ catalog.Profile, _ catalog.Progress) (*catalog.Catalog, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if w := buildRequest(a, `{"profile":"expanded"}`); w.Code != 202 {
		t.Fatal(w.Body)
	}
	<-entered
	cancel()
	waitForBuild(t, a)
	if s := a.Status(); !s.Ready || s.Profile != "compact" || s.Error == "" {
		t.Fatalf("cancelled build lost the catalogue: %+v", s)
	}
	assertServing(t, a)
	assertRestartable(t, a, before)
}

// A save that cannot finish (here the destination folder is replaced by a file
// so the temporary snapshot cannot be created) keeps the working index.
func TestSaveFailureKeepsWorkingIndex(t *testing.T) {
	a, _ := savedApp(t)
	broken := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(broken, nil, 0600)
	a.mu.Lock()
	a.catalogDir = broken // the folder cannot be written
	a.mu.Unlock()
	a.DownloadCatalog = func(context.Context, catalog.Profile, catalog.Progress) (*catalog.Catalog, error) {
		return catalog.Load("../../testdata/catalog.json")
	}
	if w := buildRequest(a, `{"profile":"standard"}`); w.Code != 202 {
		t.Fatal(w.Body)
	}
	waitForBuild(t, a)
	if s := a.Status(); !s.Ready || s.Profile != "compact" || s.Error == "" {
		t.Fatalf("save failure replaced the catalogue: %+v", s)
	}
	assertServing(t, a)
}

func TestCorruptSnapshotFallsBackToPrevious(t *testing.T) {
	a, good := savedApp(t)
	path := a.catalogPath()
	if err := os.WriteFile(path+".previous", good, 0600); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"garbage":     []byte("not a catalogue"),
		"truncated":   good[:len(good)/2],
		"empty":       nil,
		"bad-gzip":    append([]byte{0x1f, 0x8b}, bytes.Repeat([]byte{0}, 40)...),
		"wrong-shape": gzipped(t, `{"version":1,"titles":[{"id":"bad","name":"x","type":"movie"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			b, err := New(context.Background(), a.dataDir, a.BaseURL, "test")
			if err != nil {
				t.Fatal(err)
			}
			if err := b.Restore(); err != nil || !b.Status().Ready {
				t.Fatalf("previous snapshot not used: %v", err)
			}
		})
	}
	// Both unreadable: start without a catalogue instead of crashing, and a
	// rebuild replaces the corrupt file.
	_ = os.WriteFile(path, []byte("x"), 0600)
	_ = os.WriteFile(path+".previous", []byte("y"), 0600)
	b, _ := New(context.Background(), a.dataDir, a.BaseURL, "test")
	if err := b.Restore(); err == nil || b.Status().Ready {
		t.Fatal("corrupt catalogue reported as ready")
	}
	b.DownloadCatalog = func(context.Context, catalog.Profile, catalog.Progress) (*catalog.Catalog, error) {
		return catalog.Load("../../testdata/catalog.json")
	}
	if w := buildRequest(b, `{"profile":"compact"}`); w.Code != 202 {
		t.Fatal(w.Body)
	}
	waitForBuild(t, b)
	if !b.Status().Ready {
		t.Fatalf("rebuild after corruption failed: %+v", b.Status())
	}
	if _, err := catalog.Load(path); err != nil {
		t.Fatal("rebuilt snapshot unreadable:", err)
	}
}

func gzipped(t *testing.T, s string) []byte {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	_, _ = z.Write([]byte(s))
	_ = z.Close()
	return b.Bytes()
}

// Searches running while catalogues are swapped and moved must never fail or
// see an empty index. Run with -race.
func TestSearchesDuringRebuildAndMove(t *testing.T) {
	a, _ := savedApp(t)
	a.DownloadCatalog = func(context.Context, catalog.Profile, catalog.Progress) (*catalog.Catalog, error) {
		return catalog.Load("../../testdata/catalog.json")
	}
	var stop atomic.Bool
	var failures atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				w := request(a, "GET", "/catalog/movie/smart-search/search=interstelar.json", "", "")
				// A full search slot answers empty by design; anything else is a failure.
				if w.Code != 200 || (!strings.Contains(w.Body.String(), "tt0816692") && !strings.Contains(w.Body.String(), `"metas":[]`)) {
					failures.Add(1)
				}
				_ = request(a, "GET", "/api/status", "", a.Token)
				_ = request(a, "GET", "/api/storage", "", a.Token)
			}
		}()
	}
	for round := 0; round < 5; round++ {
		if w := buildRequest(a, fmt.Sprintf(`{"profile":%q}`, []string{"compact", "standard"}[round%2])); w.Code != 202 {
			t.Fatal(w.Body)
		}
		waitForBuild(t, a)
		target := ""
		if round%2 == 0 {
			target = t.TempDir()
		}
		if w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(target)+`}`); w.Code != 200 {
			t.Fatalf("move %d: %d %s", round, w.Code, w.Body)
		}
	}
	stop.Store(true)
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d searches failed during rebuilds and moves", failures.Load())
	}
	if s := a.Status(); !s.Ready || s.Error != "" {
		t.Fatalf("unexpected final status: %+v", s)
	}
}

// Disk exhaustion: the free-space check refuses a move before copying.
func TestMoveRefusedWhenDestinationIsFull(t *testing.T) {
	if os.Getenv("SSS_TINY_FS") == "" {
		t.Skip("set SSS_TINY_FS to a small mounted filesystem to run")
	}
	a, before := savedApp(t)
	big := filepath.Join(a.CatalogDir(), catalogFile+".previous")
	if err := os.WriteFile(big, make([]byte, 120<<20), 0600); err != nil {
		t.Fatal(err)
	}
	w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(os.Getenv("SSS_TINY_FS"))+`}`)
	if w.Code != 507 {
		t.Fatalf("full drive: %d %s", w.Code, w.Body)
	}
	assertRestartable(t, a, before)
}

// Disk exhaustion while a rebuild writes its snapshot: the write fails, the
// old snapshot and the serving index stay, and no temporary file is left.
func TestRebuildOnFullDiskKeepsCatalogue(t *testing.T) {
	tiny := os.Getenv("SSS_TINY_FS")
	if tiny == "" {
		t.Skip("set SSS_TINY_FS to a small mounted filesystem to run")
	}
	a, _ := savedApp(t)
	dir, err := os.MkdirTemp(tiny, "rebuild-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if w := postJSON(a, "/api/storage/move", `{"path":`+mustJSON(dir)+`}`); w.Code != 200 {
		t.Fatalf("move onto the small drive: %d %s", w.Code, w.Body)
	}
	before := mustRead(t, a.catalogPath())
	// Fill the drive, leaving well under the size of the next snapshot.
	space, _ := diskFree(dir)
	filler, _ := os.Create(filepath.Join(dir, "filler"))
	_, _ = filler.Write(make([]byte, int(space)-(512<<10)))
	filler.Close()
	defer os.Remove(filepath.Join(dir, "filler"))
	free, _ := diskFree(dir)
	if free > 2<<20 {
		t.Skipf("could not fill %s (%d bytes free)", tiny, free)
	}
	a.DownloadCatalog = func(context.Context, catalog.Profile, catalog.Progress) (*catalog.Catalog, error) {
		c := &catalog.Catalog{Version: 1, Profile: "standard"}
		for i := 0; i < 200000; i++ {
			c.Titles = append(c.Titles, catalog.Title{ID: fmt.Sprintf("tt%07d", i), Name: fmt.Sprintf("Title %x %d", i*7919, i), Type: "movie", Votes: i})
		}
		return c, nil
	}
	if w := buildRequest(a, `{"profile":"standard"}`); w.Code != 202 {
		t.Fatal(w.Body)
	}
	waitForBuild(t, a)
	if s := a.Status(); !s.Ready || s.Profile != "compact" || s.Error == "" {
		t.Fatalf("full disk replaced the catalogue: %+v", s)
	}
	assertServing(t, a)
	entries, _ := os.ReadDir(a.CatalogDir())
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temporary snapshot left on the full drive: %s", e.Name())
		}
	}
	os.Remove(filepath.Join(dir, "filler"))
	assertRestartable(t, a, before)
}
