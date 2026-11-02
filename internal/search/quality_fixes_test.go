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

func TestCoveredTitleWinsCloseMatches(t *testing.T) {
	// Real IMDb titles; vote counts rounded. Terminator 2 is more popular, but
	// "the terminater" covers every word of The Terminator.
	e := New([]catalog.Title{
		{ID: "tt0103064", Name: "Terminator 2: Judgment Day", Type: "movie", Year: 1991, Votes: 1200000},
		{ID: "tt0088247", Name: "The Terminator", Type: "movie", Year: 1984, Votes: 950000},
	})
	if got := first(t, e, "the terminater", "movie"); len(got) != 2 || got[0] != "tt0088247" {
		t.Fatalf("got %v", got)
	}
	// Exact names keep their band: the bonus never lifts a word match over one.
	if got := first(t, e, "terminator 2 judgment day", "movie"); got[0] != "tt0103064" {
		t.Fatalf("exact: %v", got)
	}
}

// A short query word used to stand only for the first 20 vocabulary words it
// starts, alphabetically, so "war" never reached "wars" in a real catalogue
// and "star war" found nothing. Completions are now chosen by popularity.
func TestShortWordPrefixPrefersPopularCompletions(t *testing.T) {
	titles := []catalog.Title{
		{ID: "tt0076759", Name: "Star Wars: Episode IV - A New Hope", Type: "movie", Year: 1977, Votes: 1500000},
		{ID: "tt0944947", Name: "Game of Thrones", Type: "series", Year: 2011, Votes: 2500000},
		{ID: "tt10872600", Name: "Spider-Man: No Way Home", Type: "movie", Year: 2021, Votes: 1080000},
		{ID: "tt2250912", Name: "Spider-Man: Homecoming", Type: "movie", Year: 2017, Votes: 845000},
	}
	// Obscure titles whose words sort before "wars" and "thrones".
	for i, word := range []string{"war", "thr"} {
		for n := 0; n < 40; n++ {
			suffix := string(rune('a'+n/26)) + string(rune('a'+n%26))
			titles = append(titles, catalog.Title{ID: "tt9" + string(rune('0'+i)) + string(rune('0'+n/10)) + string(rune('0'+n%10)) + "000",
				Name: "Star Game " + word + "a" + suffix, Type: "movie", Year: 2000, Votes: 10})
		}
	}
	e := New(titles)
	for _, tc := range []struct{ query, kind, want string }{
		{"star war", "movie", "tt0076759"},
		{"game of thr", "series", "tt0944947"},
		// A completion one letter on outweighs a longer one: "hom" is "home".
		{"spidr man no wy hom", "movie", "tt10872600"},
	} {
		if got := first(t, e, tc.query, tc.kind); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s: wanted %s first, got %v", tc.query, tc.want, got)
		}
	}
}
