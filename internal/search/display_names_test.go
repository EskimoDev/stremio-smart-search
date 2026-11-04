package search

import (
	"context"
	"strings"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

func TestDisplayNamesPreserveResultMetadata(t *testing.T) {
	titles := []catalog.Title{
		{ID: "tt0000001", Name: "Été 日本 — <A&B>", Type: "movie", Year: 2001, Votes: 100, Rating: 8.1},
		{ID: "tt0000002", Name: "Same Title", Type: "movie", Year: 1980, Votes: 200, Rating: 7.5},
		{ID: "tt0000003", Name: "Same Title", Type: "series", Year: 2020, Votes: 300, Rating: 9.2},
		{ID: "tt0000004", Name: strings.Repeat("N", 1000), Original: "A very long display name", Type: "movie", Year: 2014, Votes: 400, Rating: 6.7},
		{ID: "tt184467440737095516", Name: "Wide & Precise", Type: "movie", Year: 2027, Votes: 17, Rating: 8.1234567},
	}
	engine := New(titles)
	for _, want := range titles {
		query := want.Name
		if want.Original != "" {
			query = want.Original
		}
		results := engine.Search(context.Background(), query, want.Type, 20)
		found := false
		for _, got := range results {
			if got.ID != want.ID {
				continue
			}
			found = true
			if got.Name != want.Name || got.Type != want.Type || got.Year != want.Year || got.Votes != want.Votes || got.Rating != want.Rating {
				t.Errorf("%s: result metadata changed: %+v", want.ID, got)
			}
		}
		if !found {
			t.Errorf("%s: could not retrieve %q using %q", want.ID, want.Name, query)
		}
	}
}
