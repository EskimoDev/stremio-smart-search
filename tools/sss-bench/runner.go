package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/tools/internal/benchdata"
	"github.com/EskimoDev/stremio-smart-search/tools/sss-bench/probes/probekit"
)

const (
	appGOMAXPROCS  = 2   // what cmd/smart-search sets for itself
	appMemoryMiB   = 256 // likewise
	refreshTimeout = 10 * time.Minute
	sustainSampler = 250 * time.Millisecond
)

// Profile is one named amount of work.
type Profile struct {
	Name         string
	tier         int
	Titles       int
	Repetitions  int
	WarmupPasses int
	Passes       int
	Settle       time.Duration
	Throughput   time.Duration
	Concurrency  []int
	Sustain      time.Duration
	Suites       []string
	Generated    bool
	CatalogOps   bool
	Rebuilds     int
	Cost         string
}

func profiles() []Profile {
	allSuites := []string{"cases", "phonetic-holdout", "phonetic-validation"}
	return []Profile{
		{Name: "quick", tier: tierQuick, Titles: 20_000, Repetitions: 6, WarmupPasses: 1, Passes: 3, Settle: 3 * time.Second, Suites: []string{"cases"}, Cost: "about 30 seconds; a few MB of temporary disk"},
		{Name: "standard", tier: tierStandard, Titles: 50_000, Repetitions: 8, WarmupPasses: 2, Passes: 10, Settle: 3 * time.Second, Throughput: 2 * time.Second, Concurrency: []int{1, 2, 4}, Sustain: 10 * time.Second, Suites: allSuites, Cost: "about 4 minutes; a few MB of temporary disk"},
		{Name: "full", tier: tierFull, Titles: 200_000, Repetitions: 10, WarmupPasses: 3, Passes: 20, Settle: 3 * time.Second, Throughput: 3 * time.Second, Concurrency: []int{1, 2, 4, 8}, Sustain: time.Minute, Suites: allSuites, Generated: true, CatalogOps: true, Rebuilds: 2, Cost: "about 20 minutes; about 100 MB of temporary disk"},
	}
}

func profileByName(name string) (Profile, bool) {
	for _, p := range profiles() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// Config is a validated command line.
type Config struct {
	Profile     Profile
	Sources     []string // source trees under test, for scrubbing paths from messages
	Catalog     string
	IMDbDir     string
	Repo        string
	Label       string
	WorkDir     string
	KeepWork    bool
	Interleaved bool
}

// logger writes progress to stderr, so stdout stays machine-readable.
type logger struct {
	out   io.Writer
	quiet bool
}

func (l *logger) printf(format string, args ...any) {
	if !l.quiet {
		fmt.Fprintf(l.out, "sss-bench: "+format+"\n", args...)
	}
}

// session is everything shared by the targets of one run.
type session struct {
	cfg         Config
	profile     Profile
	log         *logger
	ws          *workspace
	env         []string
	environment Environment
	harness     HarnessInfo
	workload    WorkloadInfo
	catalogue   CatalogueInfo
	catalogPath string
	queries     []benchQuery
	wire        []probekit.Query
	suites      []suiteFile // those this profile scores
	firstSearch probekit.Query
	imdbDir     string
	opsProfile  string
	opsNote     string
	limits      probekit.Limits
	warnings    []string
	scrubber    *strings.Replacer
}

func topTitle(c *catalog.Catalog, kind string) probekit.Query {
	best := catalog.Title{}
	for _, t := range c.Titles {
		if t.Type == kind && (t.Votes > best.Votes || best.ID == "") {
			best = t
		}
	}
	return probekit.Query{Text: best.Name, Kind: kind}
}

// inputError marks a problem with what the user supplied, as opposed to a
// failure while measuring. It maps to exit code 2.
type inputError struct{ error }

// newSession resolves all inputs before anything is measured, so a bad path or
// corrupt file costs a second rather than a long run. Nothing here touches a
// file outside the new workspace.
func newSession(ctx context.Context, cfg Config, log *logger) (*session, error) {
	s := &session{cfg: cfg, profile: cfg.Profile, log: log, limits: probekit.Limits{GOMAXPROCS: appGOMAXPROCS, MemoryLimitMiB: appMemoryMiB}}
	commit, dirty := gitState(ctx, cfg.Repo)
	s.harness = HarnessInfo{Version: harnessVersion, MeasurementVersion: measurementVersion, Commit: commit, Dirty: dirty, Go: runtime.Version()}
	s.environment = describeEnvironment()
	caseDir := filepath.Join(cfg.Repo, "testdata", "quality")
	// The fixture always holds the titles of every suite, so it does not vary with
	// which suites a profile scores.
	all := []suiteFile{}
	for _, name := range []string{"cases", "phonetic-holdout", "phonetic-validation"} {
		suite, err := loadSuite(caseDir, name)
		if err != nil {
			return nil, inputError{err}
		}
		all = append(all, suite)
		if slices.Contains(s.profile.Suites, name) {
			s.suites = append(s.suites, suite)
		}
	}
	var supplied *catalog.Catalog
	if cfg.Catalog != "" {
		c, err := catalog.Load(cfg.Catalog)
		if err != nil {
			return nil, inputError{fmt.Errorf("catalogue %s could not be read: %w", filepath.Base(cfg.Catalog), err)}
		}
		supplied = c
	}
	if cfg.IMDbDir != "" {
		for _, name := range []string{"title.ratings.tsv.gz", "title.basics.tsv.gz"} {
			if _, err := os.Stat(filepath.Join(cfg.IMDbDir, name)); err != nil {
				return nil, inputError{fmt.Errorf("--imdb-dir has no %s; the harness never downloads it", name)}
			}
		}
	}
	ws, err := newWorkspace(cfg.WorkDir)
	if err != nil {
		return nil, inputError{err}
	}
	s.ws = ws
	home, _ := os.UserHomeDir()
	pairs := []string{ws.root, "<workspace>", cfg.Repo, "<repo>"}
	for _, source := range cfg.Sources {
		if source != "" { // a --binary target has none, and "" would match everywhere
			pairs = append(pairs, source, "<source>")
		}
	}
	if home != "" {
		pairs = append(pairs, home, "<home>")
	}
	s.scrubber = strings.NewReplacer(pairs...)
	if warning := ws.acquireLock(); warning != "" {
		s.warnings = append(s.warnings, warning)
	}
	var removed []string
	s.env, removed = childEnv(ws)
	if len(removed) > 0 {
		s.warnings = append(s.warnings, "removed from the benchmark's environment: "+strings.Join(removed, ", "))
	}
	if installedHelperRunning() {
		s.warnings = append(s.warnings, fmt.Sprintf("something is listening on Smart Search's default port %d. It is left alone, but a running helper can add noise", defaultAppPort))
	}
	if err := s.prepareCatalogue(supplied, all); err != nil {
		s.close()
		return nil, err
	}
	cases := all[0].cases
	s.queries = buildQueries(cases)
	s.wire = make([]probekit.Query, len(s.queries))
	for i, q := range s.queries {
		s.wire[i] = q.Query
	}
	s.workload = WorkloadInfo{Version: workloadVersion, Profile: s.profile.Name, Repetitions: s.profile.Repetitions, Interleaved: cfg.Interleaved, Queries: len(s.queries), QueriesSHA256: queriesSHA(s.queries), WarmupPasses: s.profile.WarmupPasses, Passes: s.profile.Passes, SettleSeconds: s.profile.Settle.Seconds(), Throughput: s.profile.Throughput.Seconds(), Concurrency: s.profile.Concurrency, SustainSeconds: s.profile.Sustain.Seconds(), Rebuilds: s.profile.Rebuilds, Seed: workloadSeed, GOMAXPROCS: appGOMAXPROCS, MemoryLimitMiB: appMemoryMiB, Inputs: map[string]string{}}
	for _, suite := range s.suites {
		s.workload.Inputs[suite.name] = suite.sha256
	}
	if s.cfg.IMDbDir != "" {
		for _, name := range []string{"title.ratings.tsv.gz", "title.basics.tsv.gz"} {
			sum, _, err := fileFingerprint(filepath.Join(s.cfg.IMDbDir, name))
			if err != nil {
				s.close()
				return nil, inputError{err}
			}
			s.workload.Inputs[name] = sum
		}
	}
	s.workload.ID = workloadID(s.workload)
	return s, nil
}

// prepareCatalogue fixes the data under test inside the workspace: a generated
// fixture, or a private copy of a supplied snapshot, which is then never opened
// for writing.
func (s *session) prepareCatalogue(supplied *catalog.Catalog, all []suiteFile) error {
	directory := filepath.Join(s.ws.root, "catalogue")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	var c *catalog.Catalog
	var err error
	if supplied != nil {
		c = supplied
		s.catalogPath = filepath.Join(directory, filepath.Base(s.cfg.Catalog))
		if err = copyFile(s.cfg.Catalog, s.catalogPath); err != nil {
			return err
		}
		s.catalogue, err = describeCatalogue(c, "supplied", filepath.Base(s.cfg.Catalog), s.cfg.Catalog)
		if err != nil {
			return err
		}
		s.warnings = append(s.warnings, "supplied catalogue: title names may appear in the saved report; keep it private if the data's terms require that")
	} else {
		s.log.printf("generating a %d-title fixture", s.profile.Titles)
		c = buildFixture(s.profile.Titles, all, fixtureLanguage)
		s.catalogPath = filepath.Join(directory, "fixture.json.gz")
		if err = catalog.Save(s.catalogPath, c); err != nil {
			return err
		}
		s.catalogue, err = describeCatalogue(c, "synthetic", fmt.Sprintf("fixture-%d", s.profile.Titles), s.catalogPath)
		if err != nil {
			return err
		}
	}
	s.firstSearch = topTitle(c, "movie")
	if s.profile.CatalogOps {
		s.opsProfile = catalog.Profiles()[len(catalog.Profiles())-1].ID
		for _, p := range catalog.Profiles() {
			if s.catalogue.Movies <= p.Movies && s.catalogue.Series <= p.Series {
				s.opsProfile = p.ID
				break
			}
		}
		if c.Profile != "" {
			s.opsProfile = c.Profile
		}
		switch {
		case s.cfg.IMDbDir != "":
			s.imdbDir = s.cfg.IMDbDir
		case supplied == nil:
			s.imdbDir = filepath.Join(s.ws.root, "imdb")
			if err := os.MkdirAll(s.imdbDir, 0o700); err != nil {
				return err
			}
			if err := writeIMDbShaped(s.imdbDir, c); err != nil {
				return err
			}
		default:
			s.opsNote = "needs --imdb-dir with local IMDb files for a supplied catalogue"
		}
	}
	// The generated catalogue must not sit in the harness's heap during timing.
	c = nil
	supplied = nil
	debug.FreeOSMemory()
	return nil
}

func (s *session) close() {
	s.ws.releaseLock()
	if s.cfg.KeepWork {
		s.log.printf("keeping workspace %s", s.ws.root)
		return
	}
	if err := s.ws.cleanup(); err != nil {
		fmt.Fprintln(s.log.out, "sss-bench: warning:", err)
	}
}

func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// timedQuery is one latency observation and the workload query it belongs to.
type timedQuery struct {
	query int
	ms    float64
}

func sortedMS(samples []timedQuery, keep func(int) bool) []float64 {
	values := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if keep == nil || keep(sample.query) {
			values = append(values, sample.ms)
		}
	}
	sort.Float64s(values)
	return values
}

// addLatency records one path's percentiles for one fresh process.
func (s *session) addLatency(c *collector, path string, cold, warm []timedQuery) {
	prefix := "search." + path + "."
	sorted := sortedMS(cold, nil)
	if len(sorted) > 0 {
		c.addCount(prefix+"cold.p50_ms", percentile(sorted, 50), len(sorted))
		c.addCount(prefix+"cold.p95_ms", percentile(sorted, 95), len(sorted))
	}
	sorted = sortedMS(warm, nil)
	if len(sorted) == 0 {
		return
	}
	c.addCount(prefix+"warm.mean_ms", mean(sorted), len(sorted))
	c.addCount(prefix+"warm.p50_ms", percentile(sorted, 50), len(sorted))
	c.addCount(prefix+"warm.p95_ms", percentile(sorted, 95), len(sorted))
	c.addCount(prefix+"warm.p99_ms", percentile(sorted, 99), len(sorted))
	for _, class := range queryClasses {
		values := sortedMS(warm, func(i int) bool { return s.queries[i].Class == class })
		if len(values) > 0 {
			c.addCount(prefix+"class."+class+".p95_ms", percentile(values, 95), len(values))
		}
	}
	for _, length := range lengthClasses {
		values := sortedMS(warm, func(i int) bool { return s.queries[i].Length == length })
		if len(values) > 0 {
			c.addCount(prefix+"length."+length+".p95_ms", percentile(values, 95), len(values))
		}
	}
}

// passCounts totals what sequential passes did besides succeeding.
type passCounts struct {
	errors, shed, overBudget int
	firstError               string
}

// httpPass runs the workload once in that pass's fixed order.
func (s *session) httpPass(ctx context.Context, sr *searcher, pass int, into []timedQuery, counts *passCounts) []timedQuery {
	for _, i := range probekit.Order(len(s.queries), s.workload.Seed, pass) {
		if ctx.Err() != nil {
			return into
		}
		ns, result, err := sr.timed(s.queries[i].Path)
		switch result {
		case served:
			into = append(into, timedQuery{i, float64(ns) / 1e6})
			if time.Duration(ns) >= searchBudget {
				counts.overBudget++
			}
		case shed:
			counts.shed++
		default:
			counts.errors++
			if counts.firstError == "" && err != nil {
				counts.firstError = err.Error()
			}
		}
	}
	return into
}

func (s *session) searchPaths() []string {
	paths := make([]string, len(s.queries))
	for i, q := range s.queries {
		paths[i] = q.Path
	}
	return paths
}

// addMemory records one counter, or says plainly that this OS has no such thing.
func addMemory(c *collector, id string, value uint64) {
	switch {
	case c.defs[id].counter == "":
		c.unavailable(id, "this platform has no equivalent counter")
	case value == 0:
		c.failed(id, "the counter read as zero")
	default:
		c.add(id, float64(value))
	}
}

// appStage measures the real binary, in a fresh process, over loopback HTTP. The
// client runs in this process, so its work never lands in the application's
// memory or CPU counters.
func (s *session) appStage(ctx context.Context, t *targetRun, rep int) error {
	c := t.col
	runDir := filepath.Join(t.dir, "runs", strconv.Itoa(rep))
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(runDir)
	a, err := startApp(ctx, t.appPath, filepath.Join(runDir, "data"), s.catalogPath, s.firstSearch.Text, s.env)
	if err != nil {
		return err
	}
	stopped := false
	defer func() {
		if !stopped {
			a.kill()
		}
	}()
	c.add("startup.ready_ms", float64(a.readyNS)/1e6)
	c.add("startup.first_search_ms", float64(a.firstSearchNS)/1e6)
	loaded, err := readProcess(a.pid)
	if err != nil {
		return fmt.Errorf("reading process counters: %w", err)
	}
	addMemory(c, "memory.app.after_load", loaded.Resident)

	status, err := a.status()
	if err != nil {
		return err
	}
	if !status.Ready || status.Movies != s.catalogue.Movies || status.Series != s.catalogue.Series {
		return fmt.Errorf("application indexed %d movies and %d series, expected %d and %d", status.Movies, status.Series, s.catalogue.Movies, s.catalogue.Series)
	}
	if rep == 0 {
		c.add("catalog.titles", float64(status.Movies+status.Series))
		c.add("catalog.movies", float64(status.Movies))
		c.add("catalog.series", float64(status.Series))
		c.add("catalog.aliases", float64(status.Aliases))
		c.add("catalog.file_bytes", float64(s.catalogue.FileBytes))
	}

	before, err := readProcess(a.pid)
	if err != nil {
		return err
	}
	machineBefore, machineKnown := systemCPU()
	settleStart := probekit.Nanos()
	if err := wait(ctx, s.profile.Settle); err != nil {
		return err
	}
	machineAfter, machineStillKnown := systemCPU()
	switch share, measured := busyShare(machineBefore, machineAfter); {
	case !machineKnown || !machineStillKnown:
		c.unavailable("system.background_cpu_pct", "this platform has no machine-wide CPU counter")
	case s.profile.Settle == 0 || !measured:
		c.unavailable("system.background_cpu_pct", "needs a settle period")
	default:
		c.add("system.background_cpu_pct", share)
	}
	settled, err := readProcess(a.pid)
	if err != nil {
		return err
	}
	if s.profile.Settle > 0 && before.HasCPU && settled.HasCPU {
		elapsed := time.Duration(probekit.Nanos() - settleStart)
		c.add("cpu.app.idle_pct", 100*float64(settled.CPU-before.CPU)/float64(elapsed))
	} else {
		c.unavailable("cpu.app.idle_pct", "needs a settle period and a CPU counter")
	}
	addMemory(c, "memory.app.idle", settled.Resident)
	switch {
	case settled.Peak == 0:
		c.unavailable("memory.app.load_overhead", "this platform cannot read the peak of a running process")
	case settled.Peak >= settled.Resident:
		c.add("memory.app.load_overhead", float64(settled.Peak-settled.Resident))
	}
	addMemory(c, "memory.app.private_idle", settled.Private)

	if err := a.warmConnection(); err != nil {
		return err
	}
	sr := a.searcher()
	var counts passCounts
	cold := s.httpPass(ctx, sr, 0, make([]timedQuery, 0, len(s.queries)), &counts)
	for pass := 1; pass <= s.profile.WarmupPasses; pass++ {
		s.httpPass(ctx, sr, pass, make([]timedQuery, 0, len(s.queries)), &passCounts{})
	}
	cpuBefore, err := readProcess(a.pid)
	if err != nil {
		return err
	}
	warm := make([]timedQuery, 0, s.profile.Passes*len(s.queries))
	first := s.profile.WarmupPasses + 1
	for pass := first; pass < first+s.profile.Passes; pass++ {
		warm = s.httpPass(ctx, sr, pass, warm, &counts)
	}
	cpuAfter, err := readProcess(a.pid)
	if err != nil {
		return err
	}
	if cpuBefore.HasCPU && cpuAfter.HasCPU && len(warm) > 0 {
		c.add("cpu.app.per_request_ms", float64(cpuAfter.CPU-cpuBefore.CPU)/float64(time.Millisecond)/float64(len(warm)))
	} else {
		c.unavailable("cpu.app.per_request_ms", "this platform has no process CPU counter")
	}
	s.addLatency(c, "http", cold, warm)
	c.add("search.http.errors", float64(counts.errors))
	c.add("search.http.shed", float64(counts.shed))
	c.add("search.http.over_budget", float64(counts.overBudget))
	if counts.errors > 0 {
		t.fail("search", fmt.Errorf("repetition %d: %d failed requests, first: %s", rep+1, counts.errors, counts.firstError))
	}
	if counts.shed > 0 {
		t.fail("search", fmt.Errorf("repetition %d: %d requests were shed with only one client running", rep+1, counts.shed))
	}
	searched, err := readProcess(a.pid)
	if err != nil {
		return err
	}
	addMemory(c, "memory.app.after_search", searched.Resident)

	paths := s.searchPaths()
	for _, level := range s.profile.Concurrency {
		r := a.closedLoop(ctx, paths, level, s.profile.Throughput, s.workload.Seed)
		c.addCount(fmt.Sprintf("search.http.served_rps.c%d", level), float64(r.Served)/r.Seconds, int(r.Served))
		if total := r.Served + r.Shed; total > 0 {
			c.add(fmt.Sprintf("search.http.shed_pct.c%d", level), 100*float64(r.Shed)/float64(total))
		}
		if len(r.Latencies) > 0 {
			sort.Float64s(r.Latencies)
			c.addCount(fmt.Sprintf("search.http.concurrent.c%d.p95_ms", level), percentile(r.Latencies, 95), len(r.Latencies))
		}
		if r.Errors > 0 {
			t.fail("concurrency", fmt.Errorf("repetition %d, %d clients: %d failed requests, first: %s", rep+1, level, r.Errors, r.FirstError))
		}
	}
	if s.profile.Sustain > 0 {
		s.sustain(ctx, a, t, paths)
	}
	stopErr := a.stop()
	stopped = true
	addMemory(c, "memory.app.peak", a.peak())
	return stopErr
}

// sustain searches continuously while sampling memory. Thirds rather than the end
// points are compared because the Go heap saws up and down between collections.
func (s *session) sustain(ctx context.Context, a *instance, t *targetRun, paths []string) {
	type reading struct {
		at       int64
		resident float64
	}
	var mu sync.Mutex
	var readings []reading
	done := make(chan struct{})
	var sampler sync.WaitGroup
	sampler.Add(1)
	go func() {
		defer sampler.Done()
		ticker := time.NewTicker(sustainSampler)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if sample, err := readProcess(a.pid); err == nil {
					mu.Lock()
					readings = append(readings, reading{probekit.Nanos(), float64(sample.Resident)})
					mu.Unlock()
				}
			}
		}
	}()
	r := a.closedLoop(ctx, paths, 1, s.profile.Sustain, s.workload.Seed+1)
	close(done)
	sampler.Wait()
	if r.Errors > 0 {
		t.fail("sustain", fmt.Errorf("%d failed requests, first: %s", r.Errors, r.FirstError))
	}
	third := len(readings) / 3
	if third < 2 {
		t.fail("sustain", fmt.Errorf("only %d memory samples in %s", len(readings), s.profile.Sustain))
		return
	}
	values := func(window []reading) []float64 {
		out := make([]float64, len(window))
		for i, item := range window {
			out[i] = item.resident
		}
		return out
	}
	head, foot := readings[:third], readings[len(readings)-third:]
	start, end := median(values(head)), median(values(foot))
	minutes := time.Duration(foot[len(foot)/2].at - head[len(head)/2].at).Minutes()
	t.col.addCount("memory.app.sustained_end", end, len(foot))
	t.col.addCount("memory.app.sustained_growth", (end-start)/mib/minutes, len(readings))
}

// engineStage measures the search engine directly, in a fresh process.
func (s *session) engineStage(ctx context.Context, t *targetRun, rep int) error {
	request := probekit.EngineRequest{Mode: "measure", Catalog: s.catalogPath, Queries: s.wire, Limit: searchLimit, Seed: s.workload.Seed, WarmupPasses: s.profile.WarmupPasses, Passes: s.profile.Passes, ThroughputMS: int(s.profile.Throughput.Milliseconds()), Concurrency: s.profile.Concurrency, Rebuilds: s.profile.Rebuilds, Limits: s.limits}
	var r probekit.EngineResponse
	if err := s.runProbe(ctx, t, t.enginePath, "engine", request, &r); err != nil {
		return err
	}
	if want := len(s.queries) * s.profile.Passes; len(r.ColdNS) != len(s.queries) || len(r.WarmNS) != want {
		return fmt.Errorf("probe returned %d cold and %d warm timings, expected %d and %d", len(r.ColdNS), len(r.WarmNS), len(s.queries), want)
	}
	c := t.col
	ms := func(ns int64) float64 { return float64(ns) / 1e6 }
	c.add("catalog.read_ms", ms(r.ReadNS))
	c.add("catalog.load_ms", ms(r.LoadNS))
	c.add("catalog.load_cpu_ms", ms(r.LoadCPUNS))
	c.add("catalog.index_ms", ms(r.IndexNS))
	c.add("catalog.index_cpu_ms", ms(r.IndexCPUNS))
	c.add("memory.engine.heap_after_load", float64(r.HeapAfterLoad))
	c.add("memory.engine.heap_after_index", float64(r.HeapAfterIndex))
	c.add("memory.engine.heap_after_search", float64(r.HeapAfterSearch))
	addMemory(c, "memory.engine.peak", r.PeakResident)
	c.add("search.engine.allocs_per_search", r.AllocsPerSearch)
	c.add("search.engine.bytes_per_search", r.BytesPerSearch)
	attribute := func(pass int, durations []int64) []timedQuery {
		order := probekit.Order(len(s.queries), s.workload.Seed, pass)
		out := make([]timedQuery, len(durations))
		for i, ns := range durations {
			out[i] = timedQuery{order[i], ms(ns)}
		}
		return out
	}
	warm := make([]timedQuery, 0, len(r.WarmNS))
	first := s.profile.WarmupPasses + 1
	for pass := 0; pass < s.profile.Passes; pass++ {
		warm = append(warm, attribute(first+pass, r.WarmNS[pass*len(s.queries):(pass+1)*len(s.queries)])...)
	}
	s.addLatency(c, "engine", attribute(0, r.ColdNS), warm)
	for _, run := range r.Throughput {
		c.addCount(fmt.Sprintf("search.engine.qps.c%d", run.Concurrency), float64(run.Completed)/run.Seconds, int(run.Completed))
	}
	if len(r.RebuildNS) > 0 {
		values := make([]float64, len(r.RebuildNS))
		for i, ns := range r.RebuildNS {
			values[i] = ms(ns)
		}
		c.addCount("catalog.rebuild_ms", median(values), len(values))
	}
	return nil
}

// opsStage prepares a snapshot and runs a live refresh, from local files only.
func (s *session) opsStage(ctx context.Context, t *targetRun, rep int) error {
	if s.imdbDir == "" {
		return nil
	}
	c := t.col
	runDir := filepath.Join(t.dir, "ops", strconv.Itoa(rep))
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(runDir)
	ms := func(ns int64) float64 { return float64(ns) / 1e6 }
	prepare := probekit.OpsRequest{Mode: "prepare", IMDbDir: s.imdbDir, Profile: s.opsProfile, Output: filepath.Join(runDir, "prepared.json.gz"), Limits: s.limits}
	var r probekit.OpsResponse
	if err := s.runProbe(ctx, t, t.opsPath, "prepare", prepare, &r); err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	c.add("catalog.prepare_ms", ms(r.ParseNS))
	c.add("catalog.prepare_cpu_ms", ms(r.ParseCPUNS))
	c.add("catalog.save_ms", ms(r.SaveNS))
	addMemory(c, "memory.ops.prepare_peak", r.PeakResident)
	if rep == 0 {
		c.add("catalog.prepared_bytes", float64(r.FileBytes))
	}
	refresh := probekit.OpsRequest{Mode: "refresh", IMDbDir: s.imdbDir, Profile: s.opsProfile, Catalog: s.catalogPath, DataDir: filepath.Join(runDir, "data"), Query: s.firstSearch.Text, Kind: s.firstSearch.Kind, TimeoutSeconds: int(refreshTimeout.Seconds()), Limits: s.limits}
	r = probekit.OpsResponse{}
	if err := s.runProbe(ctx, t, t.opsPath, "refresh", refresh, &r); err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	c.add("catalog.refresh_ms", ms(r.RefreshNS))
	c.add("catalog.refresh_cpu_ms", ms(r.RefreshCPUNS))
	c.add("catalog.refresh_searches_served", float64(r.SearchesServed))
	c.add("catalog.refresh_searches_empty", float64(r.SearchesEmpty))
	addMemory(c, "memory.ops.refresh_peak", r.PeakResident)
	if r.SearchesEmpty > 0 {
		t.fail("refresh", fmt.Errorf("repetition %d: %d searches lost their result during the refresh", rep+1, r.SearchesEmpty))
	}
	return nil
}

// primeRuns is how many quick runs clear the first-launch effect described at prime.
const primeRuns = 3

// prime settles a new executable before anything is timed. The first launches of a
// new file behave differently on Windows: measured here, 48 MiB of working set
// against 11 MiB, with four times the page faults and identical private memory, so
// shared pages rather than the application's own. It clears after a few runs, not
// with time, and every freshly built binary would otherwise carry it into its first
// repetition. One discarded launch and pass then pages in the code being measured.
func (s *session) prime(ctx context.Context, t *targetRun) error {
	s.log.printf("%s: priming", t.spec.name)
	for i := 0; i < primeRuns; i++ {
		version := exec.CommandContext(ctx, t.appPath, "--version")
		version.Env = s.env
		version.Dir = s.ws.home
		if err := version.Run(); err != nil {
			return fmt.Errorf("%s --version: %w", appBinary, err)
		}
		for _, probe := range []string{t.enginePath, t.opsPath} {
			if probe == "" {
				continue
			}
			var ignored struct{}
			if err := s.runProbe(ctx, t, probe, "prime", map[string]string{"mode": "prime"}, &ignored); err != nil {
				return fmt.Errorf("priming a probe: %w", err)
			}
		}
	}
	runDir := filepath.Join(t.dir, "prime")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(runDir)
	a, err := startApp(ctx, t.appPath, filepath.Join(runDir, "data"), s.catalogPath, s.firstSearch.Text, s.env)
	if err != nil {
		return err
	}
	if err := a.warmConnection(); err != nil {
		a.kill()
		return err
	}
	s.httpPass(ctx, a.searcher(), 0, make([]timedQuery, 0, len(s.queries)), &passCounts{})
	if err := a.stop(); err != nil {
		return err
	}
	if t.enginePath == "" {
		return nil
	}
	var ignored probekit.EngineResponse
	request := probekit.EngineRequest{Mode: "dump", Catalog: s.catalogPath, Queries: s.wire, Limit: searchLimit, Limits: s.limits}
	return s.runProbe(ctx, t, t.enginePath, "prime-dump", request, &ignored)
}

// repetition runs one independent sample of every stage for one target.
func (s *session) repetition(ctx context.Context, t *targetRun, rep int) {
	stage := func(name string, run func() error) {
		if ctx.Err() != nil {
			return
		}
		if err := run(); err != nil {
			t.fail(fmt.Sprintf("%s repetition %d", name, rep+1), err)
		}
	}
	s.log.printf("%s: repetition %d of %d", t.spec.name, rep+1, s.profile.Repetitions)
	stage("application", func() error { return s.appStage(ctx, t, rep) })
	if t.enginePath != "" {
		stage("engine", func() error { return s.engineStage(ctx, t, rep) })
	}
	if s.profile.CatalogOps && t.opsPath != "" {
		stage("catalogue", func() error { return s.opsStage(ctx, t, rep) })
	}
}

func hitsEqual(a, b [][]probekit.Hit) bool { return reflect.DeepEqual(a, b) }

// qualityStage scores the suites. Results come from the engine probe when there is
// one: it has no time budget, so scores do not depend on machine load. A second
// fresh process checks that claim of determinism on every run.
func (s *session) qualityStage(ctx context.Context, t *targetRun) {
	if ctx.Err() != nil {
		return
	}
	s.log.printf("%s: scoring search quality", t.spec.name)
	c, err := catalog.Load(s.catalogPath)
	if err != nil {
		t.fail("quality", err)
		return
	}
	curated := suiteQueries(s.suites)
	queries := curated
	var generated []benchdata.Query
	if s.profile.Generated {
		var extra []probekit.Query
		generated, extra = generatedQueries(c)
		seen := map[probekit.Query]bool{}
		for _, q := range queries {
			seen[q] = true
		}
		for _, q := range extra {
			if !seen[q] {
				queries = append(queries[:len(queries):len(queries)], q)
			}
		}
	}
	var found answers
	path := pathHTTP
	runApp := func() (*instance, error) {
		runDir := filepath.Join(t.dir, "quality")
		if err := os.MkdirAll(runDir, 0o700); err != nil {
			return nil, err
		}
		return startApp(ctx, t.appPath, filepath.Join(runDir, "data"), s.catalogPath, s.firstSearch.Text, s.env)
	}
	if t.enginePath != "" {
		path = pathEngine
		request := probekit.EngineRequest{Mode: "dump", Catalog: s.catalogPath, Queries: queries, Limit: searchLimit, Limits: s.limits}
		var first, second probekit.EngineResponse
		if err := s.runProbe(ctx, t, t.enginePath, "dump", request, &first); err != nil {
			t.fail("quality", err)
			return
		}
		if err := s.runProbe(ctx, t, t.enginePath, "dump", request, &second); err != nil {
			t.fail("quality", err)
			return
		}
		if !hitsEqual(first.Results, second.Results) {
			t.fail("quality", errors.New("two identical searches in fresh processes returned different results: quality scores are not reproducible"))
		}
		found = answers{}
		for i, q := range queries {
			found[q] = first.Results[i]
		}
		a, err := runApp()
		if err != nil {
			t.fail("quality", err)
		} else {
			over, err := fetchHTTP(a, curated)
			stopErr := a.stop()
			if err == nil {
				err = stopErr
			}
			if err != nil {
				t.fail("quality", err)
			} else {
				curatedAnswers := answers{}
				for _, q := range curated {
					curatedAnswers[q] = found[q]
				}
				different, err := httpMismatches(curatedAnswers, over)
				if err != nil {
					t.fail("quality", err)
				} else if len(different) > 0 {
					t.warn("%d curated queries returned different results over HTTP than from the engine, for example %q: the server's time budget, or a change above the engine", len(different), different[0])
				}
			}
		}
	} else {
		a, err := runApp()
		if err != nil {
			t.fail("quality", err)
			return
		}
		found, err = fetchHTTP(a, queries)
		stopErr := a.stop()
		if err == nil {
			err = stopErr
		}
		if err != nil {
			t.fail("quality", err)
			return
		}
		t.warn("quality came from HTTP because this target has no engine probe; the server's time budget can affect it on a busy machine")
	}
	for _, suite := range s.suites {
		result := scoreSuite(c, suite, found, path)
		if result.Totals.TruthErrors > 0 {
			t.fail("quality", fmt.Errorf("%s: %d cases name an IMDb id that disagrees with this catalogue; they are excluded from every denominator", suite.name, result.Totals.TruthErrors))
		}
		t.suites = append(t.suites, result)
	}
	if s.profile.Generated {
		t.suites = append(t.suites, scoreGenerated(generated, found, path))
	}
}

// checkSupplied confirms a supplied catalogue is exactly as it was found.
func (s *session) checkSupplied(t *targetRun) {
	if s.cfg.Catalog == "" {
		return
	}
	sum, _, err := fileFingerprint(s.cfg.Catalog)
	if err != nil || sum != s.catalogue.FileSHA256 {
		t.fail("isolation", fmt.Errorf("the supplied catalogue changed during the run (read error: %v)", err))
	}
}

// result assembles what one target produced, with paths and names scrubbed: this
// text ends up in public pull requests.
func (s *session) result(t *targetRun, runID string, started time.Time) *Result {
	if s.opsNote != "" {
		for id, d := range t.col.defs {
			if d.needs == needsOps {
				t.col.unavailable(id, s.opsNote)
			}
		}
	}
	for id, d := range t.col.defs {
		if d.counter == "" && strings.HasPrefix(id, "memory.") {
			t.col.unavailable(id, "this platform has no equivalent counter")
		}
	}
	r := &Result{Schema: schemaResult, SchemaVersion: schemaVersion, RunID: runID, Label: s.cfg.Label, StartedAt: started.UTC(), DurationSeconds: time.Since(started).Seconds(), Harness: s.harness, Target: t.info, Environment: s.environment, Catalogue: s.catalogue, Workload: s.workload, Quality: t.suites, Errors: []Problem{}, Warnings: []string{}}
	r.Metrics = t.col.metrics(s.profile, s.workload.Repetitions)
	for _, p := range t.problems {
		r.Errors = append(r.Errors, Problem{Stage: p.Stage, Message: s.scrubber.Replace(p.Message)})
	}
	for _, w := range append(slices.Clone(s.warnings), t.warnings...) {
		r.Warnings = append(r.Warnings, s.scrubber.Replace(w))
	}
	if high, known := backgroundLoad(r); known && high > backgroundLimitPct {
		r.Warnings = append(r.Warnings, fmt.Sprintf("other work used up to %.0f%% of this machine's CPU while the application sat idle, so timings are less reliable; close it and repeat", high))
	}
	measured, broken := 0, false
	for i := range r.Metrics {
		r.Metrics[i].Reason = s.scrubber.Replace(r.Metrics[i].Reason)
		switch r.Metrics[i].Status {
		case metricOK:
			measured++
		case metricFailed:
			broken = true
		}
	}
	r.Target.EngineProbe = s.scrubber.Replace(r.Target.EngineProbe)
	r.Target.OpsProbe = s.scrubber.Replace(r.Target.OpsProbe)
	switch {
	case t.fatal || measured == 0:
		r.Status = statusFailed
	case broken || len(r.Errors) > 0:
		r.Status = statusPartial
	default:
		r.Status = statusComplete
	}
	return r
}

// run measures one target.
func (s *session) run(ctx context.Context, spec targetSpec, runID string) *Result {
	started := time.Now()
	t := s.prepareTarget(ctx, spec)
	if !t.fatal {
		if err := s.prime(ctx, t); err != nil {
			t.fail("priming", err)
		}
		for rep := 0; rep < s.profile.Repetitions; rep++ {
			s.repetition(ctx, t, rep)
		}
		s.qualityStage(ctx, t)
		s.checkSupplied(t)
	}
	if ctx.Err() != nil {
		t.fail("run", context.Canceled)
	}
	return s.result(t, runID, started)
}

// runInterleaved measures two targets in one session, alternating them as
// A B B A so that slow drift in the machine falls on both equally. Sequential
// before and after runs cannot offer that.
func (s *session) runInterleaved(ctx context.Context, baseline, candidate targetSpec, runID string) (*Result, *Result) {
	started := time.Now()
	a, b := s.prepareTarget(ctx, baseline), s.prepareTarget(ctx, candidate)
	if !a.fatal && !b.fatal {
		for _, t := range []*targetRun{a, b} {
			if err := s.prime(ctx, t); err != nil {
				t.fail("priming", err)
			}
		}
		for rep := 0; rep < s.profile.Repetitions; rep++ {
			first, second := a, b
			if rep%2 == 1 {
				first, second = b, a
			}
			s.repetition(ctx, first, rep)
			s.repetition(ctx, second, rep)
		}
		s.qualityStage(ctx, a)
		s.qualityStage(ctx, b)
		s.checkSupplied(a)
	}
	if ctx.Err() != nil {
		a.fail("run", context.Canceled)
		b.fail("run", context.Canceled)
	}
	return s.result(a, runID+"-baseline", started), s.result(b, runID+"-candidate", started)
}

// marshal renders a result or comparison the same way everywhere.
func marshal(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
