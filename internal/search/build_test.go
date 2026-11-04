package search

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"unsafe"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

func buildTitles() []catalog.Title {
	return []catalog.Title{
		{ID: "tt0816692", Name: "Interstellar", Type: "movie", Year: 2014, Votes: 2300000, Rating: 8.7},
		{ID: "tt0944947", Name: "Game of Thrones", Type: "series", Year: 2011, Votes: 2400000, Rating: 9.2, Aliases: []string{"Le Trône de fer", "GoT"}},
		{ID: "tt0110912", Name: "Pulp Fiction", Type: "movie", Year: 1994, Votes: 2200000, Rating: 8.9},
		{ID: "tt0000002", Name: "Same Title", Type: "movie", Year: 1980, Votes: 200, Rating: 7.5},
		{ID: "tt0000003", Name: "Same Title", Type: "series", Year: 2020, Votes: 300, Rating: 9.2},
		{ID: "tt0245429", Name: "Spirited Away", Original: "千と千尋の神隠し", Type: "movie", Year: 2001, Votes: 900000, Rating: 8.6},
	}
}

// Build takes the catalogue's titles but must index exactly what New does.
func TestBuildMatchesNew(t *testing.T) {
	want := New(buildTitles())
	c := &catalog.Catalog{Titles: buildTitles()}
	got := Build(c)
	if c.Titles != nil {
		t.Fatal("Build left the titles on the catalogue")
	}
	for _, query := range []string{"interstellar", "game of thrones", "trone de fer", "got", "pulp fikshun", "same title", "千と千尋", "spirted", "gme thrn"} {
		for _, kind := range []string{"movie", "series"} {
			a := want.Search(context.Background(), query, kind, 20)
			b := got.Search(context.Background(), query, kind, 20)
			if !reflect.DeepEqual(a, b) {
				t.Errorf("%q %s: New %+v, Build %+v", query, kind, a, b)
			}
		}
	}
	if !slices.Equal(want.vocabulary, got.vocabulary) || !slices.Equal(want.grams, got.grams) ||
		!slices.Equal(want.names.keys, got.names.keys) || !slices.Equal(want.alternate.keys, got.alternate.keys) {
		t.Error("Build and New indexed different strings")
	}
}

func TestPackStringsSharesOneBacking(t *testing.T) {
	list := []string{"", "alpha", "beta", "beta", "日本", "", "gamma"}
	want := slices.Clone(list)
	packStrings(list)
	if !slices.Equal(list, want) {
		t.Fatalf("packStrings changed the strings: %q", list)
	}
	// Every non-empty string lies in one block, and equal neighbours share bytes.
	first := unsafe.StringData(list[1])
	end := uintptr(unsafe.Pointer(first)) + uintptr(len("alpha")+len("beta")+len("日本")+len("gamma"))
	for _, s := range list {
		if s == "" {
			continue
		}
		p := uintptr(unsafe.Pointer(unsafe.StringData(s)))
		if p < uintptr(unsafe.Pointer(first)) || p+uintptr(len(s)) > end {
			t.Errorf("%q is outside the packed block", s)
		}
	}
	if unsafe.StringData(list[2]) != unsafe.StringData(list[3]) {
		t.Error("equal neighbours were stored twice")
	}
}
