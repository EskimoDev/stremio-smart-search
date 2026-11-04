package app

import (
	"runtime/debug"
	"testing"
)

func TestLowerMemoryLimitForLoad(t *testing.T) {
	previous := debug.SetMemoryLimit(256 << 20)
	defer debug.SetMemoryLimit(previous)

	first := lowerMemoryLimitForLoad()
	if got := debug.SetMemoryLimit(-1); got != loadingMemoryLimit {
		t.Fatalf("limit while loading = %d, want %d", got, loadingMemoryLimit)
	}
	second := lowerMemoryLimitForLoad() // a standby wake overlapping startup
	first()
	first() // restoring twice must not release the other load's lowering
	if got := debug.SetMemoryLimit(-1); got != loadingMemoryLimit {
		t.Fatalf("limit after one of two loads = %d, want %d", got, loadingMemoryLimit)
	}
	second()
	if got := debug.SetMemoryLimit(-1); got != 256<<20 {
		t.Fatalf("limit after loading = %d, want %d", got, 256<<20)
	}

	// A lower limit someone chose is never raised.
	debug.SetMemoryLimit(64 << 20)
	restore := lowerMemoryLimitForLoad()
	if got := debug.SetMemoryLimit(-1); got != 64<<20 {
		t.Fatalf("limit while loading under a lower one = %d", got)
	}
	restore()
	if got := debug.SetMemoryLimit(-1); got != 64<<20 {
		t.Fatalf("limit after loading under a lower one = %d", got)
	}
}
