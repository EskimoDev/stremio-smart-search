package search

import (
	"context"
	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"os"
	"strings"
	"testing"
)

func fixture(t testing.TB) *Engine {
	t.Helper()
	c, err := catalog.Load("../../testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	return New(c.Titles)
}
func TestSearchUserCases(t *testing.T) {
	e := fixture(t)
	for _, tc := range []struct{ query, kind, want string }{
		{"hotd", "series", "tt11198330"}, {"got", "series", "tt0944947"}, {"LOTR", "movie", "tt0120737"}, {"tdk", "movie", "tt0468569"},
		{"house of dragom", "series", "tt11198330"}, {"house of the draogn", "series", "tt11198330"}, {"interstelar", "movie", "tt0816692"},
		{"strager things", "series", "tt4574334"}, {"spiderman", "movie", "tt0145487"}, {"amelie", "movie", "tt0211915"},
		{"Up", "movie", "tt1049413"}, {"It", "movie", "tt1396484"}, {"It", "series", "tt0099864"}, {"the matrix", "movie", "tt0133093"},
	} {
		t.Run(tc.query+"/"+tc.kind, func(t *testing.T) {
			result := e.Search(context.Background(), tc.query, tc.kind, 20)
			if len(result) == 0 || result[0].ID != tc.want {
				t.Fatalf("wanted %s first, got %+v", tc.want, result)
			}
			for _, r := range result {
				if r.Type != tc.kind {
					t.Fatal("wrong media type")
				}
			}
		})
	}
}
func TestAmbiguousAliasPreservesAlternatives(t *testing.T) {
	r := fixture(t).Search(context.Background(), "hotd", "series", 20)
	if len(r) < 2 || r[0].ID != "tt11198330" || r[1].ID != "tt1685401" {
		t.Fatalf("lost valid alias interpretations: %+v", r)
	}
}
func TestBoundsAndUnknownQueries(t *testing.T) {
	e := fixture(t)
	for _, q := range []string{"", "the", "qzxqvvnonexistent", strings.Repeat("a", 121), "one two three four five six seven eight nine"} {
		if got := e.Search(context.Background(), q, "movie", 20); len(got) != 0 {
			t.Fatalf("unexpected results for %q: %+v", q, got)
		}
	}
	if len(e.Search(context.Background(), "it", "movie", 0)) != 0 {
		t.Fatal("limit ignored")
	}
}
func BenchmarkSearch(b *testing.B) {
	e := fixture(b)
	b.ReportAllocs()
	for b.Loop() {
		e.Search(context.Background(), "house of dragom", "series", 20)
	}
}

// aliasEngine combines the canonical fixture with the handwritten IMDb-shaped
// alias fixture for one language, as a real build would.
func aliasEngine(t testing.TB, options catalog.AliasOptions) *Engine {
	t.Helper()
	base, err := catalog.Load("../../testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	open := func(name string) *os.File {
		f, err := os.Open("../../testdata/aliases/" + name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}
	profile, _ := catalog.ProfileByID("standard")
	extra, err := catalog.ParseIMDbProfile(context.Background(), open("title.ratings.tsv"), open("title.basics.tsv"), profile)
	if err != nil {
		t.Fatal(err)
	}
	index := map[string]int{}
	for i, title := range base.Titles {
		index[title.ID] = i
	}
	for _, title := range extra.Titles {
		if _, ok := index[title.ID]; !ok {
			index[title.ID] = len(base.Titles)
			base.Titles = append(base.Titles, title)
		}
	}
	if err := catalog.ParseIMDbAliases(context.Background(), open("title.akas.tsv"), base, options); err != nil {
		t.Fatal(err)
	}
	return New(base.Titles)
}

func ids(titles []catalog.Title) []string {
	out := make([]string, 0, len(titles))
	for _, title := range titles {
		out = append(out, title.ID)
	}
	return out
}

func TestAlternativeTitleSearch(t *testing.T) {
	french := aliasEngine(t, catalog.AliasOptions{Language: "fr"})
	english := aliasEngine(t, catalog.AliasOptions{Language: "en"})
	japanese := aliasEngine(t, catalog.AliasOptions{Language: "ja"})
	korean := aliasEngine(t, catalog.AliasOptions{Language: "ko"})
	for _, tc := range []struct {
		engine            *Engine
		query, kind, want string
	}{
		{french, "Frozen", "movie", "tt2294629"},
		{french, "La Reine des neiges", "movie", "tt2294629"},
		{french, "la reine des neiges", "movie", "tt2294629"},
		{french, "La rene des neige", "movie", "tt2294629"}, // misspelled French
		{french, "lareinedesneiges", "movie", "tt2294629"},
		{french, "lahaut", "movie", "tt1049413"},
		{french, "Zootopie", "movie", "tt2948356"},
		{french, "zootopi", "movie", "tt2948356"},
		{french, "La-haut", "movie", "tt1049413"},
		{french, "là haut", "movie", "tt1049413"},
		{french, "Le Trône de fer", "series", "tt0944947"},
		{french, "le trone de fer", "series", "tt0944947"},
		{french, "le tronne de fer", "series", "tt0944947"},
		{french, "voyage de chihiro", "movie", "tt0245429"},
		{english, "Zootropolis", "movie", "tt2948356"},
		{english, "zootropolsi", "movie", "tt2948356"},
		{english, "Zootopia", "movie", "tt2948356"},
		{japanese, "千と千尋の神隠し", "movie", "tt0245429"},
		{japanese, "千と千尋", "movie", "tt0245429"},
		{japanese, "spirited away", "movie", "tt0245429"},
		{japanese, "sen to chihiro", "movie", "tt0245429"}, // original title, any locale
		{korean, "기생충", "movie", "tt6751668"},
		{korean, "gisaengchung", "movie", "tt6751668"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			result := tc.engine.Search(context.Background(), tc.query, tc.kind, 20)
			if len(result) == 0 || result[0].ID != tc.want {
				t.Fatalf("wanted %s first, got %v", tc.want, ids(result))
			}
		})
	}
}

func TestAlternativeTitlesAreSearchKeysOnly(t *testing.T) {
	e := aliasEngine(t, catalog.AliasOptions{Language: "fr"})
	result := e.Search(context.Background(), "La Reine des neiges", "movie", 20)
	// Two different films share the alias: both listed, popular first, each once.
	if got := ids(result); len(got) != 2 || got[0] != "tt2294629" || got[1] != "tt9999902" {
		t.Fatalf("shared alias: %v", got)
	}
	if result[0].Name != "Frozen" || result[0].Aliases != nil {
		t.Fatalf("results must keep canonical names and drop aliases: %+v", result[0])
	}
	seen := map[string]bool{}
	for _, title := range e.Search(context.Background(), "reine", "movie", 20) {
		if seen[title.ID] {
			t.Fatal("duplicate listing")
		}
		seen[title.ID] = true
	}
	if got := e.Search(context.Background(), "la reine des neiges", "series", 20); len(got) != 0 {
		t.Fatal("alias crossed media type")
	}
	if got := e.Search(context.Background(), "lrdn", "movie", 20); len(got) != 0 {
		t.Fatalf("acronym generated from an alias: %v", ids(got))
	}
	if got := e.Search(context.Background(), "lo", "movie", 20); len(got) != 0 {
		t.Fatalf("short alias indexed: %v", ids(got))
	}
	if e.AlternateTitles() == 0 {
		t.Fatal("alias count missing")
	}
}

func TestCanonicalExactBeatsAlternativeExact(t *testing.T) {
	c, err := catalog.Load("../../testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	for i := range c.Titles {
		if c.Titles[i].ID == "tt0816692" { // Interstellar, more votes than Alien
			c.Titles[i].Aliases = []string{"Alien", "The Matrix Reloaded"}
		}
	}
	e := New(c.Titles)
	if got := ids(e.Search(context.Background(), "alien", "movie", 20)); len(got) < 2 || got[0] != "tt0078748" || got[1] != "tt0816692" {
		t.Fatalf("canonical exact should win over a more popular alias: %v", got)
	}
	// A short alias that spells another title's acronym ranks with acronyms,
	// so the more popular title keeps first place (IMDb lists "H.O.T.D." for
	// Highschool of the Dead in English).
	for i := range c.Titles {
		if c.Titles[i].ID == "tt1685401" {
			c.Titles[i].Aliases = []string{"H.O.T.D.", "HOTD"}
		}
	}
	e = New(c.Titles)
	if got := ids(e.Search(context.Background(), "HOTD", "series", 20)); len(got) < 2 || got[0] != "tt11198330" || got[1] != "tt1685401" {
		t.Fatalf("short alias outranked a popular acronym: %v", got)
	}
	// Token matches on canonical words beat equally popular alias words.
	if got := ids(e.Search(context.Background(), "matrix", "movie", 20)); len(got) < 2 || got[0] != "tt0133093" {
		t.Fatalf("canonical words should win: %v", got)
	}
}

func TestAliasesDoNotChangeCanonicalResults(t *testing.T) {
	plain := fixture(t)
	withAliases := aliasEngine(t, catalog.AliasOptions{Language: "fr"})
	for _, tc := range []struct{ query, kind string }{
		{"hotd", "series"}, {"got", "series"}, {"LOTR", "movie"}, {"tdk", "movie"}, {"house of dragom", "series"}, {"interstelar", "movie"},
		{"strager things", "series"}, {"spiderman", "movie"}, {"amelie", "movie"}, {"Up", "movie"}, {"It", "movie"}, {"the matrix", "movie"},
	} {
		want := plain.Search(context.Background(), tc.query, tc.kind, 20)
		got := withAliases.Search(context.Background(), tc.query, tc.kind, 20)
		if len(want) == 0 || len(got) == 0 || want[0].ID != got[0].ID {
			t.Fatalf("%s: top result changed from %v to %v", tc.query, ids(want), ids(got))
		}
	}
}

func BenchmarkAliasSearch(b *testing.B) {
	e := aliasEngine(b, catalog.AliasOptions{Language: "fr"})
	b.ReportAllocs()
	for b.Loop() {
		e.Search(context.Background(), "la rene des neige", "movie", 20)
	}
}

// Short words have too few trigrams for a central typo to share one; the
// single-edit lookup covers them for canonical titles as well as aliases.
func TestShortWordCentralTypos(t *testing.T) {
	e := fixture(t)
	for _, tc := range []struct{ query, kind, want string }{
		{"alein", "movie", "tt0078748"}, {"aleen", "movie", "tt0078748"}, {"matirx", "movie", "tt0133093"}, {"hosue of the dragon", "series", "tt11198330"},
	} {
		if got := e.Search(context.Background(), tc.query, tc.kind, 20); len(got) == 0 || got[0].ID != tc.want {
			t.Fatalf("%s: wanted %s first, got %v", tc.query, tc.want, ids(got))
		}
	}
}
