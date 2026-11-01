// Package probekit is shared by the sss-bench harness and the probes it compiles
// into the source tree under test: the timer, the process CPU reader and the
// request and response types of the probe protocol.
//
// Probes are built with go build -overlay against trees that may predate the
// harness, so this package may import only the standard library.
package probekit

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

// Resolution reports the smallest step of the timer seen in a short sample.
func Resolution() time.Duration {
	best := time.Duration(1<<63 - 1)
	for i := 0; i < 1000; i++ {
		a := Nanos()
		b := Nanos()
		for b == a {
			b = Nanos()
		}
		if d := time.Duration(b - a); d < best {
			best = d
		}
	}
	return best
}

// Query is one search of the workload.
type Query struct {
	Text string `json:"q"`
	Kind string `json:"k"`
}

// Limits mirrors what cmd/smart-search applies to itself at startup, so probes
// run under the same scheduler and memory settings as the shipped helper.
type Limits struct {
	GOMAXPROCS     int   `json:"gomaxprocs"`
	MemoryLimitMiB int64 `json:"memory_limit_mib"`
}

// Apply sets the limits on this process.
func (l Limits) Apply() {
	runtime.GOMAXPROCS(min(l.GOMAXPROCS, runtime.NumCPU()))
	debug.SetMemoryLimit(l.MemoryLimitMiB << 20)
}

// Order is the fixed, shuffled query order of one pass. The harness and the
// probes derive it independently, so a probe reports durations alone and the
// harness can still attribute each one to its query.
func Order(n int, seed uint64, pass int) []int {
	return rand.New(rand.NewPCG(seed, uint64(pass)+1)).Perm(n)
}

// EngineRequest asks the engine probe to measure a catalogue ("measure") or to
// return the results of a list of queries ("dump").
type EngineRequest struct {
	Mode         string  `json:"mode"`
	Catalog      string  `json:"catalog"`
	Queries      []Query `json:"queries"`
	Limit        int     `json:"limit"`
	Seed         uint64  `json:"seed"`
	WarmupPasses int     `json:"warmup_passes"`
	Passes       int     `json:"passes"`
	ThroughputMS int     `json:"throughput_ms"`
	Concurrency  []int   `json:"concurrency"`
	Rebuilds     int     `json:"rebuilds"`
	Limits       Limits  `json:"limits"`
}

// Hit is one search result.
type Hit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Throughput is a closed-loop run at one concurrency.
type Throughput struct {
	Concurrency int     `json:"concurrency"`
	Completed   int64   `json:"completed"`
	Seconds     float64 `json:"seconds"`
}

// EngineResponse holds raw measurements; the harness does the statistics.
type EngineResponse struct {
	ReadNS          int64        `json:"read_ns"`
	LoadNS          int64        `json:"load_ns"`
	LoadCPUNS       int64        `json:"load_cpu_ns"`
	IndexNS         int64        `json:"index_ns"`
	IndexCPUNS      int64        `json:"index_cpu_ns"`
	HeapAfterLoad   uint64       `json:"heap_after_load"`
	HeapAfterIndex  uint64       `json:"heap_after_index"`
	HeapAfterSearch uint64       `json:"heap_after_search"`
	ColdNS          []int64      `json:"cold_ns"`
	WarmNS          []int64      `json:"warm_ns"`
	AllocsPerSearch float64      `json:"allocs_per_search"`
	BytesPerSearch  float64      `json:"bytes_per_search"`
	Throughput      []Throughput `json:"throughput,omitempty"`
	RebuildNS       []int64      `json:"rebuild_ns,omitempty"`
	PeakResident    uint64       `json:"peak_resident,omitempty"`
	Results         [][]Hit      `json:"results,omitempty"`
}

// OpsRequest asks the catalogue-operations probe to parse local IMDb-shaped
// files ("prepare") or to run a live refresh from them ("refresh").
type OpsRequest struct {
	Mode    string `json:"mode"`
	IMDbDir string `json:"imdb_dir"`
	Profile string `json:"profile"`
	Output  string `json:"output,omitempty"`
	Catalog string `json:"catalog,omitempty"`
	DataDir string `json:"data_dir,omitempty"`
	// Query is searched throughout a refresh; it must return a result from the
	// catalogue being served.
	Query          string `json:"query,omitempty"`
	Kind           string `json:"kind,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Limits         Limits `json:"limits"`
}

// OpsResponse reports one catalogue operation.
type OpsResponse struct {
	ParseNS        int64  `json:"parse_ns,omitempty"`
	ParseCPUNS     int64  `json:"parse_cpu_ns,omitempty"`
	SaveNS         int64  `json:"save_ns,omitempty"`
	RefreshNS      int64  `json:"refresh_ns,omitempty"`
	RefreshCPUNS   int64  `json:"refresh_cpu_ns,omitempty"`
	Titles         int    `json:"titles"`
	FileBytes      int64  `json:"file_bytes,omitempty"`
	SearchesServed int    `json:"searches_served,omitempty"`
	SearchesEmpty  int    `json:"searches_empty,omitempty"`
	PeakResident   uint64 `json:"peak_resident,omitempty"`
}

// Serve runs a probe: it decodes the request file named by the first argument,
// handles it and writes the response to stdout.
func Serve[Request, Response any](handle func(Request) (Response, error)) {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: probe request.json")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	response, err := handle(request)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
