package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func formatValue(unit string, v float64) string {
	switch unit {
	case "ms":
		switch {
		case v < 1:
			return fmt.Sprintf("%.3f ms", v)
		case v < 100:
			return fmt.Sprintf("%.2f ms", v)
		}
		return fmt.Sprintf("%.0f ms", v)
	case "bytes":
		switch {
		case v >= mib:
			return fmt.Sprintf("%.1f MiB", v/mib)
		case v >= 1024:
			return fmt.Sprintf("%.1f KiB", v/1024)
		}
		return fmt.Sprintf("%.0f B", v)
	case "pct":
		return fmt.Sprintf("%.1f%%", v)
	case "count", "req/s", "searches/s":
		return strconv.FormatFloat(math.Round(v), 'f', 0, 64) + strings.TrimPrefix(" "+unit, " count")
	}
	return fmt.Sprintf("%.2f %s", v, unit)
}

func metricValue(m *Metric) string {
	switch {
	case m == nil:
		return "–"
	case m.Status != metricOK:
		return m.Status
	}
	return formatValue(m.Unit, *m.Value)
}

func index(r *Result) map[string]*Metric {
	found := make(map[string]*Metric, len(r.Metrics))
	for i := range r.Metrics {
		found[r.Metrics[i].ID] = &r.Metrics[i]
	}
	return found
}

func shortCommit(commit string) string {
	if commit == "" {
		return "unknown commit"
	}
	return commit[:min(9, len(commit))]
}

func describeTarget(r *Result) string {
	text := "`" + shortCommit(r.Target.Commit) + "`"
	if r.Target.Kind == "binary" {
		text = "binary `" + short(r.Target.BinarySHA256) + "`"
	}
	if r.Target.Dirty {
		text += " (uncommitted changes)"
	}
	if r.Target.AppVersion != "" {
		text += " version " + r.Target.AppVersion
	}
	return text
}

func describeMachine(e Environment) string {
	text := fmt.Sprintf("%s/%s, %s, %d threads, %.0f GiB", e.OS, e.Arch, firstNonEmpty(e.CPUModel, "unknown CPU"), e.LogicalCPUs, float64(e.MemoryTotalBytes)/(1<<30))
	if e.CI != nil {
		text += ", " + e.CI.Provider
		if e.CI.Hosted {
			text += " hosted runner"
		}
	}
	return text
}

func rowFor(b *strings.Builder, label string, metrics map[string]*Metric, ids ...string) {
	cells := []string{label}
	shown := false
	for _, id := range ids {
		m := metrics[id]
		cells = append(cells, metricValue(m))
		shown = shown || m != nil
	}
	if shown {
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
}

// renderRun writes the human report: short enough for a pull request comment,
// with the denominators and caveats kept in view.
func renderRun(r *Result) string {
	var b strings.Builder
	m := index(r)
	fmt.Fprintf(&b, "## SSS benchmark: %s profile, %s\n\n", r.Workload.Profile, r.Status)
	if r.Label != "" {
		fmt.Fprintf(&b, "**Label:** %s  \n", r.Label)
	}
	fmt.Fprintf(&b, "**Target:** %s  \n", describeTarget(r))
	fmt.Fprintf(&b, "**Machine:** %s  \n", describeMachine(r.Environment))
	fmt.Fprintf(&b, "**Catalogue:** %s, %d titles (%d movies, %d series, %d alternative titles), `%s`  \n", r.Catalogue.Name, r.Catalogue.Titles, r.Catalogue.Movies, r.Catalogue.Series, r.Catalogue.Aliases, short(r.Catalogue.ContentSHA256))
	fmt.Fprintf(&b, "**Workload:** `%s`, %d fresh-process repetitions of %d queries, %d passes after %d warmup; values are medians across repetitions  \n", r.Workload.ID, r.Workload.Repetitions, r.Workload.Queries, r.Workload.Passes, r.Workload.WarmupPasses)
	fmt.Fprintf(&b, "**Timer resolution:** %d ns; **limits:** GOMAXPROCS %d, %d MiB soft memory target\n\n", r.Environment.ClockResolutionNS, r.Workload.GOMAXPROCS, r.Workload.MemoryLimitMiB)
	if r.Catalogue.Kind == "synthetic" {
		b.WriteString("> Synthetic catalogue. Quality scores below detect ranking changes between builds; they are not accuracy on real titles.\n\n")
	}

	b.WriteString("### Search latency\n\n| Path | p50 | p95 | p99 | Mean |\n| --- | ---: | ---: | ---: | ---: |\n")
	rowFor(&b, "HTTP, warm", m, "search.http.warm.p50_ms", "search.http.warm.p95_ms", "search.http.warm.p99_ms", "search.http.warm.mean_ms")
	rowFor(&b, "Engine, warm", m, "search.engine.warm.p50_ms", "search.engine.warm.p95_ms", "search.engine.warm.p99_ms", "search.engine.warm.mean_ms")
	fmt.Fprintf(&b, "\nCold (first pass in a fresh process, p50 / p95): HTTP %s / %s; engine %s / %s.  \n", metricValue(m["search.http.cold.p50_ms"]), metricValue(m["search.http.cold.p95_ms"]), metricValue(m["search.engine.cold.p50_ms"]), metricValue(m["search.engine.cold.p95_ms"]))
	fmt.Fprintf(&b, "Startup to ready %s; to first search %s. Application CPU per request %s.\n", metricValue(m["startup.ready_ms"]), metricValue(m["startup.first_search_ms"]), metricValue(m["cpu.app.per_request_ms"]))
	throughput := false
	for _, level := range levels {
		if m[fmt.Sprintf("search.http.served_rps.c%d", level)] != nil {
			throughput = true
		}
	}
	if throughput {
		b.WriteString("\n### Throughput\n\n| Clients | Served over HTTP | Shed by server | p95 of served | Direct engine |\n| ---: | ---: | ---: | ---: | ---: |\n")
		for _, level := range levels {
			rowFor(&b, strconv.Itoa(level), m, fmt.Sprintf("search.http.served_rps.c%d", level), fmt.Sprintf("search.http.shed_pct.c%d", level), fmt.Sprintf("search.http.concurrent.c%d.p95_ms", level), fmt.Sprintf("search.engine.qps.c%d", level))
		}
		b.WriteString("\nThe server has two search slots and answers any other request at once with an empty list, so shed requests are counted separately and never as fast successes.\n")
	}

	b.WriteString("\n### Memory\n\n| Measure | Value | Counter |\n| --- | ---: | --- |\n")
	for _, row := range []struct{ label, id string }{
		{"After load", "memory.app.after_load"}, {"Idle, after settling", "memory.app.idle"}, {"Private, idle", "memory.app.private_idle"},
		{"After searching", "memory.app.after_search"}, {"End of sustained searching", "memory.app.sustained_end"}, {"Peak", "memory.app.peak"},
		{"Go heap after index", "memory.engine.heap_after_index"}, {"Engine probe peak", "memory.engine.peak"},
		{"Peak preparing a catalogue", "memory.ops.prepare_peak"}, {"Peak during a refresh", "memory.ops.refresh_peak"},
	} {
		if metric := m[row.id]; metric != nil {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", row.label, metricValue(metric), firstNonEmpty(metric.Counter, "–"))
		}
	}
	if metric := m["memory.app.sustained_growth"]; metric != nil {
		fmt.Fprintf(&b, "\nApproximate growth during sustained searching: %s.\n", metricValue(metric))
	}
	b.WriteString("\nCounters differ by platform and are not equivalent; compare only like with like.\n")

	b.WriteString("\n### Catalogue operations\n\n| Operation | Wall | CPU |\n| --- | ---: | ---: |\n")
	rowFor(&b, "Load snapshot", m, "catalog.load_ms", "catalog.load_cpu_ms")
	rowFor(&b, "Build index", m, "catalog.index_ms", "catalog.index_cpu_ms")
	if m["catalog.rebuild_ms"] != nil {
		rowFor(&b, "Rebuild index", m, "catalog.rebuild_ms", "none")
	}
	rowFor(&b, "Prepare from local files", m, "catalog.prepare_ms", "catalog.prepare_cpu_ms")
	rowFor(&b, "Live refresh", m, "catalog.refresh_ms", "catalog.refresh_cpu_ms")
	fmt.Fprintf(&b, "\nDisk read of the snapshot: %s (warm file cache). No network is used. Stored snapshot %s; application binary %s.\n", metricValue(m["catalog.read_ms"]), metricValue(m["catalog.file_bytes"]), metricValue(m["binary.app_bytes"]))

	if len(r.Quality) > 0 {
		b.WriteString("\n### Search quality\n\nTop-1, top-5 and found-in-20 are out of the ranked cases. Cases whose title is absent from this catalogue are coverage gaps, not misses.\n\n")
		b.WriteString("| Suite | Answered by | Top-1 | Top-5 | Found in 20 | Absent | Other language | False-positive cases |\n| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, suite := range r.Quality {
			row := suite.Categories[0]
			c := row.Counts
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d | %d | %s |\n", suite.Name, suite.Path, share(row.Top1, c.Ranked), share(row.Top5, c.Ranked), share(row.Retrieved, c.Ranked), c.Absent, c.OtherLanguage+c.AliasGap, share(row.FalsePositiveCases, row.FalsePositiveChecks))
		}
		b.WriteString("\n<details><summary>By category</summary>\n\n| Suite | Category | Top-1 | Top-5 | Found in 20 | Absent | False-positive cases |\n| --- | --- | ---: | ---: | ---: | ---: | ---: |\n")
		for _, suite := range r.Quality {
			for _, row := range suite.Categories[1:] {
				c := row.Counts
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d | %s |\n", suite.Name, row.Category, share(row.Top1, c.Ranked), share(row.Top5, c.Ranked), share(row.Retrieved, c.Ranked), c.Absent, share(row.FalsePositiveCases, row.FalsePositiveChecks))
			}
		}
		b.WriteString("\n</details>\n")
	}

	var unavailable []string
	for _, metric := range r.Metrics {
		if metric.Status == metricUnavailable {
			unavailable = append(unavailable, fmt.Sprintf("- `%s`: %s", metric.ID, metric.Reason))
		}
	}
	if len(unavailable) > 0 {
		b.WriteString("\n### Not measured here\n\n" + strings.Join(unavailable, "\n") + "\n")
	}
	if len(r.Errors) > 0 {
		b.WriteString("\n### Errors\n\n")
		for _, p := range r.Errors {
			fmt.Fprintf(&b, "- %s: %s\n", p.Stage, p.Message)
		}
	}
	var failed []string
	for _, metric := range r.Metrics {
		if metric.Status == metricFailed {
			failed = append(failed, "`"+metric.ID+"`")
		}
	}
	if len(failed) > 0 {
		b.WriteString("\n**Failed metrics:** " + strings.Join(failed, ", ") + "\n")
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\n### Warnings\n\n")
		for _, w := range r.Warnings {
			b.WriteString("- " + w + "\n")
		}
	}
	return b.String()
}

func share(part, whole int) string {
	if whole == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", part, whole, 100*float64(part)/float64(whole))
}

func verdictLine(c *Comparison) string {
	switch c.Verdict {
	case overallRegressed:
		return "regression found"
	case overallImproved:
		return "improvement found"
	case overallIncomparable:
		return "not directly comparable"
	}
	return "no significant change"
}

func comparisonRow(m MetricComparison) string {
	value := func(v *float64) string {
		if v == nil {
			return "–"
		}
		return formatValue(m.Unit, *v)
	}
	change, p := "–", "–"
	if m.ChangePct != nil {
		change = fmt.Sprintf("%+.1f%%", *m.ChangePct)
	}
	if m.P != nil {
		p = fmt.Sprintf("%.3f", *m.P)
	}
	return fmt.Sprintf("| `%s` | %s | %s | %s | %s | %d/%d | %s |\n", m.ID, value(m.Baseline), value(m.Candidate), change, p, m.NBaseline, m.NCandidate, strings.ReplaceAll(m.Verdict, "_", " "))
}

const comparisonHeader = "| Metric | Baseline | Candidate | Change | p | Runs | Verdict |\n| --- | ---: | ---: | ---: | ---: | ---: | --- |\n"

func pointChange(d RateDelta) string {
	if d.ChangePoints == nil {
		return d.Baseline + " → " + d.Candidate
	}
	return fmt.Sprintf("%s → %s (%+.1f pp)", d.Baseline, d.Candidate, *d.ChangePoints)
}

// renderComparison writes the human comparison, leading with what can be claimed.
func renderComparison(c *Comparison) string {
	var b strings.Builder
	name := func(r RunRef) string {
		return firstNonEmpty(r.Label, "`"+shortCommit(r.Commit)+"`")
	}
	fmt.Fprintf(&b, "## SSS benchmark comparison: %s\n\n", verdictLine(c))
	fmt.Fprintf(&b, "**Baseline:** %s (%s, %d repetitions, %s)  \n**Candidate:** %s (%s, %d repetitions, %s)  \n", name(c.Baseline), c.Baseline.Profile, c.Baseline.Runs, c.Baseline.Status, name(c.Candidate), c.Candidate.Profile, c.Candidate.Runs, c.Candidate.Status)
	s := c.Summary
	fmt.Fprintf(&b, "**Headline metrics:** %d regressed, %d improved, %d no significant change, %d below the practical floor, %d too few samples, %d not comparable, %d missing, %d failed  \n", s.Regressed, s.Improved, s.NoSignificant, s.BelowThreshold, s.Insufficient, s.NotComparable, s.Missing, s.Failed)
	fmt.Fprintf(&b, "**Search quality:** %d cases worse, %d cases better\n\n", s.QualityRegressions, s.QualityImprovements)
	if c.Incomplete {
		b.WriteString("> **Incomplete:** at least one side failed part of its measurements. Treat absent results as unknown, not as unchanged.\n\n")
	}
	var problems []string
	for _, check := range c.Compatibility {
		if check.Status != "ok" {
			problems = append(problems, fmt.Sprintf("| %s | %s | %s | %s | %s |\n", check.Status, check.Check, check.Baseline, check.Candidate, check.Detail))
		}
	}
	if len(problems) > 0 {
		b.WriteString("### Comparability\n\n| Status | Check | Baseline | Candidate | Why it matters |\n| --- | --- | --- | --- | --- |\n" + strings.Join(problems, "") + "\n")
	}
	section := func(title, verdict string) {
		var rows []string
		for _, m := range c.Metrics {
			if m.Verdict == verdict && !m.Detail {
				rows = append(rows, comparisonRow(m))
			}
		}
		if len(rows) > 0 {
			b.WriteString("### " + title + "\n\n" + comparisonHeader + strings.Join(rows, "") + "\n")
		}
	}
	section("Regressions", verdictRegressed)
	section("Improvements", verdictImproved)
	for _, suite := range c.Quality {
		fmt.Fprintf(&b, "### Quality: %s\n\n", suite.Suite)
		if suite.Status != "compared" {
			fmt.Fprintf(&b, "%s: %s\n\n", strings.ReplaceAll(suite.Status, "_", " "), suite.Reason)
			continue
		}
		if len(suite.Categories) > 0 {
			row := suite.Categories[0]
			fmt.Fprintf(&b, "Top-1 %s; top-5 %s; found in 20 %s; coverage %s; false-positive cases %s.\n\n", pointChange(row.Top1), pointChange(row.Top5), pointChange(row.Retrieved), pointChange(row.Coverage), pointChange(row.FalsePos))
		}
		list := func(title string, changes []CaseChange) {
			if len(changes) == 0 {
				return
			}
			fmt.Fprintf(&b, "%s:\n\n", title)
			for _, change := range changes[:min(20, len(changes))] {
				fmt.Fprintf(&b, "- `%s` %q: rank %d → %d, false positives %+d\n", change.Name, change.Query, change.Before, change.After, change.FPDiff)
			}
			if len(changes) > 20 {
				fmt.Fprintf(&b, "- and %d more\n", len(changes)-20)
			}
			b.WriteString("\n")
		}
		list("Worse", suite.Regressions)
		list("Better", suite.Improvements)
		for _, change := range suite.StatusChanges {
			fmt.Fprintf(&b, "- scoring status changed for %s\n", change)
		}
	}
	b.WriteString("<details><summary>All metrics</summary>\n\n" + comparisonHeader)
	for _, m := range c.Metrics {
		b.WriteString(comparisonRow(m))
	}
	b.WriteString("\n</details>\n")
	if len(c.Notes) > 0 {
		b.WriteString("\n### Notes\n\n")
		for _, note := range c.Notes {
			b.WriteString("- " + note + "\n")
		}
	}
	b.WriteString("\nRelative metrics show percentage change; shares show percentage points (pp). Verdicts use a two-sided exact Mann-Whitney test across fresh-process repetitions plus a practical floor per metric.\n")
	return b.String()
}

// renderBench writes Go benchmark format, one line per repetition, so that
// benchstat before.bench after.bench gives an independent reading of a result.
func renderBench(r *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "goos: %s\ngoarch: %s\npkg: github.com/EskimoDev/stremio-smart-search/tools/sss-bench\ncpu: %s\n", r.Environment.OS, r.Environment.Arch, r.Environment.CPUModel)
	for _, m := range r.Metrics {
		if m.Status != metricOK || m.Deterministic {
			continue
		}
		for _, sample := range m.Samples {
			fmt.Fprintf(&b, "BenchmarkSSS/%s 1 %s %s\n", m.ID, strconv.FormatFloat(sample, 'g', -1, 64), m.Unit)
		}
	}
	return b.String()
}
