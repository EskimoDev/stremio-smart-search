package quality

import (
	"context"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

type fixed map[string][]catalog.Title

func (f fixed) Search(_ context.Context, query, _ string, _ int) []catalog.Title { return f[query] }

func TestCaseFileLoadsAndMatchesFixture(t *testing.T) {
	cases, err := LoadCases("../../testdata/quality/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalog.Load("../../testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range Verify(c, cases) {
		if p.Status == TruthError {
			t.Errorf("%s: %s", p.Name, p.Detail)
		}
	}
}

func TestMetrics(t *testing.T) {
	cat := &catalog.Catalog{Titles: []catalog.Title{
		{ID: "tt1", Name: "Alpha", Type: "movie", Year: 2000},
		{ID: "tt2", Name: "Beta", Type: "movie", Year: 2001},
		{ID: "tt3", Name: "Kuch Kuch Hota Hai", Type: "movie"},
	}}
	cases := []Case{
		{Name: "a", Query: "alpha", Kind: "movie", Category: "exact", Expect: []Want{{ID: "tt1", Title: "Alpha", Year: 2000}}},
		{Name: "b", Query: "beta", Kind: "movie", Category: "fuzzy", Expect: []Want{{ID: "tt2", Title: "Beta"}}, Forbid: []string{"hota hai"}},
		{Name: "c", Query: "gamma", Kind: "movie", Category: "fuzzy", Expect: []Want{{ID: "tt9", Title: "Gamma"}}},
		{Name: "d", Query: "zzz", Kind: "movie", Category: "negative"},
		{Name: "e", Query: "x", Kind: "movie", Category: "exact", Expect: []Want{{ID: "tt1", Title: "Wrong"}}},
	}
	engine := fixed{
		"alpha": {cat.Titles[0]},
		"beta":  {cat.Titles[2], cat.Titles[1]},
		"zzz":   {cat.Titles[0]},
	}
	results := Run(context.Background(), engine, Verify(cat, cases))
	want := map[string]string{"a": Scored, "b": Scored, "c": Uncovered, "d": Scored, "e": TruthError}
	for _, r := range results {
		if r.Status != want[r.Name] {
			t.Fatalf("%s: status %s", r.Name, r.Status)
		}
	}
	all := Summarize(results)[0]
	if all.Cases != 3 || all.Ranked != 2 || all.Top1 != 1 || all.RecallK != 2 || all.MRR != .75 || all.FPCases != 2 {
		t.Fatalf("summary %+v", all)
	}
	before := results
	after := Run(context.Background(), fixed{"alpha": {cat.Titles[1], cat.Titles[0]}, "beta": {cat.Titles[1]}}, Verify(cat, cases))
	regressions, improvements := Compare(before, after)
	if len(regressions) != 1 || regressions[0].Name != "a" || len(improvements) != 2 {
		t.Fatalf("regressions %+v improvements %+v", regressions, improvements)
	}
}
