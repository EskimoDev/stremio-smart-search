package app

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// standbyApp is a saved catalogue in "only while Stremio is open" mode with a
// clock the test moves.
func standbyApp(t *testing.T) (*App, *time.Time) {
	t.Helper()
	a, _ := savedApp(t)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	a.standby.seen = now
	if err := a.SetRunMode(RunStremio); err != nil {
		t.Fatal(err)
	}
	return a, &now
}

func TestRunModeDefaultsToAlwaysAndPersists(t *testing.T) {
	a := testApp(t)
	if a.Status().RunMode != RunAlways {
		t.Fatalf("default run mode %q", a.Status().RunMode)
	}
	a.StandbyTick(false)
	a.now = func() time.Time { return time.Now().Add(time.Hour) }
	a.StandbyTick(false)
	if a.Status().Standby || a.engine.Load() == nil {
		t.Fatal("always mode must never drop the index")
	}
	if err := a.SetRunMode("sometimes"); err == nil {
		t.Fatal("unknown run mode accepted")
	}
	if err := a.SetRunMode(RunStremio); err != nil {
		t.Fatal(err)
	}
	if loadRunMode(a.dataDir) != RunStremio {
		t.Fatal("run mode not saved")
	}
}

func TestStandbyDropsIndexAfterStremioClosesAndWakesWhenItOpens(t *testing.T) {
	a, now := standbyApp(t)
	a.StandbyTick(true)
	*now = now.Add(time.Minute)
	a.StandbyTick(false)
	if a.Status().Standby {
		t.Fatal("dropped the index before the grace period")
	}
	*now = now.Add(standbyAfterClose)
	a.StandbyTick(false)
	status := a.Status()
	if !status.Standby || a.engine.Load() != nil {
		t.Fatal("index kept after Stremio closed")
	}
	if !status.Ready || status.Movies == 0 {
		t.Fatal("standby must keep the catalogue's status for the settings page")
	}
	a.StandbyTick(true)
	if a.Status().Standby || a.engine.Load() == nil {
		t.Fatal("index not loaded when Stremio opened")
	}
	assertServing(t, a)
}

func TestSearchInStandbyLoadsIndexAndAnswers(t *testing.T) {
	a, now := standbyApp(t)
	*now = now.Add(standbyAfterClose)
	a.StandbyTick(false)
	if !a.Status().Standby {
		t.Fatal("not in standby")
	}
	// Several searches at once share one load and all get results.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ { // Stremio asks for movies and series together
		wg.Add(1)
		go func() { defer wg.Done(); assertServing(t, a) }()
	}
	wg.Wait()
	if a.Status().Standby {
		t.Fatal("still in standby after a search")
	}
	// A browser search with no Stremio app open keeps it loaded for a while.
	*now = now.Add(standbyAfterClose)
	a.StandbyTick(false)
	if a.Status().Standby {
		t.Fatal("dropped the index straight after a browser search")
	}
	*now = now.Add(standbyAfterSearch)
	a.StandbyTick(false)
	if !a.Status().Standby {
		t.Fatal("index kept long after the last browser search")
	}
}

func TestStandbyLeavesBuildsAlone(t *testing.T) {
	a, now := standbyApp(t)
	a.mu.Lock()
	a.status.Building = true
	a.mu.Unlock()
	*now = now.Add(time.Hour)
	a.StandbyTick(false)
	if a.Status().Standby {
		t.Fatal("dropped the index during a catalogue build")
	}
	a.mu.Lock()
	a.status.Building = false
	a.mu.Unlock()
	a.StandbyTick(false)
	if !a.Status().Standby {
		t.Fatal("not in standby")
	}
	// Waking during a build must not end the build's status.
	a.mu.Lock()
	a.status.Building = true
	a.mu.Unlock()
	a.Wake()
	if !a.Status().Building || a.Status().Standby {
		t.Fatal("waking changed the build status or stayed asleep")
	}
}

func TestSwitchingBackToAlwaysLoadsIndex(t *testing.T) {
	a, now := standbyApp(t)
	*now = now.Add(time.Hour)
	a.StandbyTick(false)
	if !a.Status().Standby {
		t.Fatal("not in standby")
	}
	w := postJSON(a, "/api/run-mode", `{"mode":"always"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"runMode":"always"`) {
		t.Fatalf("run-mode: %d %s", w.Code, w.Body)
	}
	if a.Status().Standby || a.engine.Load() == nil {
		t.Fatal("always mode left the index dropped")
	}
	if w := postJSON(a, "/api/run-mode", `{"mode":"never"}`); w.Code != 400 {
		t.Fatalf("invalid mode: %d", w.Code)
	}
}

func TestClientProcessNames(t *testing.T) {
	for _, name := range []string{"stremio.exe", "Stremio.exe", "stremio-shell-ng.exe", "dx.bi.exe", "Stremio Enhanced.exe", "stremio\n", "stremio-shell-n", "dx.bi", "Stremio Enhanced", "stremio-enhance"} {
		if !isClientProcess(name) {
			t.Errorf("%q not recognised", name)
		}
	}
	for _, name := range []string{"", "stremio-runtime", "stremio-smart-search", "StremioSmartSearch.exe", "chrome.exe", "dx.bi Helper", "stremioservice.exe"} {
		if isClientProcess(name) {
			t.Errorf("%q wrongly recognised", name)
		}
	}
}

func TestClientRunningDoesNotFailHere(t *testing.T) {
	// No Stremio app runs in the test environment; the scan must simply work.
	if testing.Short() {
		t.Skip()
	}
	_ = testApp(t).ClientRunning()
}
