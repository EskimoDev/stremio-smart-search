package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Versions recorded in every result. Bump measurementVersion when a metric's
// meaning changes and workloadVersion when the fixture or query set changes:
// compare then reports results as not directly comparable, which is the point.
const (
	schemaResult       = "sss-bench/result"
	schemaComparison   = "sss-bench/comparison"
	schemaVersion      = 1
	harnessVersion     = "1.0.0"
	measurementVersion = 1
	workloadVersion    = 1
)

// Result statuses.
const (
	statusComplete = "complete" // every selected metric measured or explained
	statusPartial  = "partial"  // something failed; see errors
	statusFailed   = "failed"   // nothing usable was measured
)

// Metric statuses.
const (
	metricOK          = "ok"
	metricUnavailable = "unavailable" // this platform or target cannot measure it
	metricFailed      = "failed"      // it should have worked and did not
)

// Result is one benchmark run: the versioned contract with agents and CI.
type Result struct {
	Schema          string        `json:"schema"`
	SchemaVersion   int           `json:"schema_version"`
	Status          string        `json:"status"`
	RunID           string        `json:"run_id"`
	Label           string        `json:"label,omitempty"`
	StartedAt       time.Time     `json:"started_at"`
	DurationSeconds float64       `json:"duration_seconds"`
	Harness         HarnessInfo   `json:"harness"`
	Target          TargetInfo    `json:"target"`
	Environment     Environment   `json:"environment"`
	Catalogue       CatalogueInfo `json:"catalogue"`
	Workload        WorkloadInfo  `json:"workload"`
	Metrics         []Metric      `json:"metrics"`
	Quality         []SuiteResult `json:"quality,omitempty"`
	Errors          []Problem     `json:"errors"`
	Warnings        []string      `json:"warnings"`
}

// HarnessInfo identifies the measuring tool, as distinct from what it measures.
type HarnessInfo struct {
	Version            string `json:"version"`
	MeasurementVersion int    `json:"measurement_version"`
	Commit             string `json:"commit,omitempty"`
	Dirty              bool   `json:"dirty,omitempty"`
	Go                 string `json:"go"`
}

// TargetInfo identifies the build being measured.
type TargetInfo struct {
	Kind         string `json:"kind"` // source or binary
	Commit       string `json:"commit,omitempty"`
	Dirty        bool   `json:"dirty,omitempty"`
	AppVersion   string `json:"app_version,omitempty"`
	Go           string `json:"go,omitempty"`
	BuildFlags   string `json:"build_flags,omitempty"`
	BinarySHA256 string `json:"binary_sha256"`
	BinaryBytes  int64  `json:"binary_bytes"`
	EngineProbe  string `json:"engine_probe"` // built, or why not
	OpsProbe     string `json:"ops_probe"`
}

// CIInfo is set when the run happened in continuous integration.
type CIInfo struct {
	Provider string `json:"provider"`
	Hosted   bool   `json:"hosted"`
	Runner   string `json:"runner,omitempty"`
	RunID    string `json:"run_id,omitempty"`
}

// Environment describes the machine. It deliberately omits the host name, user
// name and paths: reports get pasted into public pull requests.
type Environment struct {
	OS                string  `json:"os"`
	Arch              string  `json:"arch"`
	OSVersion         string  `json:"os_version"`
	CPUModel          string  `json:"cpu_model"`
	LogicalCPUs       int     `json:"logical_cpus"`
	MemoryTotalBytes  uint64  `json:"memory_total_bytes"`
	HardwareID        string  `json:"hardware_id"`
	ClockResolutionNS int64   `json:"clock_resolution_ns"`
	CI                *CIInfo `json:"ci,omitempty"`
}

// CatalogueInfo fingerprints the data under test.
type CatalogueInfo struct {
	Kind          string    `json:"kind"` // synthetic or supplied
	Name          string    `json:"name"`
	ContentSHA256 string    `json:"content_sha256"`
	FileSHA256    string    `json:"file_sha256"`
	FileBytes     int64     `json:"file_bytes"`
	Titles        int       `json:"titles"`
	Movies        int       `json:"movies"`
	Series        int       `json:"series"`
	Aliases       int       `json:"aliases"`
	AliasLanguage string    `json:"alias_language,omitempty"`
	Profile       string    `json:"profile,omitempty"`
	BuiltAt       time.Time `json:"built_at"`
	Source        string    `json:"source,omitempty"`
}

// WorkloadInfo fixes what was run. ID covers everything that changes the meaning
// of a timing; Repetitions is left out so that a longer run stays comparable.
type WorkloadInfo struct {
	ID             string            `json:"id"`
	Version        int               `json:"version"`
	Profile        string            `json:"profile"`
	Repetitions    int               `json:"repetitions"`
	Interleaved    bool              `json:"interleaved,omitempty"`
	Queries        int               `json:"queries"`
	QueriesSHA256  string            `json:"queries_sha256"`
	WarmupPasses   int               `json:"warmup_passes"`
	Passes         int               `json:"passes"`
	SettleSeconds  float64           `json:"settle_seconds"`
	Throughput     float64           `json:"throughput_seconds,omitempty"`
	Concurrency    []int             `json:"concurrency,omitempty"`
	SustainSeconds float64           `json:"sustain_seconds,omitempty"`
	Rebuilds       int               `json:"rebuilds,omitempty"`
	Seed           uint64            `json:"seed"`
	GOMAXPROCS     int               `json:"gomaxprocs"`
	MemoryLimitMiB int64             `json:"memory_limit_mib"`
	Inputs         map[string]string `json:"inputs,omitempty"` // file fingerprints
}

// Metric is one measured quantity. Samples holds one value per fresh process, the
// unit of replication; Value is their median.
type Metric struct {
	ID            string    `json:"id"`
	Group         string    `json:"group"`
	Unit          string    `json:"unit"`
	Better        string    `json:"better"` // lower, higher or none
	Definition    string    `json:"definition"`
	Counter       string    `json:"counter,omitempty"` // exact OS or runtime counter
	Status        string    `json:"status"`
	Reason        string    `json:"reason,omitempty"`
	Deterministic bool      `json:"deterministic,omitempty"`
	Value         *float64  `json:"value,omitempty"`
	N             int       `json:"n"`
	Samples       []float64 `json:"samples,omitempty"`
	Min           *float64  `json:"min,omitempty"`
	Max           *float64  `json:"max,omitempty"`
	Mean          *float64  `json:"mean,omitempty"`
	StdDev        *float64  `json:"stddev,omitempty"`
	CV            *float64  `json:"cv,omitempty"`
	CILow         *float64  `json:"ci95_low,omitempty"`
	CIHigh        *float64  `json:"ci95_high,omitempty"`
	Flags         []string  `json:"flags,omitempty"`
	EachRunCount  int       `json:"samples_per_repetition,omitempty"`
}

// Problem is something that went wrong, kept in the result rather than dropped.
type Problem struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// SuiteResult is one case file (or the generated queries) scored on the catalogue.
type SuiteResult struct {
	Name       string           `json:"name"`
	SHA256     string           `json:"sha256"`
	Path       string           `json:"path"` // engine or http: what answered the queries
	Totals     CaseCounts       `json:"totals"`
	Categories []CategoryResult `json:"categories"`
	Cases      []CaseResult     `json:"cases,omitempty"`
}

// CaseCounts says what happened to every case in a suite. Total is the sum of
// the other fields, so nothing silently disappears from a denominator.
type CaseCounts struct {
	Total         int `json:"total"`
	Ranked        int `json:"ranked"`         // scored, with an expected answer
	NoAnswer      int `json:"no_answer"`      // scored, with no right answer
	Absent        int `json:"absent"`         // expected title not in this catalogue
	OtherLanguage int `json:"other_language"` // needs another alias language
	AliasGap      int `json:"alias_gap"`      // catalogue kept other regional titles
	TruthErrors   int `json:"truth_errors"`   // expected id disagrees with catalogue
}

// CategoryResult keeps retrieval, ranking and coverage apart. Top1, Top5 and
// Retrieved are all out of Ranked; absent titles are never counted as misses.
type CategoryResult struct {
	Category     string     `json:"category"`
	Counts       CaseCounts `json:"counts"`
	Top1         int        `json:"top1"`
	Top5         int        `json:"top5"`
	Retrieved    int        `json:"retrieved"` // in the first 20 results
	Top1Pct      *float64   `json:"top1_pct,omitempty"`
	Top5Pct      *float64   `json:"top5_pct,omitempty"`
	RetrievedPct *float64   `json:"retrieved_pct,omitempty"`
	CoveragePct  *float64   `json:"coverage_pct,omitempty"` // ranked / (ranked + absent)
	MRR          *float64   `json:"mrr,omitempty"`
	// FalsePositiveCases is out of FalsePositiveChecks: cases able to show one,
	// meaning no-answer cases and those listing forbidden titles.
	FalsePositiveCases  int `json:"false_positive_cases"`
	FalsePositiveChecks int `json:"false_positive_checks"`
}

// CaseResult is the per-case record compare uses to name ranking regressions.
type CaseResult struct {
	Name           string   `json:"name"`
	Query          string   `json:"query"`
	Kind           string   `json:"kind"`
	Category       string   `json:"category"`
	Status         string   `json:"status"`
	Rank           int      `json:"rank"`
	FalsePositives []string `json:"false_positives,omitempty"`
}

// readResult loads and checks a saved result, so that a corrupt or foreign file
// is reported as such rather than compared as if it were empty.
func readResult(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if err := r.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

func (r *Result) validate() error {
	switch {
	case r.Schema != schemaResult:
		return fmt.Errorf("not an sss-bench result (schema %q)", r.Schema)
	case r.SchemaVersion != schemaVersion:
		return fmt.Errorf("schema version %d is not supported by this harness (expected %d)", r.SchemaVersion, schemaVersion)
	case r.Status != statusComplete && r.Status != statusPartial && r.Status != statusFailed:
		return fmt.Errorf("unknown status %q", r.Status)
	case r.Workload.ID == "" || r.Workload.Profile == "":
		return errors.New("workload identity is missing")
	case r.Environment.OS == "" || r.Environment.Arch == "":
		return errors.New("environment is missing")
	}
	seen := map[string]bool{}
	for _, m := range r.Metrics {
		switch {
		case m.ID == "" || seen[m.ID]:
			return fmt.Errorf("missing or duplicate metric id %q", m.ID)
		case m.Status == metricOK && (m.Value == nil || m.N != len(m.Samples) || m.N == 0):
			return fmt.Errorf("metric %s: measured but without consistent samples", m.ID)
		case m.Status != metricOK && m.Status != metricUnavailable && m.Status != metricFailed:
			return fmt.Errorf("metric %s: unknown status %q", m.ID, m.Status)
		}
		seen[m.ID] = true
	}
	return nil
}
