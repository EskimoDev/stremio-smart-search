package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/EskimoDev/stremio-smart-search/internal/quality"
)

// defaultAlpha is the significance level, benchstat's default.
const defaultAlpha = 0.05

// Metric verdicts.
const (
	verdictRegressed    = "regressed"
	verdictImproved     = "improved"
	verdictNoChange     = "no_significant_change"
	verdictBelow        = "below_threshold" // real but too small to matter
	verdictChanged      = "changed"         // a count with no better direction
	verdictInsufficient = "insufficient_samples"
	verdictMissing      = "missing"
	verdictFailed       = "failed"
	verdictUnavailable  = "unavailable"
	verdictIncomparable = "not_comparable"
)

// Overall verdicts.
const (
	overallRegressed    = "regressed"
	overallImproved     = "improved"
	overallNoChange     = "no_significant_change"
	overallIncomparable = "not_comparable"
)

// Comparison is the versioned output of compare.
type Comparison struct {
	Schema        string             `json:"schema"`
	SchemaVersion int                `json:"schema_version"`
	Verdict       string             `json:"verdict"`
	Comparable    string             `json:"comparable"` // full, partial or none
	Incomplete    bool               `json:"incomplete"`
	Alpha         float64            `json:"alpha"`
	Baseline      RunRef             `json:"baseline"`
	Candidate     RunRef             `json:"candidate"`
	Compatibility []CompatCheck      `json:"compatibility"`
	Summary       ComparisonSummary  `json:"summary"`
	Metrics       []MetricComparison `json:"metrics"`
	Quality       []SuiteComparison  `json:"quality"`
	Notes         []string           `json:"notes"`
}

// RunRef identifies one side of a comparison.
type RunRef struct {
	RunID      string `json:"run_id"`
	Label      string `json:"label,omitempty"`
	Status     string `json:"status"`
	Commit     string `json:"commit,omitempty"`
	AppVersion string `json:"app_version,omitempty"`
	Profile    string `json:"profile"`
	Runs       int    `json:"repetitions"`
}

// CompatCheck is one reason results may or may not be compared.
type CompatCheck struct {
	Check     string `json:"check"`
	Status    string `json:"status"` // ok, warning or blocking
	Scope     string `json:"scope"`  // performance, quality or all
	Baseline  string `json:"baseline"`
	Candidate string `json:"candidate"`
	Detail    string `json:"detail,omitempty"`
}

// ComparisonSummary counts metric verdicts over headline metrics.
type ComparisonSummary struct {
	Regressed           int `json:"regressed"`
	Improved            int `json:"improved"`
	NoSignificant       int `json:"no_significant_change"`
	BelowThreshold      int `json:"below_threshold"`
	Changed             int `json:"changed"`
	Insufficient        int `json:"insufficient_samples"`
	Missing             int `json:"missing"`
	Failed              int `json:"failed"`
	Unavailable         int `json:"unavailable"`
	NotComparable       int `json:"not_comparable"`
	DetailRegressed     int `json:"detail_regressed"`
	QualityRegressions  int `json:"quality_regressions"`
	QualityImprovements int `json:"quality_improvements"`
	Tested              int `json:"significance_tests"`
}

// MetricComparison is one metric on both sides. ChangePct and P are absent when
// no honest number exists.
type MetricComparison struct {
	ID         string   `json:"id"`
	Group      string   `json:"group"`
	Unit       string   `json:"unit"`
	Detail     bool     `json:"detail,omitempty"`
	Baseline   *float64 `json:"baseline,omitempty"`
	Candidate  *float64 `json:"candidate,omitempty"`
	ChangePct  *float64 `json:"change_pct,omitempty"`
	P          *float64 `json:"p_value,omitempty"`
	NBaseline  int      `json:"n_baseline"`
	NCandidate int      `json:"n_candidate"`
	Verdict    string   `json:"verdict"`
	Reason     string   `json:"reason,omitempty"`
}

// RateDelta is a share on both sides. Change is in percentage points.
type RateDelta struct {
	Baseline     string   `json:"baseline"` // "191/192"
	Candidate    string   `json:"candidate"`
	BaselinePct  *float64 `json:"baseline_pct,omitempty"`
	CandidatePct *float64 `json:"candidate_pct,omitempty"`
	ChangePoints *float64 `json:"change_points,omitempty"`
}

// CategoryComparison is one row of a suite's comparison.
type CategoryComparison struct {
	Category  string    `json:"category"`
	Top1      RateDelta `json:"top1"`
	Top5      RateDelta `json:"top5"`
	Retrieved RateDelta `json:"retrieved"`
	Coverage  RateDelta `json:"coverage"`
	FalsePos  RateDelta `json:"false_positive_cases"`
}

// CaseChange is one case that moved.
type CaseChange struct {
	Name   string `json:"name"`
	Query  string `json:"query"`
	Before int    `json:"rank_before"`
	After  int    `json:"rank_after"`
	FPDiff int    `json:"false_positive_change"`
}

// SuiteComparison compares one quality suite.
type SuiteComparison struct {
	Suite         string               `json:"suite"`
	Status        string               `json:"status"` // compared, not_comparable or missing
	Reason        string               `json:"reason,omitempty"`
	Categories    []CategoryComparison `json:"categories,omitempty"`
	Regressions   []CaseChange         `json:"regressions,omitempty"`
	Improvements  []CaseChange         `json:"improvements,omitempty"`
	StatusChanges []string             `json:"status_changes,omitempty"`
}

func short(sha string) string { return sha[:min(12, len(sha))] }

// compatibility decides what may be compared. Differences in hardware, catalogue,
// workload or definitions block a claim instead of qualifying it, because a
// percentage printed next to a warning still reads as a result.
func compatibility(b, c *Result) []CompatCheck {
	check := func(name, status, scope, before, after, detail string) CompatCheck {
		if before == after {
			status, detail = "ok", ""
		}
		return CompatCheck{Check: name, Status: status, Scope: scope, Baseline: before, Candidate: after, Detail: detail}
	}
	hosted := func(r *Result) string {
		if r.Environment.CI != nil && r.Environment.CI.Hosted {
			return "hosted"
		}
		return "own machine"
	}
	checks := []CompatCheck{
		check("os", "blocking", "performance", b.Environment.OS, c.Environment.OS, "timings and memory counters from different systems are different quantities"),
		check("architecture", "blocking", "performance", b.Environment.Arch, c.Environment.Arch, "different CPU architectures"),
		check("hardware", "blocking", "performance", b.Environment.HardwareID, c.Environment.HardwareID, "different hardware"),
		check("catalogue", "blocking", "all", short(b.Catalogue.ContentSHA256), short(c.Catalogue.ContentSHA256), "different catalogue contents: neither speed nor quality can be attributed to the code"),
		check("workload", "blocking", "performance", b.Workload.ID, c.Workload.ID, "different workload: profile, queries or parameters differ"),
		check("measurement definitions", "blocking", "all", fmt.Sprint(b.Harness.MeasurementVersion), fmt.Sprint(c.Harness.MeasurementVersion), "metrics were defined differently"),
		check("clock resolution", "warning", "performance", fmt.Sprint(b.Environment.ClockResolutionNS), fmt.Sprint(c.Environment.ClockResolutionNS), "the timers differ, as they can between runs of a virtual machine"),
		check("application Go version", "warning", "performance", b.Target.Go, c.Target.Go, "a toolchain change can move timings by itself"),
		check("harness version", "warning", "all", b.Harness.Version, c.Harness.Version, "results came from different harness versions"),
	}
	if hosted(b) == "hosted" || hosted(c) == "hosted" {
		checks = append(checks, CompatCheck{Check: "hosted runner", Status: "warning", Scope: "performance", Baseline: hosted(b), Candidate: hosted(c), Detail: "hosted runners share hardware with other jobs; treat any timing difference as unproven"})
	}
	if b.Target.Dirty || c.Target.Dirty {
		checks = append(checks, CompatCheck{Check: "uncommitted changes", Status: "warning", Scope: "all", Baseline: fmt.Sprint(b.Target.Dirty), Candidate: fmt.Sprint(c.Target.Dirty), Detail: "a commit does not fully identify a modified tree"})
	}
	return checks
}

func blocked(checks []CompatCheck, names ...string) string {
	for _, c := range checks {
		if c.Status == "blocking" && (len(names) == 0 || slices.Contains(names, c.Check)) {
			return c.Check + ": " + c.Detail
		}
	}
	return ""
}

// floorFor returns a metric's practical-significance floor. Saved results do not
// carry it, because it is a judgement that may be revised without re-measuring.
func floorFor(id string, defs map[string]metricDef) (pct, absolute float64, detail, known bool) {
	d, ok := defs[id]
	return d.minPct, d.minAbs, d.detail, ok
}

// compareMetric gives one metric its verdict.
func compareMetric(b, c *Metric, alpha float64, perfBlock, dataBlock string, defs map[string]metricDef, summary *ComparisonSummary) MetricComparison {
	any := b
	if any == nil {
		any = c
	}
	out := MetricComparison{ID: any.ID, Group: any.Group, Unit: any.Unit}
	minPct, minAbs, detail, _ := floorFor(any.ID, defs)
	out.Detail = detail
	if b != nil {
		out.Baseline, out.NBaseline = b.Value, b.N
	}
	if c != nil {
		out.Candidate, out.NCandidate = c.Value, c.N
	}
	switch {
	case b == nil || c == nil:
		out.Verdict, out.Reason = verdictMissing, "measured on only one side"
		return out
	case b.Status == metricFailed || c.Status == metricFailed:
		out.Verdict, out.Reason = verdictFailed, firstNonEmpty(b.Reason, c.Reason)
		return out
	case b.Status == metricUnavailable || c.Status == metricUnavailable:
		out.Verdict, out.Reason = verdictUnavailable, firstNonEmpty(b.Reason, c.Reason)
		return out
	case b.Unit != c.Unit || b.Definition != c.Definition || b.Counter != c.Counter:
		out.Verdict, out.Reason = verdictIncomparable, fmt.Sprintf("definitions differ (%s against %s)", firstNonEmpty(b.Counter, b.Definition), firstNonEmpty(c.Counter, c.Definition))
		return out
	}
	if any.Deterministic && dataBlock != "" {
		out.Verdict, out.Reason = verdictIncomparable, dataBlock
		return out
	}
	if !any.Deterministic && perfBlock != "" {
		out.Verdict, out.Reason = verdictIncomparable, perfBlock
		return out
	}
	before, after := *b.Value, *c.Value
	if before != 0 {
		change := 100 * (after - before) / math.Abs(before)
		out.ChangePct = &change
	}
	direction := func() string {
		worse := (b.Better == "lower" && after > before) || (b.Better == "higher" && after < before)
		if worse {
			return verdictRegressed
		}
		return verdictImproved
	}
	if any.Deterministic {
		switch {
		case before == after:
			out.Verdict = verdictNoChange
		case b.Better == "none":
			out.Verdict = verdictChanged
		case math.Abs(after-before) < minAbs:
			out.Verdict = verdictBelow
		default:
			out.Verdict = direction()
		}
		return out
	}
	if b.N < minSignificanceSamples || c.N < minSignificanceSamples {
		out.Verdict = verdictInsufficient
		out.Reason = fmt.Sprintf("%d and %d repetitions; at least %d per side are needed", b.N, c.N, minSignificanceSamples)
		return out
	}
	if b.Better == "none" {
		out.Verdict = verdictNoChange
		return out
	}
	p := mannWhitneyP(b.Samples, c.Samples)
	out.P = &p
	summary.Tested++
	switch {
	case p >= alpha:
		out.Verdict = verdictNoChange
	case math.Abs(after-before) < minAbs || (out.ChangePct != nil && math.Abs(*out.ChangePct) < minPct):
		out.Verdict = verdictBelow
		out.Reason = fmt.Sprintf("smaller than the %g%% and %g %s floor", minPct, minAbs, any.Unit)
	default:
		out.Verdict = direction()
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func rate(part, whole int, value *float64) string {
	if whole == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d", part, whole)
}

func delta(beforePart, beforeWhole, afterPart, afterWhole int) RateDelta {
	d := RateDelta{Baseline: rate(beforePart, beforeWhole, nil), Candidate: rate(afterPart, afterWhole, nil), BaselinePct: percent(beforePart, beforeWhole), CandidatePct: percent(afterPart, afterWhole)}
	if d.BaselinePct != nil && d.CandidatePct != nil {
		change := *d.CandidatePct - *d.BaselinePct
		d.ChangePoints = &change
	}
	return d
}

func toQuality(cases []CaseResult) []quality.Result {
	out := make([]quality.Result, len(cases))
	for i, c := range cases {
		out[i] = quality.Result{Name: c.Name, Query: c.Query, Status: c.Status, Rank: c.Rank, FalsePositives: c.FalsePositives}
	}
	return out
}

func toChanges(list []quality.Change) []CaseChange {
	out := make([]CaseChange, len(list))
	for i, c := range list {
		out[i] = CaseChange{Name: c.Name, Query: c.Query, Before: c.Before, After: c.After, FPDiff: c.FPDiff}
	}
	return out
}

// compareSuite compares one suite. Ranks are deterministic, so any change is a
// real change and no significance test applies.
func compareSuite(b, c *SuiteResult, block string) SuiteComparison {
	name := ""
	if b != nil {
		name = b.Name
	} else {
		name = c.Name
	}
	out := SuiteComparison{Suite: name, Status: "compared"}
	switch {
	case b == nil || c == nil:
		out.Status, out.Reason = "missing", "scored on only one side"
		return out
	case block != "":
		out.Status, out.Reason = "not_comparable", block
		return out
	case b.SHA256 != c.SHA256:
		out.Status, out.Reason = "not_comparable", "the case file changed: a different set of questions cannot show a change in answers"
		return out
	case b.Path != c.Path:
		out.Status, out.Reason = "not_comparable", fmt.Sprintf("scored through %s against %s", b.Path, c.Path)
		return out
	}
	rows := map[string]CategoryResult{}
	for _, row := range b.Categories {
		rows[row.Category] = row
	}
	for _, after := range c.Categories {
		before, ok := rows[after.Category]
		if !ok {
			continue
		}
		out.Categories = append(out.Categories, CategoryComparison{
			Category:  after.Category,
			Top1:      delta(before.Top1, before.Counts.Ranked, after.Top1, after.Counts.Ranked),
			Top5:      delta(before.Top5, before.Counts.Ranked, after.Top5, after.Counts.Ranked),
			Retrieved: delta(before.Retrieved, before.Counts.Ranked, after.Retrieved, after.Counts.Ranked),
			Coverage:  delta(before.Counts.Ranked, before.Counts.Ranked+before.Counts.Absent, after.Counts.Ranked, after.Counts.Ranked+after.Counts.Absent),
			FalsePos:  delta(before.FalsePositiveCases, before.FalsePositiveChecks, after.FalsePositiveCases, after.FalsePositiveChecks),
		})
	}
	regressions, improvements := quality.Compare(toQuality(b.Cases), toQuality(c.Cases))
	out.Regressions, out.Improvements = toChanges(regressions), toChanges(improvements)
	statuses := map[string]string{}
	for _, row := range b.Cases {
		statuses[row.Name] = row.Status
	}
	for _, row := range c.Cases {
		if was, ok := statuses[row.Name]; ok && was != row.Status {
			out.StatusChanges = append(out.StatusChanges, fmt.Sprintf("%s: %s to %s", row.Name, was, row.Status))
		}
	}
	// A generated suite keeps no per-case records, so its only evidence is totals.
	if len(b.Cases) == 0 && len(c.Cases) == 0 {
		for _, row := range out.Categories {
			if row.Top1.ChangePoints != nil && *row.Top1.ChangePoints < 0 {
				out.Regressions = append(out.Regressions, CaseChange{Name: "category " + row.Category, Query: "top-1 " + row.Top1.Baseline + " to " + row.Top1.Candidate})
			}
			if row.Top1.ChangePoints != nil && *row.Top1.ChangePoints > 0 {
				out.Improvements = append(out.Improvements, CaseChange{Name: "category " + row.Category, Query: "top-1 " + row.Top1.Baseline + " to " + row.Top1.Candidate})
			}
		}
	}
	return out
}

func refOf(r *Result) RunRef {
	return RunRef{RunID: r.RunID, Label: r.Label, Status: r.Status, Commit: r.Target.Commit, AppVersion: r.Target.AppVersion, Profile: r.Workload.Profile, Runs: r.Workload.Repetitions}
}

// compareResults compares two saved results.
func compareResults(b, c *Result, alpha float64) *Comparison {
	checks := compatibility(b, c)
	out := &Comparison{Schema: schemaComparison, SchemaVersion: schemaVersion, Alpha: alpha, Baseline: refOf(b), Candidate: refOf(c), Compatibility: checks, Metrics: []MetricComparison{}, Quality: []SuiteComparison{}, Notes: []string{}}
	perfBlock := blocked(checks)
	dataBlock := blocked(checks, "os", "architecture", "catalogue", "measurement definitions")
	defs := map[string]metricDef{}
	for _, d := range metricDefs() {
		defs[d.id] = d
	}
	byID := func(r *Result) map[string]*Metric {
		found := make(map[string]*Metric, len(r.Metrics))
		for i := range r.Metrics {
			found[r.Metrics[i].ID] = &r.Metrics[i]
		}
		return found
	}
	before, after := byID(b), byID(c)
	seen := map[string]bool{}
	visit := func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		m := compareMetric(before[id], after[id], alpha, perfBlock, dataBlock, defs, &out.Summary)
		out.Metrics = append(out.Metrics, m)
		s := &out.Summary
		if m.Detail {
			if m.Verdict == verdictRegressed {
				s.DetailRegressed++
			}
			return
		}
		switch m.Verdict {
		case verdictRegressed:
			s.Regressed++
		case verdictImproved:
			s.Improved++
		case verdictNoChange:
			s.NoSignificant++
		case verdictBelow:
			s.BelowThreshold++
		case verdictChanged:
			s.Changed++
		case verdictInsufficient:
			s.Insufficient++
		case verdictMissing:
			s.Missing++
		case verdictFailed:
			s.Failed++
		case verdictUnavailable:
			s.Unavailable++
		case verdictIncomparable:
			s.NotComparable++
		}
	}
	for _, m := range b.Metrics {
		visit(m.ID)
	}
	for _, m := range c.Metrics {
		visit(m.ID)
	}
	qualityBlock := blocked(checks, "catalogue", "measurement definitions")
	suites := map[string]*SuiteResult{}
	for i := range b.Quality {
		suites[b.Quality[i].Name] = &b.Quality[i]
	}
	seenSuite := map[string]bool{}
	for i := range c.Quality {
		seenSuite[c.Quality[i].Name] = true
		out.Quality = append(out.Quality, compareSuite(suites[c.Quality[i].Name], &c.Quality[i], qualityBlock))
	}
	for i := range b.Quality {
		if !seenSuite[b.Quality[i].Name] {
			out.Quality = append(out.Quality, compareSuite(&b.Quality[i], nil, qualityBlock))
		}
	}
	qualityCompared := false
	for _, suite := range out.Quality {
		out.Summary.QualityRegressions += len(suite.Regressions)
		out.Summary.QualityImprovements += len(suite.Improvements)
		if suite.Status == "compared" {
			qualityCompared = true
		} else {
			out.Incomplete = true
		}
	}
	summary := out.Summary
	performance := summary.Regressed + summary.Improved + summary.NoSignificant + summary.BelowThreshold + summary.Insufficient
	switch {
	case performance > 0 && qualityCompared:
		out.Comparable = "full"
	case performance > 0 || qualityCompared:
		out.Comparable = "partial"
	default:
		out.Comparable = "none"
	}
	if summary.Missing+summary.Failed > 0 || b.Status != statusComplete || c.Status != statusComplete {
		out.Incomplete = true
	}
	switch {
	case out.Comparable == "none":
		out.Verdict = overallIncomparable
	case summary.Regressed+summary.QualityRegressions > 0:
		out.Verdict = overallRegressed
	case summary.Improved+summary.QualityImprovements > 0:
		out.Verdict = overallImproved
	default:
		out.Verdict = overallNoChange
	}
	if summary.Tested > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("%d significance tests at alpha %g: about %.1f would look significant by chance alone, before the practical floors are applied", summary.Tested, alpha, float64(summary.Tested)*alpha))
	}
	if !b.Workload.Interleaved || !c.Workload.Interleaved {
		out.Notes = append(out.Notes, "the two sides were measured one after the other, so slow drift in the machine can look like a change; sss-bench ab interleaves them")
	}
	if b.Target.BinarySHA256 == c.Target.BinarySHA256 && b.Target.BinarySHA256 != "" {
		out.Notes = append(out.Notes, "both sides are the same binary: any reported change is noise")
	}
	if summary.Insufficient > 0 {
		out.Notes = append(out.Notes, "metrics with too few repetitions show their change but no verdict; run with more --reps")
	}
	return out
}
