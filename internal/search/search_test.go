package search

import (
	"context"
	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
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
