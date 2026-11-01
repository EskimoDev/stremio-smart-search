package main

import (
	"strings"
	"testing"
	"time"
)

// sampleResult builds a valid result with one latency, one memory and one count
// metric, for the comparison rules to be tested without running anything.
func sampleResult(latency, memory []float64) *Result {
	r := &Result{
		Schema: schemaResult, SchemaVersion: schemaVersion, Status: statusComplete, RunID: "run", StartedAt: time.Unix(0, 0).UTC(),
		Harness:     HarnessInfo{Version: harnessVersion, MeasurementVersion: measurementVersion, Go: "go1.27.0"},
		Target:      TargetInfo{Kind: "source", Commit: "aaaaaaaaaaaa", Go: "go1.27.0", AppVersion: "0.1.0-dev", BinarySHA256: "binary-a"},
		Environment: Environment{OS: "windows", Arch: "amd64", CPUModel: "Test CPU", LogicalCPUs: 8, MemoryTotalBytes: 1 << 34, HardwareID: "hardware-a", ClockResolutionNS: 100},
		Catalogue:   CatalogueInfo{Kind: "synthetic", Name: "fixture-20000", ContentSHA256: "catalogue-a", Titles: 3},
		Workload:    WorkloadInfo{ID: "workload-a", Version: workloadVersion, Profile: "quick", Repetitions: len(latency), Queries: 3},
		Errors:      []Problem{},
		Warnings:    []string{},
	}
	add := func(id string, values []float64) {
		c := newCollector()
		d := c.defs[id]
		m := Metric{ID: id, Group: d.group, Unit: d.unit, Better: d.better, Definition: d.definition, Counter: d.counter, Deterministic: d.deterministic, Status: metricOK}
		fill(&m, values)
		r.Metrics = append(r.Metrics, m)
	}
	add("search.http.warm.p95_ms", latency)
	add("memory.app.idle", memory)
	add("binary.app_bytes", []float64{8_000_000})
	return r
}

func series(base float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = base + 0.001*base*float64(i)
	}
	return out
}

func metricOf(t *testing.T, c *Comparison, id string) MetricComparison {
	t.Helper()
	for _, m := range c.Metrics {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("no metric %s", id)
	return MetricComparison{}
}

const latencyID = "search.http.warm.p95_ms"

func TestRegressionNeedsEvidenceAndSize(t *testing.T) {
	baseline := sampleResult(series(1, 6), series(50*mib, 6))
	for _, test := range []struct {
		name    string
		latency []float64
		want    string
	}{
		{"large and clearly separated", series(1.5, 6), verdictRegressed},
		{"improved", series(0.5, 6), verdictImproved},
		{"separated but under the practical floor", series(1.02, 6), verdictBelow},
		{"overlapping samples", []float64{0.9, 1.3, 1.0, 1.2, 0.95, 1.25}, verdictNoChange},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := compareResults(baseline, sampleResult(test.latency, series(50*mib, 6)), defaultAlpha)
			if got := metricOf(t, c, latencyID); got.Verdict != test.want {
				t.Fatalf("verdict %s (change %v, p %v), want %s", got.Verdict, *got.ChangePct, *got.P, test.want)
			}
		})
	}
}

func TestTinyAbsoluteChangeIsNotARegression(t *testing.T) {
	// 20% is large, but 2 microseconds on a 10-microsecond metric is timer noise.
	c := compareResults(sampleResult(series(0.01, 6), series(50*mib, 6)), sampleResult(series(0.012, 6), series(50*mib, 6)), defaultAlpha)
	if got := metricOf(t, c, latencyID); got.Verdict != verdictBelow {
		t.Fatalf("verdict %s", got.Verdict)
	}
}

func TestFewerSamplesThanTheFamilyAllowsGiveNoVerdict(t *testing.T) {
	// Four against four can reach 0.029, which fails once there are two primary
	// metrics to correct for. Saying "no change" there would be a false reassurance.
	c := compareResults(sampleResult(series(1, 4), series(50*mib, 4)), sampleResult(series(9, 4), series(50*mib, 4)), defaultAlpha)
	if got := metricOf(t, c, latencyID); got.Verdict != verdictInsufficient || !strings.Contains(got.Reason, "primary metrics") {
		t.Fatalf("verdict %s: %s", got.Verdict, got.Reason)
	}
}

func TestSinglePrimaryMetricOutOfManyNeedsToSurviveCorrection(t *testing.T) {
	// One clean separation at p = 0.0022 is not much evidence among thirty metrics:
	// by chance alone one of them could look like that.
	var metrics []MetricComparison
	for i := 0; i < 30; i++ {
		p := 0.5
		metrics = append(metrics, MetricComparison{ID: "m", Primary: true, P: &p, NBaseline: 8, NCandidate: 8})
	}
	strong := 0.0022
	metrics[0].P = &strong
	metrics[0].worse = true
	var summary ComparisonSummary
	settle(metrics, defaultAlpha, &summary)
	if metrics[0].Verdict != verdictNoChange || !strings.Contains(metrics[0].Reason, "correcting") {
		t.Fatalf("%s: %s", metrics[0].Verdict, metrics[0].Reason)
	}
	// The same p-value alone in its family is a finding.
	single := []MetricComparison{{ID: "m", Primary: true, P: &strong, worse: true, NBaseline: 6, NCandidate: 6}}
	settle(single, defaultAlpha, &summary)
	if single[0].Verdict != verdictRegressed {
		t.Fatalf("%s", single[0].Verdict)
	}
}

func TestSecondaryMetricsAreHintsAndDoNotDecideTheVerdict(t *testing.T) {
	baseline, candidate := sampleResult(series(1, 6), series(50*mib, 6)), sampleResult(series(1, 6), series(50*mib, 6))
	add := func(r *Result, value float64) {
		m := Metric{ID: "search.http.warm.mean_ms", Group: groupHTTP, Unit: "ms", Better: "lower", Status: metricOK}
		fill(&m, series(value, 6))
		r.Metrics = append(r.Metrics, m)
	}
	add(baseline, 1)
	add(candidate, 2)
	c := compareResults(baseline, candidate, defaultAlpha)
	if got := metricOf(t, c, "search.http.warm.mean_ms"); got.Verdict != verdictRegressed || got.Primary {
		t.Fatalf("%+v", got)
	}
	if c.Verdict != overallNoChange || c.Summary.Regressed != 0 || c.Summary.SecondaryRegressed != 1 {
		t.Fatalf("verdict %s, summary %+v", c.Verdict, c.Summary)
	}
	if !strings.Contains(renderComparison(c), "Secondary hints") {
		t.Fatal("the report should show the hint")
	}
}

func TestFewSamplesGiveNoVerdict(t *testing.T) {
	c := compareResults(sampleResult(series(1, 3), series(50*mib, 3)), sampleResult(series(9, 3), series(50*mib, 3)), defaultAlpha)
	got := metricOf(t, c, latencyID)
	if got.Verdict != verdictInsufficient || got.P != nil {
		t.Fatalf("verdict %s, p %v", got.Verdict, got.P)
	}
	if got.ChangePct == nil {
		t.Fatal("the change should still be shown")
	}
	if c.Verdict != overallNoChange || c.Summary.Insufficient == 0 {
		t.Fatalf("overall %s, summary %+v", c.Verdict, c.Summary)
	}
}

func TestIncompatibleResultsMakeNoClaim(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*Result)
		verdict string
	}{
		{"hardware", func(r *Result) { r.Environment.HardwareID = "hardware-b" }, overallIncomparable},
		{"operating system", func(r *Result) { r.Environment.OS = "linux" }, overallIncomparable},
		{"architecture", func(r *Result) { r.Environment.Arch = "arm64" }, overallIncomparable},
		{"workload", func(r *Result) { r.Workload.ID = "workload-b" }, overallIncomparable},
		{"catalogue", func(r *Result) { r.Catalogue.ContentSHA256 = "catalogue-b" }, overallIncomparable},
		{"measurement definitions", func(r *Result) { r.Harness.MeasurementVersion = 99 }, overallIncomparable},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := sampleResult(series(9, 6), series(90*mib, 6))
			test.change(candidate)
			c := compareResults(sampleResult(series(1, 6), series(50*mib, 6)), candidate, defaultAlpha)
			got := metricOf(t, c, latencyID)
			if got.Verdict != verdictIncomparable || got.ChangePct != nil {
				t.Fatalf("a 9x difference on different %s gave verdict %s, change %v", test.name, got.Verdict, got.ChangePct)
			}
			if c.Summary.Regressed != 0 {
				t.Fatalf("claimed a regression: %+v", c.Summary)
			}
			if c.Verdict != test.verdict {
				t.Fatalf("overall verdict %s, want %s", c.Verdict, test.verdict)
			}
			blocking := false
			for _, check := range c.Compatibility {
				blocking = blocking || check.Status == "blocking"
			}
			if !blocking {
				t.Fatal("no blocking check was reported")
			}
		})
	}
}

func TestQualityStillComparesAcrossHardware(t *testing.T) {
	// Ranks are deterministic, so a different machine blocks timings, not quality.
	baseline, candidate := sampleResult(series(1, 6), series(50*mib, 6)), sampleResult(series(9, 6), series(50*mib, 6))
	baseline.Quality = []SuiteResult{suiteOf(1, 1)}
	candidate.Quality = []SuiteResult{suiteOf(1, 3)}
	candidate.Environment.HardwareID = "hardware-b"
	c := compareResults(baseline, candidate, defaultAlpha)
	if c.Comparable != "partial" || c.Quality[0].Status != "compared" || c.Verdict != overallRegressed {
		t.Fatalf("comparable %s, quality %s, verdict %s", c.Comparable, c.Quality[0].Status, c.Verdict)
	}
	if got := metricOf(t, c, latencyID); got.Verdict != verdictIncomparable {
		t.Fatalf("timing verdict %s", got.Verdict)
	}
}

func TestDeterministicMetricsSurviveHardwareDifferences(t *testing.T) {
	candidate := sampleResult(series(1, 6), series(50*mib, 6))
	candidate.Environment.HardwareID = "hardware-b"
	candidate.Metrics[2].Samples[0] = 9_000_000
	value := 9_000_000.0
	candidate.Metrics[2].Value = &value
	c := compareResults(sampleResult(series(1, 6), series(50*mib, 6)), candidate, defaultAlpha)
	if got := metricOf(t, c, "binary.app_bytes"); got.Verdict != verdictRegressed {
		t.Fatalf("binary size verdict %s", got.Verdict)
	}
}

func TestDifferentMemoryCountersAreNotCompared(t *testing.T) {
	candidate := sampleResult(series(1, 6), series(90*mib, 6))
	candidate.Metrics[1].Counter = "linux.VmRSS"
	c := compareResults(sampleResult(series(1, 6), series(50*mib, 6)), candidate, defaultAlpha)
	got := metricOf(t, c, "memory.app.idle")
	if got.Verdict != verdictIncomparable || !strings.Contains(got.Reason, "linux.VmRSS") {
		t.Fatalf("verdict %s, reason %q", got.Verdict, got.Reason)
	}
}

func TestMissingAndFailedMetricsAreSurfaced(t *testing.T) {
	baseline := sampleResult(series(1, 6), series(50*mib, 6))
	candidate := sampleResult(series(1, 6), series(50*mib, 6))
	candidate.Metrics = candidate.Metrics[:1]
	candidate.Status = statusPartial
	baseline.Metrics[0].Status, baseline.Metrics[0].Reason = metricFailed, "broken"
	c := compareResults(baseline, candidate, defaultAlpha)
	if got := metricOf(t, c, "memory.app.idle"); got.Verdict != verdictMissing {
		t.Fatalf("missing metric verdict %s", got.Verdict)
	}
	if got := metricOf(t, c, latencyID); got.Verdict != verdictFailed {
		t.Fatalf("failed metric verdict %s", got.Verdict)
	}
	if !c.Incomplete || c.Summary.Missing == 0 || c.Summary.Failed == 0 {
		t.Fatalf("incomplete %v, summary %+v", c.Incomplete, c.Summary)
	}
}

func suiteOf(ranks ...int) SuiteResult {
	suite := SuiteResult{Name: "cases", SHA256: "cases-a", Path: pathEngine}
	all := &tally{}
	for i, rank := range ranks {
		name := string(rune('a' + i))
		suite.Cases = append(suite.Cases, CaseResult{Name: name, Query: name, Kind: "movie", Category: "exact", Status: "scored", Rank: rank})
		r := qualityResult(rank)
		all.add(r, false)
	}
	suite.Categories = []CategoryResult{all.row("all")}
	suite.Totals = all.counts
	return suite
}

func TestQualityUsesPercentagePointsAndNamesCases(t *testing.T) {
	baseline, candidate := sampleResult(series(1, 6), series(50*mib, 6)), sampleResult(series(1, 6), series(50*mib, 6))
	baseline.Quality = []SuiteResult{suiteOf(1, 1, 1, 1)}
	candidate.Quality = []SuiteResult{suiteOf(1, 1, 1, 2)}
	c := compareResults(baseline, candidate, defaultAlpha)
	suite := c.Quality[0]
	if suite.Status != "compared" || len(suite.Regressions) != 1 || suite.Regressions[0].Name != "d" {
		t.Fatalf("%+v", suite)
	}
	row := suite.Categories[0]
	if *row.Top1.ChangePoints != -25 || row.Top1.Baseline != "4/4" || row.Top1.Candidate != "3/4" {
		t.Fatalf("top-1 %+v", row.Top1)
	}
	if *row.Top5.ChangePoints != 0 {
		t.Fatalf("top-5 %+v", row.Top5)
	}
	if c.Verdict != overallRegressed {
		t.Fatalf("overall %s", c.Verdict)
	}
	if !strings.Contains(renderComparison(c), "(-25.0 pp)") {
		t.Fatal("the report should state percentage points")
	}
}

func TestQualitySuiteChangeBlocksTheComparison(t *testing.T) {
	baseline, candidate := sampleResult(series(1, 6), series(50*mib, 6)), sampleResult(series(1, 6), series(50*mib, 6))
	baseline.Quality = []SuiteResult{suiteOf(1, 1)}
	changed := suiteOf(5, 5)
	changed.SHA256 = "cases-b"
	candidate.Quality = []SuiteResult{changed}
	c := compareResults(baseline, candidate, defaultAlpha)
	if c.Quality[0].Status != "not_comparable" || c.Summary.QualityRegressions != 0 {
		t.Fatalf("%+v", c.Quality[0])
	}
}

func TestQualityPathChangeBlocksTheComparison(t *testing.T) {
	baseline, candidate := sampleResult(series(1, 6), series(50*mib, 6)), sampleResult(series(1, 6), series(50*mib, 6))
	baseline.Quality = []SuiteResult{suiteOf(1, 1)}
	http := suiteOf(1, 1)
	http.Path = pathHTTP
	candidate.Quality = []SuiteResult{http}
	if c := compareResults(baseline, candidate, defaultAlpha); c.Quality[0].Status != "not_comparable" {
		t.Fatalf("%+v", c.Quality[0])
	}
}

func TestAbsentCasesChangeCoverageNotAccuracy(t *testing.T) {
	baseline, candidate := sampleResult(series(1, 6), series(50*mib, 6)), sampleResult(series(1, 6), series(50*mib, 6))
	baseline.Quality = []SuiteResult{suiteOf(1, 1, 1, 1)}
	after := suiteOf(1, 1, 1)
	after.Categories[0].Counts.Absent = 1
	after.Categories[0].Counts.Total = 4
	candidate.Quality = []SuiteResult{after}
	row := compareResults(baseline, candidate, defaultAlpha).Quality[0].Categories[0]
	if *row.Top1.ChangePoints != 0 || *row.Coverage.ChangePoints != -25 {
		t.Fatalf("top-1 %+v coverage %+v", row.Top1, row.Coverage)
	}
}

func TestSequentialAndIdenticalRunsAreNoted(t *testing.T) {
	c := compareResults(sampleResult(series(1, 6), series(50*mib, 6)), sampleResult(series(1, 6), series(50*mib, 6)), defaultAlpha)
	text := strings.Join(c.Notes, " ")
	if !strings.Contains(text, "one after the other") || !strings.Contains(text, "same binary") {
		t.Fatalf("notes: %q", text)
	}
}
