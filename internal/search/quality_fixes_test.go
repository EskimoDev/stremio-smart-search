package search

import (
	"context"
	"slices"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// Regression examples for the ranking fixes measured with tools/quality. The
// titles are real IMDb entries; vote counts are rounded, illustrative values.
func smallEngine() *Engine {
	return New([]catalog.Title{
		{ID: "tt1160419", Name: "Dune: Part One", Original: "Dune", Type: "movie", Year: 2021, Votes: 950000},
		{ID: "tt0087182", Name: "Dune", Type: "movie", Year: 1984, Votes: 200000},
		{ID: "tt1856101", Name: "Blade Runner 2049", Type: "movie", Year: 2017, Votes: 700000},
		{ID: "tt0083658", Name: "Blade Runner", Type: "movie", Year: 1982, Votes: 850000},
		{ID: "tt1396484", Name: "It", Type: "movie", Year: 2017, Votes: 650000},
		{ID: "tt0099864", Name: "It", Type: "series", Year: 1990, Votes: 150000},
		{ID: "tt0944947", Name: "Game of Thrones", Type: "series", Year: 2011, Votes: 2500000},
		{ID: "tt0317676", Name: "House of the Dead", Type: "movie", Year: 2003, Votes: 40000},
		{ID: "tt0425112", Name: "Hot Fuzz", Type: "movie", Year: 2007, Votes: 570000},
		{ID: "tt0955308", Name: "Robin Hood", Type: "movie", Year: 2010, Votes: 300000},
		{ID: "tt0120737", Name: "The Lord of the Rings: The Fellowship of the Ring", Type: "movie", Year: 2001, Votes: 2200000,
			Aliases: []string{"El señor de los anillos: La comunidad del anillo"}},
	})
}

func first(t *testing.T, e *Engine, query, kind string) []string {
	t.Helper()
	return ids(e.Search(context.Background(), query, kind, 20))
}

func TestYearNarrowsSimilarTitles(t *testing.T) {
	e := smallEngine()
	for _, tc := range []struct{ query, kind, want string }{
		{"dune 1984", "movie", "tt0087182"},
		{"dune 2021", "movie", "tt1160419"},
		{"it 2017", "movie", "tt1396484"},
		{"blade runner 2049", "movie", "tt1856101"}, // a year that is part of the title
		{"blade runner 1982", "movie", "tt0083658"},
	} {
		if got := first(t, e, tc.query, tc.kind); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s: wanted %s first, got %v", tc.query, tc.want, got)
		}
	}
	// A year matching neither film is ignored rather than emptying the row.
	if got := first(t, e, "dune 1999", "movie"); len(got) != 2 {
		t.Errorf("unmatched year: %v", got)
	}
	// On its own a year is an ordinary word, never "every film from 2017".
	if got := first(t, e, "2017", "movie"); slices.Contains(got, "tt1396484") {
		t.Errorf("bare year matched by release year: %v", got)
	}
}

func TestLongWordTranspositionAndExtraLetter(t *testing.T) {
	e := smallEngine()
	for _, q := range []string{"game of thornes", "game of thrrones"} {
		if got := first(t, e, q, "series"); len(got) == 0 || got[0] != "tt0944947" {
			t.Errorf("%s: %v", q, got)
		}
	}
	var seen []string
	forEachSwapOrDrop("abc", func(v []byte) { seen = append(seen, string(v)) })
	if want := []string{"bc", "bac", "ac", "acb", "ab"}; !slices.Equal(seen, want) {
		t.Fatalf("edits %v, want %v", seen, want)
	}
}

func TestLongTranslatedTitle(t *testing.T) {
	// Nine words, but only six are looked up; the word bound must not reject it.
	got := first(t, smallEngine(), "el señor de los anillos la comunidad del anillo", "movie")
	if len(got) == 0 || got[0] != "tt0120737" {
		t.Fatalf("got %v", got)
	}
}

func TestShortAcronymIsNotFuzzed(t *testing.T) {
	e := smallEngine()
	if got := first(t, e, "hotd", "movie"); !slices.Equal(got, []string{"tt0317676"}) {
		t.Fatalf("hotd: one-letter variants listed with the acronym: %v", got)
	}
	// Words that are not an acronym still get typo matching.
	if got := first(t, e, "hood", "movie"); len(got) == 0 || got[0] != "tt0955308" {
		t.Fatalf("hood: %v", got)
	}
	if got := first(t, e, "fuzzz", "movie"); len(got) == 0 || got[0] != "tt0425112" {
		t.Fatalf("fuzzz: %v", got)
	}
}

func TestExactRegionalTitleRanksByPopularity(t *testing.T) {
	e := New([]catalog.Title{
		{ID: "tt0111161", Name: "The Shawshank Redemption", Type: "movie", Year: 1994, Votes: 3000000, Aliases: []string{"Cadena perpetua"}},
		{ID: "tt0077287", Name: "Life Sentence", Original: "Cadena perpetua", Type: "movie", Year: 1979, Votes: 500},
	})
	if got := first(t, e, "cadena perpetua", "movie"); !slices.Equal(got, []string{"tt0111161", "tt0077287"}) {
		t.Fatalf("got %v", got)
	}
}
