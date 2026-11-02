// Command engine is the sss-bench direct-engine probe. In a fresh process it
// loads a catalogue, builds the index and times searches with no HTTP involved.
//
// The harness compiles it against the source tree under test, which may be an
// older commit, so it depends only on catalog.Load, search.New and Engine.Search.
// Everything else it reports is raw; the harness does the statistics.
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/search"
	"github.com/EskimoDev/stremio-smart-search/tools/sss-bench/probes/probekit"
)

func main() { probekit.Serve(handle) }

func handle(request probekit.EngineRequest) (probekit.EngineResponse, error) {
	request.Limits.Apply()
	switch request.Mode {
	case "prime":
		return probekit.EngineResponse{}, nil
	case "measure":
		return measure(request)
	case "dump":
		return dump(request)
	}
	return probekit.EngineResponse{}, fmt.Errorf("unknown mode %q", request.Mode)
}

// dump returns every query's results, with no time budget, so search quality
// does not depend on how busy the machine is.
func dump(request probekit.EngineRequest) (probekit.EngineResponse, error) {
	c, err := catalog.Load(request.Catalog)
	if err != nil {
		return probekit.EngineResponse{}, err
	}
	engine := search.New(c.Titles)
	results := make([][]probekit.Hit, len(request.Queries))
	for i, q := range request.Queries {
		titles := engine.Search(context.Background(), q.Text, q.Kind, request.Limit)
		results[i] = make([]probekit.Hit, len(titles))
		for j, t := range titles {
			results[i][j] = probekit.Hit{ID: t.ID, Name: t.Name}
		}
	}
	return probekit.EngineResponse{Results: results}, nil
}

func heap() uint64 {
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return memory.HeapAlloc
}

func measure(request probekit.EngineRequest) (probekit.EngineResponse, error) {
	var r probekit.EngineResponse
	started := probekit.Nanos()
	data, err := os.ReadFile(request.Catalog)
	if err != nil {
		return r, err
	}
	r.ReadNS = probekit.Nanos() - started
	data = nil
	_ = data

	cpu := probekit.ProcessCPU()
	started = probekit.Nanos()
	c, err := catalog.Load(request.Catalog)
	if err != nil {
		return r, err
	}
	r.LoadNS = probekit.Nanos() - started
	r.LoadCPUNS = int64(probekit.ProcessCPU() - cpu)
	r.HeapAfterLoad = heap()

	cpu = probekit.ProcessCPU()
	started = probekit.Nanos()
	engine := search.New(c.Titles)
	r.IndexNS = probekit.Nanos() - started
	r.IndexCPUNS = int64(probekit.ProcessCPU() - cpu)
	// Match App.Load: drop the parsed copy and return it to the OS once.
	c = nil
	debug.FreeOSMemory()
	r.HeapAfterIndex = heap()

	ctx := context.Background()
	timed := func(pass int, into []int64) []int64 {
		for _, i := range probekit.Order(len(request.Queries), request.Seed, pass) {
			q := request.Queries[i]
			started := probekit.Nanos()
			engine.Search(ctx, q.Text, q.Kind, request.Limit)
			into = append(into, probekit.Nanos()-started)
		}
		return into
	}
	// The first pass sees each query for the first time in this process.
	r.ColdNS = timed(0, make([]int64, 0, len(request.Queries)))
	for pass := 1; pass <= request.WarmupPasses; pass++ {
		timed(pass, make([]int64, 0, len(request.Queries)))
	}
	r.WarmNS = make([]int64, 0, request.Passes*len(request.Queries))
	first := request.WarmupPasses + 1
	for pass := first; pass < first+request.Passes; pass++ {
		r.WarmNS = timed(pass, r.WarmNS)
	}

	// Allocation is measured on its own pass: ReadMemStats stops the world.
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for _, q := range request.Queries {
		engine.Search(ctx, q.Text, q.Kind, request.Limit)
	}
	runtime.ReadMemStats(&after)
	searches := float64(len(request.Queries))
	r.AllocsPerSearch = float64(after.Mallocs-before.Mallocs) / searches
	r.BytesPerSearch = float64(after.TotalAlloc-before.TotalAlloc) / searches

	for _, concurrency := range request.Concurrency {
		r.Throughput = append(r.Throughput, throughput(engine, request, concurrency))
	}
	r.HeapAfterSearch = heap()

	for i := 0; i < request.Rebuilds; i++ {
		c, err := catalog.Load(request.Catalog)
		if err != nil {
			return r, err
		}
		started := probekit.Nanos()
		replacement := search.New(c.Titles)
		r.RebuildNS = append(r.RebuildNS, probekit.Nanos()-started)
		// The old index stays reachable until now, as during a live refresh.
		engine = replacement
		c = nil
		debug.FreeOSMemory()
	}
	runtime.KeepAlive(engine)
	r.PeakResident = probekit.PeakResident()
	return r, nil
}

// throughput runs closed-loop searches. It reads the clock every few searches
// so that timing does not become the workload.
func throughput(engine *search.Engine, request probekit.EngineRequest, concurrency int) probekit.Throughput {
	var completed atomic.Int64
	var workers sync.WaitGroup
	started := probekit.Nanos()
	deadline := started + int64(request.ThroughputMS)*1e6
	for worker := 0; worker < concurrency; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			order := probekit.Order(len(request.Queries), request.Seed, 1000+worker)
			var done int64
			for i := 0; ; i++ {
				if i%8 == 0 && probekit.Nanos() >= deadline {
					break
				}
				q := request.Queries[order[i%len(order)]]
				engine.Search(context.Background(), q.Text, q.Kind, request.Limit)
				done++
			}
			completed.Add(done)
		}(worker)
	}
	workers.Wait()
	return probekit.Throughput{Concurrency: concurrency, Completed: completed.Load(), Seconds: float64(probekit.Nanos()-started) / 1e9}
}
