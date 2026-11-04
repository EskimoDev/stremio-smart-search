package search

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

func TestLongExactNamesRespectTypeAndLimit(t *testing.T) {
	for _, name := range []string{
		"Don't Be a Menace to South Central While Drinking Your Juice in the Hood",
		strings.Repeat("the ", 17) + "odyssey",
		strings.Repeat("extraordinary ", 10) + "odyssey",
		strings.Repeat("é", 256), // 512 bytes, the HTTP route's input ceiling
	} {
		t.Run(name, func(t *testing.T) {
			e := New([]catalog.Title{
				{ID: "tt0000001", Name: name, Type: "movie", Year: 1996, Votes: 100, Rating: 6.5},
				{ID: "tt0000002", Name: name, Type: "movie", Year: 2000, Votes: 10},
				{ID: "tt0000003", Name: name, Type: "series", Year: 2010, Votes: 1000},
			})
			got := e.Search(context.Background(), name, "movie", 1)
			if len(got) != 1 || got[0].ID != "tt0000001" || got[0].Name != name || got[0].Year != 1996 || got[0].Rating != 6.5 {
				t.Fatalf("exact name or metadata lost: %+v", got)
			}
			if got := first(t, e, name, "series"); !slices.Equal(got, []string{"tt0000003"}) {
				t.Fatalf("crossed media types: %v", got)
			}
			if got := e.Search(context.Background(), name, "movie", 0); len(got) != 0 {
				t.Fatalf("zero result limit ignored: %+v", got)
			}
		})
	}
}

func TestLongOriginalAndAlternativeNames(t *testing.T) {
	original := "Everything You Always Wanted to Know About Sex But Were Afraid to Ask"
	alias := "The 100 Year Old Man Who Climbed Out the Window and Disappeared"
	e := New([]catalog.Title{
		{ID: "tt0000001", Name: "First Feature", Original: original, Type: "movie", Votes: 100},
		{ID: "tt0000002", Name: "Second Feature", Aliases: []string{alias}, Type: "movie", Votes: 200},
	})
	for _, tc := range []struct{ query, id, name string }{
		{original, "tt0000001", "First Feature"},
		{alias, "tt0000002", "Second Feature"},
	} {
		got := e.Search(context.Background(), tc.query, "movie", 20)
		if len(got) != 1 || got[0].ID != tc.id || got[0].Name != tc.name {
			t.Errorf("%q: got %+v", tc.query, got)
		}
	}
}

func TestLongInputDoesNotEnableFuzzyMatching(t *testing.T) {
	name := "Don't Be a Menace to South Central While Drinking Your Juice in the Hood"
	oversized := strings.Repeat("x", 513)
	e := New([]catalog.Title{
		{ID: "tt0000001", Name: name, Type: "movie"},
		{ID: "tt0000002", Name: oversized, Type: "movie"},
		{ID: "tt0000003", Name: strings.Repeat("é", 257), Type: "movie"},
	})
	for _, q := range []string{
		strings.Replace(name, "Menace", "Menacc", 1),
		oversized,
		strings.Repeat("é", 257),
		strings.Repeat("qzxqvv ", 20),
	} {
		if got := first(t, e, q, "movie"); len(got) != 0 {
			t.Errorf("unbounded or fuzzy input %q: %v", q, got)
		}
	}
}

func TestAcronymWithYear(t *testing.T) {
	e := New([]catalog.Title{
		{ID: "tt0000001", Name: "Action Cinema Example", Type: "movie", Year: 2001, Votes: 100},
		{ID: "tt0000002", Name: "Another Cinema Example", Type: "movie", Year: 2002, Votes: 10000},
		{ID: "tt0000003", Name: "Action Cinema Example", Type: "series", Year: 2001, Votes: 100000},
		{ID: "tt0000004", Name: "Foreign Feature", Original: "Alternative Cinema Example", Type: "movie", Year: 1990, Votes: 1000},
		{ID: "tt0000005", Name: "Other Feature", Aliases: []string{"A.C.E."}, Type: "movie", Year: 1995, Votes: 500},
	})
	for _, tc := range []struct{ query, kind, id string }{
		{"ace 2001", "movie", "tt0000001"},
		{"ACE (2002)", "movie", "tt0000002"},
		{"ace 2001", "series", "tt0000003"},
		{"ace 1990", "movie", "tt0000004"},
		{"ace 1995", "movie", "tt0000005"},
		{"ace 2003", "movie", ""},
		{"ace 2002", "series", ""},
	} {
		got := first(t, e, tc.query, tc.kind)
		if tc.id == "" {
			if len(got) != 0 {
				t.Errorf("%q (%s): mismatched year or type: %v", tc.query, tc.kind, got)
			}
		} else if !slices.Equal(got, []string{tc.id}) {
			t.Errorf("%q (%s): got %v, want only %s", tc.query, tc.kind, got, tc.id)
		}
	}
	real := fixture(t)
	for _, tc := range []struct{ query, kind, id string }{
		{"got 2011", "series", "tt0944947"},
		{"lotr 2001", "movie", "tt0120737"},
		{"ＧＯＴ ２０１１", "series", "tt0944947"},
	} {
		if got := first(t, real, tc.query, tc.kind); len(got) == 0 || got[0] != tc.id {
			t.Errorf("%q: wanted %s first, got %v", tc.query, tc.id, got)
		}
	}
}

func TestEquivalentCharactersInCatalogueNames(t *testing.T) {
	e := New([]catalog.Title{
		{ID: "tt0000001", Name: "Ｓｅ７ｅｎ", Type: "movie", Votes: 100},
		{ID: "tt0000002", Name: "First Feature", Original: "Space Saga ٢", Type: "movie", Votes: 100},
		{ID: "tt0000003", Name: "Second Feature", Aliases: []string{"Ｐｌａｎｅｔ Ⅱ"}, Type: "movie", Votes: 100},
	})
	for _, tc := range []struct{ query, id, name string }{
		{"se7en", "tt0000001", "Ｓｅ７ｅｎ"},
		{"space saga 2", "tt0000002", "First Feature"},
		{"planet ii", "tt0000003", "Second Feature"},
	} {
		got := e.Search(context.Background(), tc.query, "movie", 20)
		if len(got) == 0 || got[0].ID != tc.id || got[0].Name != tc.name {
			t.Errorf("%q: indexing and querying disagree or display name changed: %+v", tc.query, got)
		}
	}
}

func TestEquivalentCharactersKeepCanonicalResults(t *testing.T) {
	e := New([]catalog.Title{
		{ID: "tt0816692", Name: "Interstellar", Type: "movie", Votes: 2000000},
		{ID: "tt0114369", Name: "Seven", Original: "Se7en", Type: "movie", Votes: 2000000},
		{ID: "tt1160419", Name: "Dune", Type: "movie", Year: 2021, Votes: 1000000},
		{ID: "tt15239678", Name: "Dune: Part Two", Type: "movie", Year: 2024, Votes: 800000},
	})
	for _, tc := range []struct{ query, plain string }{
		{"Ｉｎｔｅｒｓｔｅｌｌａｒ", "Interstellar"},
		{"Ｓｅ７ｅｎ", "Se7en"},
		{"dune ２", "dune 2"},
		{"dune ٢", "dune 2"},
		{"dune ۲", "dune 2"},
		{"dune Ⅱ", "dune ii"},
	} {
		got := e.Search(context.Background(), tc.query, "movie", 20)
		want := e.Search(context.Background(), tc.plain, "movie", 20)
		if len(want) == 0 || !slices.EqualFunc(got, want, func(a, b catalog.Title) bool {
			return a.ID == b.ID && a.Name == b.Name && a.Year == b.Year && a.Type == b.Type
		}) {
			t.Errorf("%q: got %+v, plain spelling gives %+v", tc.query, got, want)
		}
	}
}
