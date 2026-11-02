package search

import (
	"context"
	"slices"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

func TestSkeletonOf(t *testing.T) {
	for word, want := range map[string]string{
		"stranger":   "strngr",
		"succession": "scsn",
		"sccssn":     "scsn",
		"mirror":     "mr",
		"mrrr":       "mr",
		"mystery":    "mstr",
		"yellow":     "ylw",
		"peaky":      "pk",
		"inception":  "ncptn",
	} {
		got, ok := skeletonOf(nil, word)
		if !ok || string(got) != want {
			t.Errorf("skeletonOf(%q) = %q, %v; want %q", word, got, ok, want)
		}
		for _, other := range []string{"", "m", "mr", "ncpt", "ncptnz", "scsn", "strngr", "zz"} {
			if c, w := compareSkeleton(word, []byte(other)), compareString(want, other); c != w {
				t.Errorf("compareSkeleton(%q, %q) = %d; want %d", word, other, c, w)
			}
		}
	}
	if _, ok := skeletonOf(nil, "café"); ok {
		t.Error("non-ASCII word given a skeleton")
	}
}

func compareString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func vowelEngine() *Engine {
	return New([]catalog.Title{
		{ID: "tt0944947", Name: "Game of Thrones", Type: "series", Year: 2011, Votes: 2500000},
		{ID: "tt4574334", Name: "Stranger Things", Type: "series", Year: 2016, Votes: 1400000},
		{ID: "tt0108778", Name: "Friends", Type: "series", Year: 1994, Votes: 1100000},
		{ID: "tt0068646", Name: "The Godfather", Type: "movie", Year: 1972, Votes: 2100000},
		{ID: "tt1375666", Name: "Inception", Type: "movie", Year: 2010, Votes: 2600000},
		{ID: "tt2085059", Name: "Black Mirror", Type: "series", Year: 2011, Votes: 650000},
		{ID: "tt0112130", Name: "Fiends", Type: "movie", Year: 1995, Votes: 50},
		{ID: "tt0000001", Name: "Strange Things", Type: "movie", Year: 1999, Votes: 40},
	})
}

func TestDroppedVowels(t *testing.T) {
	e := vowelEngine()
	for _, tc := range []struct{ query, kind, want string }{
		{"strngr thngs", "series", "tt4574334"},
		{"gme thron", "series", "tt0944947"}, // "thron" is also cut short
		{"frnds", "series", "tt0108778"},
		{"gdfthr", "movie", "tt0068646"},
		{"incptn", "movie", "tt1375666"},
		{"blck mrrr", "series", "tt2085059"},
	} {
		if got := first(t, e, tc.query, tc.kind); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s: wanted %s first, got %v", tc.query, tc.want, got)
		}
	}
	// "frnds" is not in "fiends": every typed letter must be in the word.
	if got := first(t, e, "frnds", "movie"); len(got) != 0 {
		t.Errorf("frnds matched a word without its letters: %v", got)
	}
}

func TestSkeletonOnlyForVowelPoorWords(t *testing.T) {
	e := vowelEngine()
	// A word typed with its vowels is not read as a dropped-vowel spelling of
	// another word with the same consonants.
	for _, query := range []string{"strange", "fiends"} {
		for _, m := range e.terms(context.Background(), query, true, true) {
			if m.weight == skeletonWeight || m.weight == skeletonLongerWeight {
				t.Errorf("%s: skeleton match %q", query, e.vocabulary[m.word])
			}
		}
	}
	// A word the query already matches by spelling keeps its weight.
	terms := e.terms(context.Background(), "thngs", true, true)
	n := 0
	for _, m := range terms {
		if e.vocabulary[m.word] == "things" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("things listed %d times: %v", n, terms)
	}
	if !slices.IsSortedFunc(e.skeletons, func(a, b int32) int {
		x, _ := skeletonOf(nil, e.vocabulary[a])
		y, _ := skeletonOf(nil, e.vocabulary[b])
		return compareString(string(x), string(y))
	}) {
		t.Error("skeletons out of order")
	}
}
