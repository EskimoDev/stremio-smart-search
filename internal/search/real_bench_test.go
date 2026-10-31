package search

import (
	"context"
	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"os"
	"runtime"
	"testing"
	"time"
)

func BenchmarkLocalCatalogue(b *testing.B) {
	path := os.Getenv("SMART_SEARCH_BENCH_CATALOG")
	if path == "" {
		b.Skip("set SMART_SEARCH_BENCH_CATALOG to an explicitly selected local snapshot")
	}
	c, err := catalog.Load(path)
	if err != nil {
		b.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	engine := New(c.Titles)
	build := time.Since(started)
	runtime.GC()
	runtime.ReadMemStats(&after)
	b.Logf("%d real titles; index built in %s; added live Go heap %.1f MiB", len(c.Titles), build, float64(after.HeapAlloc-before.HeapAlloc)/(1<<20))
	for _, test := range []struct{ query, kind string }{{"hotd", "series"}, {"house of dragom", "series"}, {"interstelar", "movie"}, {"lotr", "movie"}, {"spiderman", "movie"}, {"man", "movie"}, {"the", "movie"}, {"qzxqvvnonexistent", "movie"}} {
		b.Run(test.query, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				engine.Search(context.Background(), test.query, test.kind, 20)
			}
		})
	}
}
