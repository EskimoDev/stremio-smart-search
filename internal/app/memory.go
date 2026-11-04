package app

import (
	"runtime/debug"
	"sync"
)

// loadingMemoryLimit is the Go memory limit while a saved catalogue is loaded
// and indexed with no other index in memory: at startup and when standby
// wakes. Loading holds the parsed catalogue and the half-built index at once,
// and with the usual collector pacing the heap grows to about twice that
// before a collection, so the largest catalogue (Expanded with alternative
// titles) peaked at the 256 MiB process limit for a moment. Its live memory
// during the build stays near 125 MiB, so a 200 MiB limit lets the collector
// run a little earlier without costing time measurably, and the peak falls
// with it. Smaller catalogues never reach it. The process limit returns
// once the index is built.
const loadingMemoryLimit = 200 << 20

var loading struct {
	sync.Mutex
	holders  int
	previous int64
}

// lowerMemoryLimitForLoad applies loadingMemoryLimit, unless the limit is
// already lower, until the returned function is called. Overlapping loads
// share one lowering, and the last one to finish restores the limit.
func lowerMemoryLimitForLoad() (restore func()) {
	loading.Lock()
	defer loading.Unlock()
	if loading.holders == 0 {
		loading.previous = debug.SetMemoryLimit(-1) // reads without changing
		debug.SetMemoryLimit(min(loading.previous, loadingMemoryLimit))
	}
	loading.holders++
	var once sync.Once
	return func() {
		once.Do(func() {
			loading.Lock()
			defer loading.Unlock()
			if loading.holders--; loading.holders == 0 {
				debug.SetMemoryLimit(loading.previous)
			}
		})
	}
}
