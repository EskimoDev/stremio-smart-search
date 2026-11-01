package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Profile tiers: a metric is measured from its tier upwards.
const (
	tierQuick = iota
	tierStandard
	tierFull
)

// What a metric needs from the target beyond a runnable binary.
const (
	needsApp    = ""
	needsEngine = "engine"
	needsOps    = "ops"
)

// metricDef is the fixed meaning of a metric. minPct and minAbs are the practical
// significance floor used by compare: a statistically real change smaller than
// either is reported as below threshold, not as a regression.
type metricDef struct {
	id, group, unit, better, definition string
	counter                             string
	deterministic                       bool
	detail                              bool // shown in full tables, not in the headline
	sustained                           bool // measured only with sustained searching
	level                               int  // concurrency level, if the metric has one
	tier                                int  // the lowest profile that selects it
	needs                               string
	minPct, minAbs                      float64
}

const (
	groupStartup   = "startup"
	groupHTTP      = "search-http"
	groupEngine    = "search-engine"
	groupCPU       = "cpu"
	groupMemory    = "memory"
	groupCatalogue = "catalogue"
	mib            = 1 << 20
)

var (
	queryClasses  = []string{"exact", "typo", "phonetic", "acronym", "joined", "international", "alias", "short", "ambiguous", "negative", "common"}
	lengthClasses = []string{"short", "medium", "long"}
	levels        = []int{1, 2, 4, 8}
)

func concurrencyTier(level int) int {
	if level > 4 {
		return tierFull
	}
	return tierStandard
}

// metricDefs lists every metric the harness can produce.
func metricDefs() []metricDef {
	resident, private, peak := memoryCounters()
	lat := func(id, group, what string, tier int, needs string) metricDef {
		return metricDef{id: id, group: group, unit: "ms", better: "lower", definition: what, tier: tier, needs: needs, minPct: 5, minAbs: 0.01}
	}
	cold := func(id, group, what string, needs string) metricDef {
		d := lat(id, group, what, tierQuick, needs)
		d.minPct = 10 // one sample per query, so noisier than warm percentiles
		return d
	}
	detail := func(d metricDef) metricDef {
		d.detail = true
		return d
	}
	memory := func(id, what, counter string, tier int, needs string) metricDef {
		return metricDef{id: id, group: groupMemory, unit: "bytes", better: "lower", definition: what, counter: counter, tier: tier, needs: needs, minPct: 3, minAbs: mib}
	}
	work := func(id, what string, tier int, needs string) metricDef {
		return metricDef{id: id, group: groupCatalogue, unit: "ms", better: "lower", definition: what, tier: tier, needs: needs, minPct: 5, minAbs: 5}
	}
	cpuTime := func(id, what string, tier int, needs string) metricDef {
		d := work(id, what, tier, needs)
		d.minAbs = 20 // Windows advances process CPU time about every 15.6 ms
		return d
	}
	exact := func(id, unit, what string, tier int, needs string) metricDef {
		return metricDef{id: id, group: groupCatalogue, unit: unit, better: "none", definition: what, deterministic: true, tier: tier, needs: needs}
	}
	sustained := func(d metricDef) metricDef {
		d.sustained = true
		return d
	}
	const httpNote = "Loopback HTTP through the real binary, keep-alive connection, body read in full; served requests only"
	const engineNote = "Direct Engine.Search with no HTTP and no time budget"
	defs := []metricDef{
		{id: "startup.ready_ms", group: groupStartup, unit: "ms", better: "lower", definition: "Process start to the listener announcing itself, catalogue loaded (warm file cache)", tier: tierQuick, minPct: 5, minAbs: 5},
		{id: "startup.first_search_ms", group: groupStartup, unit: "ms", better: "lower", definition: "Process start to the first search response, including a new connection", tier: tierQuick, minPct: 5, minAbs: 5},

		cold("search.http.cold.p50_ms", groupHTTP, httpNote+"; first pass in a fresh process, each query seen once", needsApp),
		cold("search.http.cold.p95_ms", groupHTTP, httpNote+"; first pass in a fresh process, each query seen once", needsApp),
		lat("search.http.warm.mean_ms", groupHTTP, httpNote+"; after warmup passes", tierQuick, needsApp),
		lat("search.http.warm.p50_ms", groupHTTP, httpNote+"; after warmup passes", tierQuick, needsApp),
		lat("search.http.warm.p95_ms", groupHTTP, httpNote+"; after warmup passes", tierQuick, needsApp),
		lat("search.http.warm.p99_ms", groupHTTP, httpNote+"; after warmup passes", tierQuick, needsApp),
		{id: "search.http.errors", group: groupHTTP, unit: "count", better: "lower", definition: "Failed or non-200 requests across the sequential passes", tier: tierQuick, minAbs: 0.5},
		{id: "search.http.shed", group: groupHTTP, unit: "count", better: "lower", definition: "Requests the server shed (HTTP 200, empty, cacheMaxAge 0) across the sequential passes; excluded from latency", tier: tierQuick, minAbs: 0.5},
		{id: "search.http.over_budget", group: groupHTTP, unit: "count", better: "lower", definition: "Sequential requests taking 100 ms or more: the server's search budget, past which results may be cut short", tier: tierQuick, minAbs: 0.5},

		cold("search.engine.cold.p50_ms", groupEngine, engineNote+"; first pass in a fresh process, each query seen once", needsEngine),
		cold("search.engine.cold.p95_ms", groupEngine, engineNote+"; first pass in a fresh process, each query seen once", needsEngine),
		lat("search.engine.warm.mean_ms", groupEngine, engineNote+"; after warmup passes", tierQuick, needsEngine),
		lat("search.engine.warm.p50_ms", groupEngine, engineNote+"; after warmup passes", tierQuick, needsEngine),
		lat("search.engine.warm.p95_ms", groupEngine, engineNote+"; after warmup passes", tierQuick, needsEngine),
		lat("search.engine.warm.p99_ms", groupEngine, engineNote+"; after warmup passes", tierQuick, needsEngine),
		{id: "search.engine.allocs_per_search", group: groupEngine, unit: "allocs", better: "lower", definition: "Heap allocations per search, over one pass", tier: tierQuick, needs: needsEngine, minPct: 3, minAbs: 1},
		{id: "search.engine.bytes_per_search", group: groupEngine, unit: "bytes", better: "lower", definition: "Heap bytes allocated per search, over one pass", tier: tierQuick, needs: needsEngine, minPct: 3, minAbs: 64},

		{id: "cpu.app.per_request_ms", group: groupCPU, unit: "ms", better: "lower", definition: "CPU time of the application process per warm request, excluding the benchmark client", tier: tierQuick, minPct: 10, minAbs: 0.02},
		{id: "cpu.app.idle_pct", group: groupCPU, unit: "pct", better: "lower", definition: "CPU used by the application during the settle period, percent of one core", tier: tierQuick, minPct: 25, minAbs: 1},

		memory("memory.app.after_load", "Resident memory as soon as the listener is up", resident, tierQuick, needsApp),
		memory("memory.app.idle", "Resident memory after the settle period; no GC forced", resident, tierQuick, needsApp),
		memory("memory.app.private_idle", "Private memory after the settle period", private, tierQuick, needsApp),
		memory("memory.app.after_search", "Resident memory after the warm passes", resident, tierQuick, needsApp),
		memory("memory.app.peak", "Peak resident memory over the application's life", peak, tierQuick, needsApp),
		memory("memory.app.load_overhead", "Temporary memory of startup: peak so far minus resident memory after settling", peak, tierQuick, needsApp),
		sustained(memory("memory.app.sustained_end", "Resident memory at the end of sustained searching (median of the last third of samples)", resident, tierStandard, needsApp)),
		{id: "memory.app.sustained_growth", group: groupMemory, unit: "MiB/min", better: "lower", definition: "Approximate growth during sustained searching: change in median resident memory from the first to the last third of samples", counter: resident, sustained: true, tier: tierStandard, minAbs: 1},
		memory("memory.engine.heap_after_load", "Live Go heap after decoding the catalogue, after a forced GC", "runtime.MemStats.HeapAlloc", tierQuick, needsEngine),
		memory("memory.engine.heap_after_index", "Live Go heap after building the index and dropping the parsed copy, after a forced GC", "runtime.MemStats.HeapAlloc", tierQuick, needsEngine),
		memory("memory.engine.heap_after_search", "Live Go heap after all searches, after a forced GC", "runtime.MemStats.HeapAlloc", tierQuick, needsEngine),
		memory("memory.engine.peak", "Peak resident memory of the engine probe, including catalogue decoding", peak, tierQuick, needsEngine),
		memory("memory.ops.prepare_peak", "Peak resident memory parsing IMDb-shaped files and saving a snapshot", peak, tierFull, needsOps),
		memory("memory.ops.refresh_peak", "Peak resident memory during a live refresh, with the old index still serving", peak, tierFull, needsOps),

		work("catalog.read_ms", "Disk read of the snapshot file into memory (warm file cache)", tierQuick, needsEngine),
		work("catalog.load_ms", "catalog.Load wall time: read, gunzip, JSON decode and validate", tierQuick, needsEngine),
		cpuTime("catalog.load_cpu_ms", "catalog.Load CPU time", tierQuick, needsEngine),
		work("catalog.index_ms", "search.New wall time", tierQuick, needsEngine),
		cpuTime("catalog.index_cpu_ms", "search.New CPU time", tierQuick, needsEngine),
		work("catalog.rebuild_ms", "Rebuilding the index with the previous one still reachable", tierFull, needsEngine),
		work("catalog.prepare_ms", "Parsing local IMDb-shaped files; download excluded", tierFull, needsOps),
		cpuTime("catalog.prepare_cpu_ms", "CPU time parsing local IMDb-shaped files", tierFull, needsOps),
		work("catalog.save_ms", "Writing the prepared snapshot to disk", tierFull, needsOps),
		work("catalog.refresh_ms", "POST /api/build to the new catalogue being live; download excluded", tierFull, needsOps),
		cpuTime("catalog.refresh_cpu_ms", "CPU time of the refresh, including the searches made during it", tierFull, needsOps),
		{id: "catalog.refresh_searches_served", group: groupCatalogue, unit: "count", better: "none", definition: "Searches answered during the refresh", tier: tierFull, needs: needsOps, detail: true},
		{id: "catalog.refresh_searches_empty", group: groupCatalogue, unit: "count", better: "lower", definition: "Searches during the refresh that lost their result: the working index must keep answering", tier: tierFull, needs: needsOps, minAbs: 0.5},

		exact("catalog.titles", "count", "Titles indexed by the running application", tierQuick, needsApp),
		exact("catalog.movies", "count", "Movies indexed by the running application", tierQuick, needsApp),
		exact("catalog.series", "count", "Series indexed by the running application", tierQuick, needsApp),
		exact("catalog.aliases", "count", "Alternative titles indexed by the running application", tierQuick, needsApp),
		exact("catalog.file_bytes", "bytes", "Stored catalogue snapshot size", tierQuick, needsApp),
		exact("catalog.prepared_bytes", "bytes", "Size of the snapshot this build prepares from the local IMDb-shaped files", tierFull, needsOps),
		{id: "binary.app_bytes", group: groupCatalogue, unit: "bytes", better: "lower", definition: "Size of the application binary", deterministic: true, tier: tierQuick, minPct: 1, minAbs: 4096},
	}
	for _, class := range queryClasses {
		defs = append(defs,
			detail(lat("search.http.class."+class+".p95_ms", groupHTTP, httpNote+"; "+class+" queries after warmup", tierQuick, needsApp)),
			detail(lat("search.engine.class."+class+".p95_ms", groupEngine, engineNote+"; "+class+" queries after warmup", tierQuick, needsEngine)))
	}
	for _, length := range lengthClasses {
		defs = append(defs,
			detail(lat("search.http.length."+length+".p95_ms", groupHTTP, httpNote+"; "+length+" queries after warmup", tierQuick, needsApp)),
			detail(lat("search.engine.length."+length+".p95_ms", groupEngine, engineNote+"; "+length+" queries after warmup", tierQuick, needsEngine)))
	}
	atLevel := func(d metricDef, level int) metricDef {
		d.level = level
		return d
	}
	for _, level := range levels {
		tier := concurrencyTier(level)
		defs = append(defs,
			atLevel(metricDef{id: fmt.Sprintf("search.http.served_rps.c%d", level), group: groupHTTP, unit: "req/s", better: "higher", definition: fmt.Sprintf("Requests actually served per second by %d closed-loop clients; shed requests excluded", level), tier: tier, minPct: 5}, level),
			atLevel(metricDef{id: fmt.Sprintf("search.http.shed_pct.c%d", level), group: groupHTTP, unit: "pct", better: "lower", definition: fmt.Sprintf("Share of requests shed by the server's two search slots at %d clients", level), tier: tier, minAbs: 1}, level),
			atLevel(lat(fmt.Sprintf("search.http.concurrent.c%d.p95_ms", level), groupHTTP, fmt.Sprintf("%s; %d closed-loop clients", httpNote, level), tier, needsApp), level),
			atLevel(metricDef{id: fmt.Sprintf("search.engine.qps.c%d", level), group: groupEngine, unit: "searches/s", better: "higher", definition: fmt.Sprintf("Direct searches per second from %d goroutines", level), tier: tier, needs: needsEngine, minPct: 5}, level))
	}
	return defs
}

// collector gathers per-repetition values for one target.
type collector struct {
	defs    map[string]metricDef
	order   []string
	samples map[string][]float64
	reasons map[string]Problem // id to status (Stage) and reason (Message)
	each    map[string]int
}

func newCollector() *collector {
	c := &collector{defs: map[string]metricDef{}, samples: map[string][]float64{}, reasons: map[string]Problem{}, each: map[string]int{}}
	for _, d := range metricDefs() {
		c.defs[d.id] = d
		c.order = append(c.order, d.id)
	}
	return c
}

// add records one repetition's value. An unknown id is a harness bug.
func (c *collector) add(id string, value float64) {
	if _, ok := c.defs[id]; !ok {
		panic("sss-bench: unknown metric " + id)
	}
	if !math.IsNaN(value) && !math.IsInf(value, 0) {
		c.samples[id] = append(c.samples[id], value)
	}
}

// addCount records a value together with how many observations produced it.
func (c *collector) addCount(id string, value float64, observations int) {
	c.add(id, value)
	c.each[id] = max(c.each[id], observations)
}

// unavailable records that a metric cannot be measured here, and why.
func (c *collector) unavailable(id, reason string) {
	c.reasons[id] = Problem{Stage: metricUnavailable, Message: reason}
}

// failed records that a metric should have been measured and was not.
func (c *collector) failed(id, reason string) {
	c.reasons[id] = Problem{Stage: metricFailed, Message: reason}
}

// inScope reports whether the configuration selected a metric at all. It follows
// what is actually run, so an override such as --sustain 0 removes those metrics
// instead of reporting them as failed.
func inScope(d metricDef, p Profile) bool {
	switch {
	case d.level > 0:
		return slices.Contains(p.Concurrency, d.level)
	case d.sustained:
		return p.Sustain > 0
	case d.id == "catalog.rebuild_ms":
		return p.Rebuilds > 0
	case d.needs == needsOps:
		return p.CatalogOps
	}
	return true
}

// metrics turns the collected values into results. A selected metric with no
// value and no explanation is reported as failed, never silently dropped.
func (c *collector) metrics(p Profile, repetitions int) []Metric {
	out := []Metric{}
	for _, id := range c.order {
		d := c.defs[id]
		if !inScope(d, p) {
			continue
		}
		m := Metric{ID: id, Group: d.group, Unit: d.unit, Better: d.better, Definition: d.definition, Counter: d.counter, Deterministic: d.deterministic, Status: metricOK, EachRunCount: c.each[id]}
		values := c.samples[id]
		if reason, ok := c.reasons[id]; ok {
			m.Status, m.Reason = reason.Stage, reason.Message
			if reason.Stage == metricUnavailable {
				values = nil
			}
		} else if len(values) == 0 {
			m.Status, m.Reason = metricFailed, "no value was recorded; see errors"
		}
		if len(values) > 0 {
			m.Status = metricOK
			fill(&m, values)
			if !d.deterministic && len(values) < repetitions {
				m.Flags = append(m.Flags, "incomplete")
			}
		}
		out = append(out, m)
	}
	return out
}

func fill(m *Metric, values []float64) {
	value, low, high, average, deviation := median(values), slices.Min(values), slices.Max(values), mean(values), sampleStdDev(values)
	m.Samples, m.N, m.Value, m.Min, m.Max, m.Mean, m.StdDev = values, len(values), &value, &low, &high, &average, &deviation
	if average != 0 {
		cv := deviation / math.Abs(average)
		m.CV = &cv
		if cv > noisyCV && len(values) > 1 {
			m.Flags = append(m.Flags, "noisy")
		}
	}
	if l, h, ok := medianInterval(values); ok {
		m.CILow, m.CIHigh = &l, &h
	}
	// A nearest-rank percentile of too few observations is only the maximum.
	if (strings.HasSuffix(m.ID, "p99_ms") && m.EachRunCount < 100) || (strings.HasSuffix(m.ID, "p95_ms") && m.EachRunCount < 20) {
		m.Flags = append(m.Flags, "low_sample_count")
	}
}
