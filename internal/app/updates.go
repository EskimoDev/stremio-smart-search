package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// Automatic catalogue updates.
//
// There is no daemon, scheduled task or always-running service: while the
// helper runs, one goroutine wakes a few minutes after start-up and then every
// half hour, decides from persisted timestamps whether an update is due, and
// if so asks IMDb (HEAD only) whether the files changed before rebuilding with
// the existing build path. Searching never waits for any of it.

const (
	updateSettingsFile = "updates.json"
	updateStateFile    = "update-state.json"

	DefaultUpdateHours = 48
	// updateStartDelay keeps the first check away from start-up, when the
	// snapshot is loading and the user is opening Stremio.
	updateStartDelay = 3 * time.Minute
	// updateTick is how often a running helper re-evaluates. Each tick that
	// is not due costs a few comparisons and no I/O.
	updateTick = 30 * time.Minute
	// recheckAfterUnchanged spaces metadata checks while IMDb has not
	// published new files. IMDb normally publishes daily.
	recheckAfterUnchanged = 6 * time.Hour
	// Failed attempts back off 1, 2, 4, 8, 16 then 24 hours, persisted, so a
	// restart never retries a large download straight away.
	backoffBase  = time.Hour
	backoffMax   = 24 * time.Hour
	checkTimeout = 30 * time.Second
)

// UpdateIntervals are the offered frequencies, in hours.
var UpdateIntervals = []int{24, 48, 72, 168}

func validInterval(hours int) bool {
	for _, h := range UpdateIntervals {
		if h == hours {
			return true
		}
	}
	return false
}

// UpdateSettings is the user's choice, stored in updates.json.
type UpdateSettings struct {
	Auto          bool `json:"auto"`
	IntervalHours int  `json:"intervalHours"`
}

// updateState is what happened, stored separately in update-state.json so the
// user's choice is never rewritten by routine bookkeeping.
type updateState struct {
	// LastSuccess is when this helper last saved a new catalogue.
	LastSuccess time.Time `json:"lastSuccess,omitzero"`
	// LastCheck is the last metadata check, and CheckResult its outcome.
	LastCheck   time.Time `json:"lastCheck,omitzero"`
	CheckResult string    `json:"checkResult,omitempty"`
	// Attempt is set while a download runs. Finding it set at start-up means
	// the previous run stopped mid-download; that counts as a failure.
	Attempt     time.Time `json:"attempt,omitzero"`
	Failures    int       `json:"failures,omitempty"`
	LastFailure time.Time `json:"lastFailure,omitzero"`
	LastError   string    `json:"lastError,omitempty"`
	RetryAfter  time.Time `json:"retryAfter,omitzero"`
	// Sources are the validators of the files the active catalogue came from.
	Sources []catalog.SourceFile `json:"sources,omitempty"`
}

// UpdateStatus is the automatic-update part of /api/status.
type UpdateStatus struct {
	Auto          bool      `json:"auto"`
	IntervalHours int       `json:"intervalHours"`
	Intervals     []int     `json:"intervals"`
	LastSuccess   time.Time `json:"lastSuccess,omitzero"`
	LastCheck     time.Time `json:"lastCheck,omitzero"`
	CheckResult   string    `json:"checkResult,omitempty"`
	NextCheck     time.Time `json:"nextCheck,omitzero"`
	// State is one of off, waiting, checking, updating, metered, retrying.
	State     string `json:"state"`
	Automatic bool   `json:"automatic"`
	Failures  int    `json:"failures"`
	LastError string `json:"lastError,omitempty"`
	// Network is metered, unmetered or unknown (no reliable detection here).
	Network string `json:"network"`
	// DownloadBytes estimates one full update with the active selection.
	DownloadBytes int64 `json:"downloadBytes"`
}

// IMDb dataset sizes on 1 November 2026, used until a download has recorded
// the real ones.
var defaultDatasetBytes = map[string]int64{
	catalog.RatingsFile: 8_728_224,
	catalog.BasicsFile:  229_540_187,
	catalog.AkasFile:    520_203_284,
}

type updater struct {
	settings  UpdateSettings
	state     updateState
	checking  bool
	automatic bool // the running build was started by the schedule
	network   string
	wake      chan struct{}
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".update-*.tmp")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// loadUpdates reads the settings and state. With no settings file, a new
// installation (no catalogue yet) gets automatic updates on; an existing one
// keeps updating only when asked, so nobody's bandwidth use changes without
// them seeing the option first. The result is written so it never flips.
func (a *App) loadUpdates() {
	u := &a.updates
	u.wake = make(chan struct{}, 1)
	u.network = "unknown"
	settingsPath := filepath.Join(a.dataDir, updateSettingsFile)
	data, err := os.ReadFile(settingsPath)
	if err == nil && json.Unmarshal(data, &u.settings) == nil {
		if !validInterval(u.settings.IntervalHours) {
			u.settings.IntervalHours = DefaultUpdateHours
		}
	} else {
		existing := false
		for _, name := range catalogFiles {
			if _, err := os.Stat(filepath.Join(a.catalogDir, name)); err == nil {
				existing = true
			}
		}
		u.settings = UpdateSettings{Auto: !existing, IntervalHours: DefaultUpdateHours}
		if errors.Is(err, os.ErrNotExist) {
			_ = writeJSONFile(settingsPath, u.settings)
		}
	}
	if data, err := os.ReadFile(filepath.Join(a.dataDir, updateStateFile)); err == nil {
		_ = json.Unmarshal(data, &u.state)
	}
}

func (a *App) saveUpdateStateLocked() {
	_ = writeJSONFile(filepath.Join(a.dataDir, updateStateFile), a.updates.state)
}

func backoff(failures int) time.Duration {
	delay := backoffBase
	for i := 1; i < failures && delay < backoffMax; i++ {
		delay *= 2
	}
	return min(delay, backoffMax)
}

func (a *App) recordFailureLocked(at time.Time, message string) {
	s := &a.updates.state
	s.Attempt = time.Time{}
	s.Failures++
	s.LastFailure = at
	s.LastError = message
	s.RetryAfter = at.Add(backoff(s.Failures))
	a.saveUpdateStateLocked()
}

// sane clamps persisted times that lie in the future, which happens after the
// clock is set back. Without it a timestamp a year ahead would stop updates
// for a year; with it the schedule restarts from now.
func sane(t, now time.Time) time.Time {
	if t.After(now) {
		return now
	}
	return t
}

// nextUpdateLocked returns when the schedule next allows a metadata check.
func (a *App) nextUpdateLocked(now time.Time) time.Time {
	u := &a.updates
	s := &u.state
	last := s.LastSuccess
	if last.IsZero() {
		last = a.status.BuiltAt // catalogues made before automatic updates
	}
	if last.IsZero() {
		last = now
	}
	next := sane(last, now).Add(time.Duration(u.settings.IntervalHours) * time.Hour)
	if s.CheckResult == string(catalog.Unchanged) && !s.LastCheck.IsZero() {
		next = later(next, sane(s.LastCheck, now).Add(recheckAfterUnchanged))
	}
	if s.Failures > 0 {
		// A retry time further ahead than the longest backoff can only come
		// from a clock change; cap it.
		retry := s.RetryAfter
		if retry.After(now.Add(backoffMax)) {
			retry = now.Add(backoffMax)
		}
		next = later(next, retry)
	}
	return next
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func (a *App) updateStatusLocked() UpdateStatus {
	u := &a.updates
	now := a.now()
	st := UpdateStatus{Auto: u.settings.Auto, IntervalHours: u.settings.IntervalHours, Intervals: UpdateIntervals,
		LastSuccess: u.state.LastSuccess, LastCheck: u.state.LastCheck, CheckResult: u.state.CheckResult,
		Failures: u.state.Failures, LastError: u.state.LastError, Network: u.network, Automatic: u.automatic && a.status.Building}
	if st.LastSuccess.IsZero() {
		st.LastSuccess = a.status.BuiltAt
	}
	aliases := catalog.AliasOptions{Language: a.status.Language, Region: a.status.Region}
	known := map[string]int64{}
	for _, f := range u.state.Sources {
		known[f.File] = f.Size
	}
	for _, file := range catalog.DatasetFiles(aliases) {
		if known[file] > 0 {
			st.DownloadBytes += known[file]
		} else {
			st.DownloadBytes += defaultDatasetBytes[file]
		}
	}
	switch {
	case !u.settings.Auto:
		st.State = "off"
	case a.status.Building:
		st.State = "updating"
	case u.checking:
		st.State = "checking"
	case u.network == "metered":
		st.State = "metered"
	case u.state.Failures > 0:
		st.State = "retrying"
	default:
		st.State = "waiting"
	}
	if u.settings.Auto && a.status.Ready {
		st.NextCheck = a.nextUpdateLocked(now)
	}
	return st
}

// SetUpdates changes the automatic-update settings. Nil fields are kept.
func (a *App) SetUpdates(auto *bool, hours *int) error {
	a.mu.Lock()
	settings := a.updates.settings
	if auto != nil {
		settings.Auto = *auto
	}
	if hours != nil {
		if !validInterval(*hours) {
			a.mu.Unlock()
			return errors.New("choose a listed update frequency")
		}
		settings.IntervalHours = *hours
	}
	err := writeJSONFile(filepath.Join(a.dataDir, updateSettingsFile), settings)
	if err == nil {
		a.updates.settings = settings
	}
	wake := a.updates.wake
	a.mu.Unlock()
	if err == nil && wake != nil {
		select { // re-evaluate soon, e.g. an overdue catalogue just switched on
		case wake <- struct{}{}:
		default:
		}
	}
	return err
}

// beginBuildLocked records a download in progress, so a run that dies during
// it is noticed at the next start-up.
func (a *App) beginBuildLocked(automatic bool) {
	a.updates.automatic = automatic
	a.updates.state.Attempt = a.now()
	a.saveUpdateStateLocked()
}

// finishBuildLocked records the outcome of any build, manual or automatic.
func (a *App) finishBuildLocked(c *catalog.Catalog, err error) {
	a.updates.automatic = false
	s := &a.updates.state
	if err != nil {
		a.recordFailureLocked(a.now(), err.Error())
		return
	}
	*s = updateState{LastSuccess: a.now(), Sources: c.Sources}
	a.saveUpdateStateLocked()
}

// StartUpdates runs the update schedule until ctx ends. It returns at once.
// Only the instance that serves searches calls it; a second launch that just
// hands over to the running helper never touches the update state.
func (a *App) StartUpdates(ctx context.Context) {
	a.mu.Lock()
	if attempt := a.updates.state.Attempt; !attempt.IsZero() && !a.status.Building {
		// The previous run stopped during a download (crash, power loss or
		// a forced close). Back off as for any other failure.
		a.recordFailureLocked(sane(attempt, a.now()), "The previous update was interrupted.")
	}
	wake := a.updates.wake
	a.mu.Unlock()
	go func() {
		timer := time.NewTimer(updateStartDelay)
		defer timer.Stop()
		started := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				started = true
			case <-wake:
				if !started {
					continue // the first tick still comes after the start-up delay
				}
				timer.Stop()
			}
			a.UpdateTick(ctx)
			timer.Reset(updateTick)
		}
	}()
}

// UpdateTick checks once whether an automatic update is due and, if IMDb has
// new files, starts it. It blocks only for the metadata check.
func (a *App) UpdateTick(ctx context.Context) {
	a.mu.Lock()
	u := &a.updates
	now := a.now()
	if !u.settings.Auto || !a.status.Ready || a.status.Building || a.moving || u.checking || now.Before(a.nextUpdateLocked(now)) {
		a.mu.Unlock()
		return
	}
	u.checking = true
	profile, _ := catalog.ProfileByID(a.status.Profile)
	aliases := catalog.AliasOptions{Language: a.status.Language, Region: a.status.Region}
	if aliases.Validate() != nil {
		aliases = catalog.AliasOptions{}
	}
	known := append([]catalog.SourceFile(nil), u.state.Sources...)
	since := u.state.LastSuccess
	a.mu.Unlock()

	network := "unknown"
	if a.NetworkCost != nil {
		network = a.NetworkCost()
	}
	result, err := catalog.Freshness(""), error(nil)
	if network != "metered" {
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		result, err = a.CheckSource(checkCtx, catalog.DatasetFiles(aliases), known)
		cancel()
	}

	a.mu.Lock()
	u.checking = false
	u.network = network
	if network == "metered" || ctx.Err() != nil || a.status.Building || !u.state.LastSuccess.Equal(since) {
		// Metered, shutting down, or a manual refresh started or finished
		// meanwhile: this check's answer no longer matters.
		a.mu.Unlock()
		return
	}
	now = a.now()
	if err != nil {
		u.state.LastCheck = now
		u.state.CheckResult = "failed"
		a.recordFailureLocked(now, "Could not reach IMDb: "+err.Error())
		a.mu.Unlock()
		return
	}
	u.state.LastCheck = now
	u.state.CheckResult = string(result)
	if result == catalog.Unchanged {
		a.saveUpdateStateLocked()
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	// Changed, or unknown with the interval elapsed: rebuild exactly as a
	// manual refresh would. If a manual build started meanwhile, this does
	// nothing.
	a.startBuildMode(profile, aliases, true)
}

// DisableUpdates turns automatic updates off for this run only, without
// saving, for a helper serving a --catalog file it must not replace.
func (a *App) DisableUpdates() {
	a.mu.Lock()
	a.updates.settings.Auto = false
	a.mu.Unlock()
}

// adoptAutomaticBuild turns a running automatic build with the same selection
// into a manual one when the user presses Refresh now, so the click is not
// refused and a failure is shown on the page.
func (a *App) adoptAutomaticBuild(profile catalog.Profile, aliases catalog.AliasOptions) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.status.Building || !a.updates.automatic || a.status.BuildProfile != profile.ID ||
		a.status.BuildLanguage != aliases.Language || a.status.BuildRegion != aliases.Region {
		return false
	}
	a.updates.automatic = false
	return true
}
