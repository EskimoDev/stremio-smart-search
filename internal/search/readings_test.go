package search

import (
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// Queries with several misspelled words, from real-data misses on 2026-11-02
// main. Titles are real IMDb entries; vote counts are rounded, illustrative
// values.
func multiTypoEngine() *Engine {
	return New([]catalog.Title{
		{ID: "tt0944947", Name: "Game of Thrones", Type: "series", Year: 2011, Votes: 2500000},
		{ID: "tt1520211", Name: "The Walking Dead", Type: "series", Year: 2010, Votes: 1100000},
		{ID: "tt0903747", Name: "Breaking Bad", Type: "series", Year: 2008, Votes: 2300000},
		{ID: "tt2861424", Name: "Rick and Morty", Type: "series", Year: 2013, Votes: 650000},
		{ID: "tt0182576", Name: "Family Guy", Type: "series", Year: 1999, Votes: 380000},
		{ID: "tt1442437", Name: "Modern Family", Type: "series", Year: 2009, Votes: 480000},
		{ID: "tt0111161", Name: "The Shawshank Redemption", Type: "movie", Year: 1994, Votes: 3000000},
		{ID: "tt1392190", Name: "Mad Max: Fury Road", Type: "movie", Year: 2015, Votes: 1100000},
		{ID: "tt0079501", Name: "Mad Max", Type: "movie", Year: 1979, Votes: 230000},
		{ID: "tt0110357", Name: "The Lion King", Type: "movie", Year: 1994, Votes: 1200000},
		{ID: "tt0088763", Name: "Back to the Future", Type: "movie", Year: 1985, Votes: 1300000},
		// Decoys: titles that share one word of a query.
		{ID: "tt0090555", Name: "Crocodile Dundee", Type: "movie", Year: 1986, Votes: 120000},
		{ID: "tt0000001", Name: "Max Payne", Type: "movie", Year: 2008, Votes: 140000},
		{ID: "tt1219827", Name: "Ghost in the Shell", Type: "movie", Year: 2017, Votes: 230000},
		{ID: "tt0436992", Name: "Doctor Who", Type: "series", Year: 2005, Votes: 260000},
		{ID: "tt0108778", Name: "Friends", Type: "series", Year: 1994, Votes: 1100000},
		{ID: "tt0112130", Name: "Pride and Prejudice", Type: "series", Year: 1995, Votes: 45000},
		{ID: "tt0081505", Name: "The Shining", Type: "movie", Year: 1980, Votes: 1100000},
		{ID: "tt0000002", Name: "King Kong", Type: "movie", Year: 2005, Votes: 450000},
		{ID: "tt0000003", Name: "Gmae Night", Type: "movie", Year: 2001, Votes: 10},
	})
}

func TestSeveralMisspelledWords(t *testing.T) {
	e := multiTypoEngine()
	for _, tc := range []struct{ query, kind, want string }{
		{"gma e of thr", "series", "tt0944947"},         // a letter pushed across a space, last word cut short
		{"gam eof thrones", "series", "tt0944947"},      // the space one letter late
		{"the wakling ded", "series", "tt1520211"},      // a three-letter word with a letter missing
		{"brekaing bda", "series", "tt0903747"},         // two typos, one in a three-letter word
		{"rik and mortey", "series", "tt2861424"},       // both words misspelled
		{"family gyu", "series", "tt0182576"},           // a swapped pair in a three-letter word
		{"teh lion king", "movie", "tt0110357"},         // a misspelled stop word
		{"shaw shank redemption", "movie", "tt0111161"}, // a word split in two
		{"mad maxfury road", "movie", "tt1392190"},      // two words run together
		{"bak to teh futre", "movie", "tt0088763"},      // three misspelled words
	} {
		if got := first(t, e, tc.query, tc.kind); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%q: wanted %s first, got %v", tc.query, tc.want, got)
		}
	}
}

// Leaving a word out must not turn two unrelated words into a match: in a
// two-word query only a short word may be dropped, and the word that is kept
// must be spelled close to the title's.
func TestDroppedWordLimits(t *testing.T) {
	e := multiTypoEngine()
	for _, tc := range []struct{ query, kind string }{
		{"zarnet frends", "series"}, // a long unknown word is what the query is about
		{"qzx glorpwhistle", "series"},
	} {
		if got := first(t, e, tc.query, tc.kind); len(got) != 0 {
			t.Errorf("%q: wanted no results, got %v", tc.query, got)
		}
	}
	// A title matching every word still beats one matching all but one.
	if got := first(t, e, "max payne", "movie"); len(got) == 0 || got[0] != "tt0000001" {
		t.Errorf("max payne: got %v", got)
	}
}
