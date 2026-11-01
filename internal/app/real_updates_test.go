package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// TestRealIMDbFreshnessCheck runs the metadata check against IMDb's real
// server: HEAD requests only, nothing is downloaded. It also reports what
// this computer's connection-cost detection says. Opt in with
// SMART_SEARCH_REAL_IMDB=1.
func TestRealIMDbFreshnessCheck(t *testing.T) {
	if os.Getenv("SMART_SEARCH_REAL_IMDB") != "1" {
		t.Skip("set SMART_SEARCH_REAL_IMDB=1 to contact IMDb (HEAD requests only)")
	}
	t.Logf("network cost: %s", networkCost())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	files := catalog.DatasetFiles(catalog.AliasOptions{Language: "fr"})
	// No stored validators: the check cannot tell.
	if got, err := catalog.IMDb.Check(ctx, files, nil); err != nil || got != catalog.Unknown {
		t.Fatalf("without validators: %v %v", got, err)
	}
	// Stale validators: changed.
	stale := []catalog.SourceFile{}
	for _, f := range files {
		stale = append(stale, catalog.SourceFile{File: f, ETag: `"0000"`})
	}
	if got, err := catalog.IMDb.Check(ctx, files, stale); err != nil || got != catalog.Changed {
		t.Fatalf("stale ETag: %v %v", got, err)
	}
	// Current validators, read from a plain HEAD: unchanged (HTTP 304).
	current := []catalog.SourceFile{}
	client := catalog.IMDb
	for _, f := range files {
		info, err := client.Head(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: ETag %s, Last-Modified %s, %d bytes", f, info.ETag, info.LastModified, info.Size)
		current = append(current, info)
	}
	start := time.Now()
	if got, err := catalog.IMDb.Check(ctx, files, current); err != nil || got != catalog.Unchanged {
		t.Fatalf("current validators: %v %v", got, err)
	}
	t.Logf("unchanged check of %d files took %v", len(files), time.Since(start))
}
