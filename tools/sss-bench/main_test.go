package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

func runCLI(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestInvalidCommandLinesExitTwo(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		args    []string
		message string
	}{
		{"no command", nil, "Commands:"},
		{"unknown command", []string{"benchmark"}, "unknown command"},
		{"unknown flag", []string{"run", "--nope"}, "-nope"},
		{"stray argument", []string{"run", "quick"}, "unexpected argument"},
		{"unknown profile", []string{"run", "--profile", "huge"}, "unknown profile"},
		{"zero repetitions", []string{"run", "--reps", "0"}, "--reps"},
		{"too many repetitions", []string{"run", "--reps", "31"}, "--reps"},
		{"negative passes", []string{"run", "--passes", "-2"}, "--passes"},
		{"tiny fixture", []string{"run", "--titles", "5"}, "--titles"},
		{"negative settle", []string{"run", "--settle", "-5s"}, "--settle"},
		{"short sustain", []string{"run", "--sustain", "1s"}, "--sustain"},
		{"missing catalogue", []string{"run", "--catalog", "no-such-file"}, "--catalog"},
		{"titles with catalogue", []string{"run", "--catalog", file, "--titles", "2000"}, "--titles"},
		{"missing IMDb folder", []string{"run", "--profile", "full", "--imdb-dir", "no-such-dir"}, "--imdb-dir"},
		{"IMDb folder without full", []string{"run", "--imdb-dir", t.TempDir()}, "full profile"},
		{"source and binary", []string{"run", "--source", ".", "--binary", file}, "not both"},
		{"missing binary", []string{"run", "--binary", "no-such-file"}, "--binary"},
		{"source without the app", []string{"run", "--source", t.TempDir()}, "not a Smart Search source tree"},
		{"ab without baseline", []string{"ab"}, "--baseline"},
		{"ab of one folder", []string{"ab", "--baseline", "../..", "--candidate", "../.."}, "same folder"},
		{"bad alpha", []string{"ab", "--baseline", "../..", "--alpha", "2"}, "--alpha"},
		{"compare without files", []string{"compare"}, "--baseline"},
		{"compare bad alpha", []string{"compare", "--baseline", "a", "--candidate", "b", "--alpha", "0"}, "--alpha"},
		{"report without input", []string{"report"}, "--input"},
	} {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(test.args...)
			if code != exitInput {
				t.Fatalf("exit %d, stderr %s", code, stderr)
			}
			if !strings.Contains(stdout+stderr, test.message) {
				t.Fatalf("no %q in %s", test.message, stderr)
			}
		})
	}
}

func TestHelpAndVersionSucceed(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"version"}, {"run", "--help"}} {
		if code, _, _ := runCLI(args...); code != exitOK {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

func TestRepositoryIsRequired(t *testing.T) {
	if _, err := findRepoRoot(t.TempDir()); err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Fatalf("%v", err)
	}
	if root, err := findRepoRoot("."); err != nil || !strings.HasSuffix(filepath.ToSlash(root), "/") && filepath.Base(root) == "sss-bench" {
		t.Fatalf("%s %v", root, err)
	}
}

func saveResult(t *testing.T, r *Result) string {
	t.Helper()
	data, err := marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResultRoundTrips(t *testing.T) {
	original := sampleResult(series(1, 6), series(50*mib, 6))
	original.Quality = []SuiteResult{suiteOf(1, 2)}
	loaded, err := readResult(saveResult(t, original))
	if err != nil {
		t.Fatal(err)
	}
	first, _ := marshal(original)
	second, _ := marshal(loaded)
	if !bytes.Equal(first, second) {
		t.Fatal("a saved result changed when read back")
	}
}

func TestSavedResultsAreValidated(t *testing.T) {
	broken := map[string]func(*Result){
		"another tool's file":   func(r *Result) { r.Schema = "something-else" },
		"a future schema":       func(r *Result) { r.SchemaVersion = 99 },
		"unknown status":        func(r *Result) { r.Status = "fine" },
		"no workload":           func(r *Result) { r.Workload.ID = "" },
		"no environment":        func(r *Result) { r.Environment.OS = "" },
		"duplicate metric":      func(r *Result) { r.Metrics[1].ID = r.Metrics[0].ID },
		"sample count mismatch": func(r *Result) { r.Metrics[0].N = 99 },
		"value missing":         func(r *Result) { r.Metrics[0].Value = nil },
		"unknown metric status": func(r *Result) { r.Metrics[0].Status = "maybe" },
	}
	for name, breakIt := range broken {
		t.Run(name, func(t *testing.T) {
			r := sampleResult(series(1, 6), series(50*mib, 6))
			breakIt(r)
			path := saveResult(t, r)
			if _, err := readResult(path); err == nil {
				t.Fatal("accepted")
			}
			if code, _, _ := runCLI("compare", "--baseline", path, "--candidate", path); code != exitInput {
				t.Fatalf("compare exit %d", code)
			}
		})
	}
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte(`{"schema":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI("report", "--input", corrupt); code != exitInput || !strings.Contains(stderr, "not valid JSON") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if code, _, _ := runCLI("report", "--input", filepath.Join(dir, "missing.json")); code != exitInput {
		t.Fatalf("exit %d", code)
	}
}

func TestCompareExitCodes(t *testing.T) {
	base := saveResult(t, sampleResult(series(1, 6), series(50*mib, 6)))
	slower := saveResult(t, sampleResult(series(2, 6), series(50*mib, 6)))
	same := saveResult(t, sampleResult(series(1, 6), series(50*mib, 6)))
	other := sampleResult(series(1, 6), series(50*mib, 6))
	other.Catalogue.ContentSHA256 = "catalogue-b"
	otherCatalogue := saveResult(t, other)
	partial := sampleResult(series(1, 6), series(50*mib, 6))
	partial.Status = statusPartial
	incomplete := saveResult(t, partial)
	for name, test := range map[string]struct {
		candidate string
		want      int
	}{"no change": {same, exitOK}, "regression": {slower, exitRegression}, "different catalogue": {otherCatalogue, exitNotComparable}, "partial run": {incomplete, exitIncomplete}} {
		t.Run(name, func(t *testing.T) {
			if code, _, stderr := runCLI("compare", "--baseline", base, "--candidate", test.candidate); code != test.want {
				t.Fatalf("exit %d, want %d: %s", code, test.want, stderr)
			}
		})
	}
}

func TestCompareJSONIsMachineReadable(t *testing.T) {
	base := saveResult(t, sampleResult(series(1, 6), series(50*mib, 6)))
	slower := saveResult(t, sampleResult(series(2, 6), series(50*mib, 6)))
	dir := t.TempDir()
	code, stdout, _ := runCLI("compare", "--baseline", base, "--candidate", slower, "--json", "--out", filepath.Join(dir, "c.json"), "--report", filepath.Join(dir, "c.md"))
	if code != exitRegression {
		t.Fatalf("exit %d", code)
	}
	var c Comparison
	if err := json.Unmarshal([]byte(stdout), &c); err != nil {
		t.Fatal(err)
	}
	if c.Schema != schemaComparison || c.SchemaVersion != schemaVersion || c.Verdict != overallRegressed || c.Summary.Regressed != 1 {
		t.Fatalf("%+v", c)
	}
	for _, name := range []string{"c.json", "c.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Error(err)
		}
	}
}

func TestReportStatesDenominatorsAndLimits(t *testing.T) {
	r := sampleResult(series(1, 6), series(50*mib, 6))
	r.Quality = []SuiteResult{suiteOf(1, 1, 3, 0)}
	r.Metrics = append(r.Metrics, Metric{ID: "memory.app.private_idle", Group: groupMemory, Unit: "bytes", Status: metricUnavailable, Reason: "this platform has no equivalent counter"})
	text := renderRun(r)
	for _, expected := range []string{"2/4 (50.0%)", "3/4 (75.0%)", "Synthetic catalogue", "not accuracy on real titles", "Not measured here", "memory.app.private_idle", "not equivalent"} {
		if !strings.Contains(text, expected) {
			t.Errorf("no %q in\n%s", expected, text)
		}
	}
}

func TestBenchOutputIsGoBenchmarkFormat(t *testing.T) {
	r := sampleResult([]float64{1.5, 1.6}, []float64{1048576, 1048577})
	text := renderBench(r)
	line := regexp.MustCompile(`(?m)^BenchmarkSSS/\S+ 1 \S+ \S+$`)
	if got := len(line.FindAllString(text, -1)); got != 4 {
		t.Fatalf("%d benchmark lines in\n%s", got, text)
	}
	if strings.Contains(text, "binary.app_bytes") {
		t.Fatal("deterministic metrics have nothing for benchstat to test")
	}
}

func TestCollectorNeverDropsASelectedMetric(t *testing.T) {
	p, _ := profileByName("quick")
	c := newCollector()
	c.add("startup.ready_ms", 10)
	c.add("startup.ready_ms", 12)
	c.unavailable("memory.app.private_idle", "no such counter here")
	c.failed("memory.app.idle", "the counter read as zero")
	metrics := map[string]Metric{}
	for _, m := range c.metrics(p, 4) {
		metrics[m.ID] = m
	}
	if m := metrics["startup.ready_ms"]; m.Status != metricOK || m.N != 2 || !strings.Contains(strings.Join(m.Flags, ","), "incomplete") {
		t.Fatalf("%+v", m)
	}
	if m := metrics["memory.app.private_idle"]; m.Status != metricUnavailable || m.Reason == "" {
		t.Fatalf("%+v", m)
	}
	if m := metrics["memory.app.idle"]; m.Status != metricFailed {
		t.Fatalf("%+v", m)
	}
	if m := metrics["startup.first_search_ms"]; m.Status != metricFailed {
		t.Fatalf("a metric with no value and no reason must fail: %+v", m)
	}
	if _, ok := metrics["memory.app.sustained_end"]; ok {
		t.Fatal("quick does not run sustained searching")
	}
	if _, ok := metrics["search.http.served_rps.c2"]; ok {
		t.Fatal("quick does not measure throughput")
	}
}

func TestBusyShare(t *testing.T) {
	share, ok := busyShare(cpuTimes{busy: 100, total: 1000}, cpuTimes{busy: 300, total: 2000})
	if !ok || !near(share, 20) {
		t.Fatalf("%v %v", share, ok)
	}
	if _, ok := busyShare(cpuTimes{total: 5}, cpuTimes{total: 5}); ok {
		t.Fatal("no time passing must not read as an idle machine")
	}
	if times, ok := systemCPU(); ok && (times.total <= 0 || times.busy > times.total) {
		t.Fatalf("%+v", times)
	}
}

func TestParseProcStat(t *testing.T) {
	// user nice system idle iowait irq softirq steal guest guest_nice
	times, ok := parseProcStat("cpu  100 10 50 700 40 5 5 90 999 999\ncpu0 1 2 3 4\n")
	if !ok || times.total != 10*time.Second || times.busy != 2600*time.Millisecond {
		t.Fatalf("%+v %v", times, ok)
	}
	for _, bad := range []string{"", "intr 1 2 3", "cpu 1 2", "cpu a b c d e"} {
		if _, ok := parseProcStat(bad); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestMetricFlags(t *testing.T) {
	noisy := Metric{ID: "startup.ready_ms"}
	fill(&noisy, []float64{1, 10, 1, 10})
	if !strings.Contains(strings.Join(noisy.Flags, ","), "noisy") {
		t.Fatalf("%+v", noisy.Flags)
	}
	few := Metric{ID: "search.http.warm.p99_ms", EachRunCount: 50}
	fill(&few, []float64{1, 1, 1, 1})
	if !strings.Contains(strings.Join(few.Flags, ","), "low_sample_count") {
		t.Fatalf("%+v", few.Flags)
	}
	enough := Metric{ID: "search.http.warm.p99_ms", EachRunCount: 864}
	fill(&enough, []float64{1, 1, 1, 1})
	if len(enough.Flags) != 0 {
		t.Fatalf("%+v", enough.Flags)
	}
}

func TestEveryPrimaryMetricExists(t *testing.T) {
	known := map[string]bool{}
	for _, d := range metricDefs() {
		known[d.id] = true
	}
	for _, id := range primaryMetrics {
		if !known[id] {
			t.Errorf("primary metric %s is not defined", id)
		}
	}
}

func TestMetricDefinitionsAreConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range metricDefs() {
		switch {
		case seen[d.id]:
			t.Errorf("%s defined twice", d.id)
		case d.definition == "" || d.unit == "" || d.group == "":
			t.Errorf("%s is incomplete", d.id)
		case d.better != "lower" && d.better != "higher" && d.better != "none":
			t.Errorf("%s: better %q", d.id, d.better)
		}
		seen[d.id] = true
	}
}

func TestProfilesAreValidAndOrdered(t *testing.T) {
	list := profiles()
	if len(list) != 3 {
		t.Fatalf("%d profiles", len(list))
	}
	for i, p := range list {
		if p.tier != i || p.Passes < 1 || len(p.Suites) == 0 || p.Cost == "" {
			t.Errorf("%+v", p)
		}
		// A profile's default must be able to reach a verdict on its primary metrics.
		family := 0
		for _, d := range metricDefs() {
			if d.primary && inScope(d, p) {
				family++
			}
		}
		if best := minAchievableP(p.Repetitions, p.Repetitions) * float64(family); best >= defaultAlpha {
			t.Errorf("%s: %d repetitions cannot reach significance across %d primary metrics", p.Name, p.Repetitions, family)
		}
		if p.Sustain > 0 && p.Sustain/sustainSampler < 6 {
			t.Errorf("%s: too little sustained searching to see a trend", p.Name)
		}
	}
	if list[0].CatalogOps || list[1].CatalogOps || !list[2].CatalogOps {
		t.Error("catalogue operations belong to full only")
	}
}

// requireEndToEnd skips tests that build and run the application unless asked,
// like the repository's other opt-in integration tests.
func requireEndToEnd(t *testing.T) {
	t.Helper()
	if os.Getenv("SSS_BENCH_E2E") == "" {
		t.Skip("set SSS_BENCH_E2E=1 to build and run the application")
	}
}

// TestRunEndToEnd measures the real application and probes on this platform with
// minimal settings: it validates the pipeline, and its numbers mean nothing.
func TestRunEndToEnd(t *testing.T) {
	requireEndToEnd(t)
	// Point the user's folders at a sentinel, so any write outside the workspace
	// would be seen. The workspace goes in a folder this test watches too.
	sentinel := t.TempDir()
	// The Go toolchain legitimately uses the real build cache, so pin it first.
	for _, name := range []string{"GOCACHE", "GOPATH", "GOMODCACHE"} {
		value, err := runGo(t.Context(), ".", "env", name)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv(name, value)
	}
	for _, name := range []string{"APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME"} {
		t.Setenv(name, sentinel)
	}
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", sentinel)
	}
	work, out := t.TempDir(), t.TempDir()
	supplied := filepath.Join(out, "catalog.json")
	data, err := os.ReadFile("../../testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(supplied, data, 0o600); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(out, "result")
	code, stdout, stderr := runCLI("run", "--reps", "2", "--passes", "1", "--titles", "2000", "--settle", "0s", "--work-dir", work, "--out", result, "--json", "--quiet", "--label", "end-to-end")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	var r Result
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatal(err)
	}
	if r.Status != statusComplete || len(r.Errors) != 0 || r.Label != "end-to-end" {
		t.Fatalf("%s: %+v", r.Status, r.Errors)
	}
	metrics := index(&r)
	for _, id := range []string{"startup.ready_ms", "search.http.warm.p95_ms", "search.engine.warm.p95_ms", "catalog.index_ms", "memory.engine.heap_after_index"} {
		m := metrics[id]
		if m == nil || m.Status != metricOK || *m.Value <= 0 {
			t.Errorf("%s: %+v", id, m)
		}
	}
	if m := metrics["catalog.titles"]; *m.Value != 2000 {
		t.Errorf("indexed %v titles", *m.Value)
	}
	if m := metrics["search.http.errors"]; *m.Value != 0 {
		t.Errorf("%v failed requests", *m.Value)
	}
	if r.Environment.ClockResolutionNS > 10_000 {
		t.Errorf("timer resolution %d ns would blur a search", r.Environment.ClockResolutionNS)
	}
	if suite := r.Quality[0]; suite.Path != pathEngine || suite.Totals.Ranked == 0 || suite.Totals.Absent != 0 {
		t.Errorf("%+v", suite.Totals)
	}
	for _, ext := range []string{".json", ".md", ".bench"} {
		if _, err := os.Stat(result + ext); err != nil {
			t.Error(err)
		}
	}

	// Everything was cleaned up, and nothing reached the user's folders.
	if left, _ := os.ReadDir(work); len(left) != 0 {
		t.Errorf("left in the work folder: %v", left)
	}
	// The Go toolchain keeps its own settings in a "go" folder under the user's config
	// folder, which on macOS is Library/Application Support. Check every file rather
	// than the top level: the installed helper's data lives in that same parent.
	err = filepath.WalkDir(sentinel, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(sentinel, path)
		if err != nil {
			return err
		}
		if !slices.Contains(strings.Split(filepath.ToSlash(relative), "/"), "go") {
			t.Errorf("wrote to the user's folders: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The report must be safe to paste publicly.
	report, err := os.ReadFile(result + ".md")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(result + ".json")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	repo, _ := findRepoRoot(".")
	for _, private := range []string{work, sentinel, repo, home} {
		if private != "" && (bytes.Contains(report, []byte(private)) || bytes.Contains(saved, []byte(strings.ReplaceAll(private, `\`, `\\`)))) {
			t.Errorf("a local path leaked into the report: %s", private)
		}
	}

	// A second run on the same source compares as the same build.
	again := filepath.Join(out, "again")
	if code, _, stderr := runCLI("run", "--reps", "2", "--passes", "1", "--titles", "2000", "--settle", "0s", "--out", again, "--quiet"); code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	code, stdout, stderr = runCLI("compare", "--baseline", result+".json", "--candidate", again+".json", "--json")
	if code != exitOK {
		t.Fatalf("exit %d\n%s\n%s", code, stderr, stdout)
	}
	var c Comparison
	if err := json.Unmarshal([]byte(stdout), &c); err != nil {
		t.Fatal(err)
	}
	if c.Summary.Regressed != 0 || c.Summary.QualityRegressions != 0 {
		t.Fatalf("%+v", c.Summary)
	}

	// A supplied catalogue is measured and left exactly as it was.
	before, _ := os.Stat(supplied)
	suppliedResult := filepath.Join(out, "supplied")
	code, stdout, stderr = runCLI("run", "--reps", "1", "--passes", "1", "--settle", "0s", "--catalog", supplied, "--out", suppliedResult, "--json", "--quiet")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	var s Result
	if err := json.Unmarshal([]byte(stdout), &s); err != nil {
		t.Fatal(err)
	}
	if s.Catalogue.Kind != "supplied" || s.Catalogue.Titles != 19 || s.Catalogue.Name != "catalog.json" {
		t.Fatalf("%+v", s.Catalogue)
	}
	after, _ := os.Stat(supplied)
	current, _ := os.ReadFile(supplied)
	if !bytes.Equal(current, data) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("the supplied catalogue changed")
	}
}

func TestPrebuiltBinaryHasHonestGaps(t *testing.T) {
	requireEndToEnd(t)
	work := t.TempDir()
	binary := filepath.Join(work, executableName("prebuilt"))
	repo, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runGo(t.Context(), repo, "build", "-trimpath", "-o", binary, "./cmd/smart-search"); err != nil {
		t.Fatal(out)
	}
	result := filepath.Join(t.TempDir(), "result")
	code, stdout, stderr := runCLI("run", "--binary", binary, "--reps", "1", "--passes", "1", "--titles", "2000", "--settle", "0s", "--out", result, "--json", "--quiet")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	var r Result
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatal(err)
	}
	metrics := index(&r)
	if m := metrics["search.engine.warm.p95_ms"]; m.Status != metricUnavailable || !strings.Contains(m.Reason, "--binary") {
		t.Fatalf("%+v", m)
	}
	if m := metrics["search.http.warm.p95_ms"]; m.Status != metricOK {
		t.Fatalf("%+v", m)
	}
	if r.Quality[0].Path != pathHTTP {
		t.Fatalf("quality came from %s", r.Quality[0].Path)
	}
}

func TestCorruptCatalogueIsAnInputError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json.gz")
	if err := os.WriteFile(path, []byte("\x1f\x8bnot really gzip"), 0o600); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	code, _, stderr := runCLI("run", "--catalog", path, "--work-dir", work, "--out", filepath.Join(t.TempDir(), "r"), "--quiet")
	if code != exitInput || !strings.Contains(stderr, "could not be read") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if left, _ := os.ReadDir(work); len(left) != 0 {
		t.Fatal("a rejected run left files behind")
	}
}

func TestIMDbFolderMustHoldTheFiles(t *testing.T) {
	code, _, stderr := runCLI("run", "--profile", "full", "--imdb-dir", t.TempDir(), "--out", filepath.Join(t.TempDir(), "r"), "--quiet")
	if code != exitInput || !strings.Contains(stderr, "never downloads") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestFixtureUsesNoRealCatalogueData(t *testing.T) {
	c := buildFixture(2000, allSuites(t), fixtureLanguage)
	if !strings.Contains(c.Source, "not IMDb data") || !c.BuiltAt.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("%q %v", c.Source, c.BuiltAt)
	}
	var _ catalog.Catalog = *c
}
