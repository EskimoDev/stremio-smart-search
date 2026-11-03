package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/search"
)

// Standby lets the helper stay small while no Stremio app is open. Almost all
// of its memory is the search index, so in standby the index is dropped and
// handed back to the OS while the port stays open: a search still gets an
// answer, it just waits for the index to load first.
//
// The run mode is "always" (the default: the index stays loaded) or "stremio"
// (load it while Stremio, dx.bi or Stremio Enhanced is running, or while
// searches keep arriving from a browser, and drop it otherwise).
const (
	RunAlways  = "always"
	RunStremio = "stremio"

	// How often the watcher looks for a running Stremio app. The scan is a
	// single process listing (Windows, Linux) or `ps` (macOS).
	standbyScan = 5 * time.Second
	// While a Stremio app is open and the index is loaded, the watcher only
	// has to notice it closing, which the 2-minute grace makes unhurried.
	// A scan of 340 processes measured 6 ms of CPU on Windows.
	standbyScanOpen = 30 * time.Second
	// How long the index stays loaded after the last Stremio window closed.
	standbyAfterClose = 2 * time.Minute
	// How long it stays loaded after a search that came while no Stremio app
	// was running, such as from web.stremio.com in a browser.
	standbyAfterSearch = 15 * time.Minute
)

func validRunMode(mode string) bool { return mode == RunAlways || mode == RunStremio }

func runModePath(dataDir string) string { return filepath.Join(dataDir, "run-mode.json") }

func loadRunMode(dataDir string) string {
	var saved struct {
		Mode string `json:"mode"`
	}
	data, err := os.ReadFile(runModePath(dataDir))
	if err == nil && json.Unmarshal(data, &saved) == nil && validRunMode(saved.Mode) {
		return saved.Mode
	}
	return RunAlways
}

// standby is guarded by App.mu unless noted.
type standby struct {
	mode       string
	asleep     bool          // the index was dropped to save memory
	seen       time.Time     // last scan that found a Stremio app
	clientOpen bool          // result of the last scan
	searched   time.Time     // last search while no Stremio app was open
	changed    chan struct{} // wakes the watcher when the mode changes
	waking     chan struct{} // closed when a load in progress finishes
}

// SetRunMode switches between always loaded and loaded only with Stremio.
func (a *App) SetRunMode(mode string) error {
	if !validRunMode(mode) {
		return errInvalidRunMode
	}
	if err := writeJSONFile(runModePath(a.dataDir), map[string]string{"mode": mode}); err != nil {
		return err
	}
	a.mu.Lock()
	a.standby.mode = mode
	a.status.RunMode = mode
	a.mu.Unlock()
	select {
	case a.standby.changed <- struct{}{}:
	default:
	}
	if mode == RunAlways {
		a.Wake()
	}
	return nil
}

var errInvalidRunMode = errors.New("choose always or only while Stremio is open")

// Wake loads the index again after standby and returns once searches can use
// it. Concurrent callers share one load.
func (a *App) Wake() {
	a.mu.Lock()
	if !a.standby.asleep {
		a.mu.Unlock()
		return
	}
	if waiting := a.standby.waking; waiting != nil {
		a.mu.Unlock()
		<-waiting
		return
	}
	done := make(chan struct{})
	a.standby.waking = done
	a.mu.Unlock()

	// Load the saved snapshot without touching the rest of the status: a
	// catalogue build may be running, and its own finish installs its index.
	// If the snapshot cannot be read, the next search tries again.
	var engine *search.Engine
	path := a.catalogPath()
	c, err := catalog.Load(path)
	if err != nil {
		c, err = catalog.Load(path + ".previous")
	}
	if err == nil {
		engine = search.New(c.Titles)
		c = nil
	}

	a.mu.Lock()
	a.standby.waking = nil
	// A build that finished meanwhile already installed a newer index and
	// cleared asleep; keep that one.
	if engine != nil && a.standby.asleep {
		a.engine.Store(engine)
		a.standby.asleep, a.status.Standby = false, false
	}
	a.mu.Unlock()
	close(done)
	if engine != nil {
		// Return the loader's parsed copy, as at startup.
		debug.FreeOSMemory()
	}
}

// sleep drops the index if nothing needs it. It never interrupts a catalogue
// build, a storage move or a load in progress.
func (a *App) sleep() bool {
	a.mu.Lock()
	if a.standby.mode != RunStremio || a.standby.asleep || a.standby.waking != nil ||
		!a.status.Ready || a.status.Building || a.moving || a.engine.Load() == nil {
		a.mu.Unlock()
		return false
	}
	a.standby.asleep = true
	a.status.Standby = true
	a.engine.Store(nil)
	a.mu.Unlock()
	// Hand the index back to the OS now rather than whenever the runtime
	// gets to it. This happens once per standby, never per search.
	debug.FreeOSMemory()
	return true
}

// noteSearch records a search for the idle timer and, in standby, loads the
// index before the search runs.
func (a *App) noteSearch() {
	a.mu.Lock()
	if a.standby.mode == RunStremio && !a.standby.clientOpen {
		a.standby.searched = a.now()
	}
	asleep := a.standby.asleep
	a.mu.Unlock()
	if asleep {
		a.Wake()
	}
}

// StandbyTick runs one watcher step: wake while a Stremio app is open, and
// drop the index once none has been open for a while and no browser
// searches came in. Tests call it directly with a fake clientRunning.
func (a *App) StandbyTick(open bool) {
	a.mu.Lock()
	if a.standby.mode != RunStremio {
		a.mu.Unlock()
		return
	}
	now := a.now()
	a.standby.clientOpen = open
	if open {
		a.standby.seen = now
	}
	asleep := a.standby.asleep
	idle := now.Sub(a.standby.seen) >= standbyAfterClose && now.Sub(a.standby.searched) >= standbyAfterSearch
	a.mu.Unlock()
	switch {
	case open && asleep:
		a.Wake()
	case !open && !asleep && idle:
		a.sleep()
	}
}

// nextScan is how long the watcher waits before looking again.
func (a *App) nextScan() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.standby.clientOpen && !a.standby.asleep {
		return standbyScanOpen
	}
	return standbyScan
}

// StartStandby runs the Stremio watcher. In "always" mode it waits for a
// mode change and does no work at all.
func (a *App) StartStandby(ctx context.Context) {
	go func() {
		timer := time.NewTimer(standbyScan)
		defer timer.Stop()
		for {
			a.mu.Lock()
			mode := a.standby.mode
			a.mu.Unlock()
			if mode == RunStremio {
				a.StandbyTick(a.ClientRunning())
				timer.Reset(a.nextScan())
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				case <-a.standby.changed:
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-a.standby.changed:
			}
		}
	}()
}

// Process names of the Stremio apps Smart Search knows, lower-cased. Linux
// truncates process names to 15 characters, so those are matched by prefix.
var clientProcessNames = []string{
	"stremio.exe", "stremio-shell-ng.exe", "dx.bi.exe", "stremio enhanced.exe", // Windows
	"stremio", "stremio-shell-ng", "dx.bi", "dxbi", "stremio enhanced", "stremio-enhanced", // macOS, Linux
}

func isClientProcess(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, want := range clientProcessNames {
		if name == want || (len(name) == 15 && strings.HasPrefix(want, name)) {
			return true
		}
	}
	return false
}
