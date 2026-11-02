package main

import (
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/quality"
	"github.com/EskimoDev/stremio-smart-search/tools/internal/benchdata"
	"github.com/EskimoDev/stremio-smart-search/tools/sss-bench/probes/probekit"
)

func qualityResult(rank int) quality.Result {
	return quality.Result{Status: quality.Scored, Want: []string{"tt1"}, Rank: rank}
}

func want(id, title string) []quality.Want { return []quality.Want{{ID: id, Title: title}} }

// scoringFixture has a right answer first, one found low down, one missed, one
// absent from the catalogue, one in another language, one with a wrong identifier,
// a clean negative, a noisy negative and a forbidden result.
func scoringFixture() (*catalog.Catalog, suiteFile, answers) {
	c := &catalog.Catalog{Version: 1, AliasLanguage: "fr", Titles: []catalog.Title{
		{ID: "tt1", Name: "Alpha", Type: "movie"}, {ID: "tt2", Name: "Beta", Type: "movie"},
		{ID: "tt3", Name: "Gamma", Type: "movie"}, {ID: "tt4", Name: "Hot Fuzz", Type: "movie"},
		{ID: "tt5", Name: "Delta", Type: "movie", Aliases: []string{"Delta en francais"}},
	}}
	suite := suiteFile{name: "cases", sha256: "abc", cases: []quality.Case{
		{Name: "first", Query: "alpha", Kind: "movie", Category: "exact", Expect: want("tt1", "Alpha")},
		{Name: "low", Query: "beta", Kind: "movie", Category: "exact", Expect: want("tt2", "Beta")},
		{Name: "lost", Query: "gamma", Kind: "movie", Category: "fuzzy", Expect: want("tt3", "Gamma")},
		{Name: "absent", Query: "omega", Kind: "movie", Category: "fuzzy", Expect: want("tt9", "Omega")},
		{Name: "german", Query: "alpha de", Kind: "movie", Category: "alias", Language: "de", Alias: "Alpha", Expect: want("tt1", "Alpha")},
		{Name: "gap", Query: "delta fr", Kind: "movie", Category: "alias", Language: "fr", Alias: "Nowhere", Expect: want("tt5", "Delta")},
		{Name: "wrong", Query: "beta two", Kind: "movie", Category: "exact", Expect: want("tt1", "Not Alpha")},
		{Name: "quiet", Query: "zzz", Kind: "movie", Category: "negative"},
		{Name: "noisy", Query: "qqq", Kind: "movie", Category: "negative"},
		{Name: "forbid", Query: "hotd", Kind: "movie", Category: "acronym", Expect: want("tt1", "Alpha"), Forbid: []string{"hot"}},
	}}
	hit := func(i int) probekit.Hit { return probekit.Hit{ID: c.Titles[i].ID, Name: c.Titles[i].Name} }
	q := func(text string) probekit.Query { return probekit.Query{Text: text, Kind: "movie"} }
	found := answers{
		q("alpha"): {hit(0)},
		q("beta"):  {hit(2), hit(3), hit(0), hit(4), hit(4), hit(1)}, // rank 6
		q("gamma"): {},
		q("zzz"):   {},
		q("qqq"):   {hit(0)},
		q("hotd"):  {hit(0), hit(3)},
	}
	return c, suite, found
}

func categoryRow(t *testing.T, suite SuiteResult, name string) CategoryResult {
	t.Helper()
	for _, row := range suite.Categories {
		if row.Category == name {
			return row
		}
	}
	t.Fatalf("no category %s", name)
	return CategoryResult{}
}

func TestScoringKeepsEveryCaseInOneDenominator(t *testing.T) {
	c, suite, found := scoringFixture()
	result := scoreSuite(c, suite, found, pathEngine)
	totals := result.Totals
	if got := totals.Ranked + totals.NoAnswer + totals.Absent + totals.OtherLanguage + totals.AliasGap + totals.TruthErrors; got != totals.Total || got != len(suite.cases) {
		t.Fatalf("%d of %d cases accounted for: %+v", got, len(suite.cases), totals)
	}
	if totals != (CaseCounts{Total: 10, Ranked: 4, NoAnswer: 2, Absent: 1, OtherLanguage: 1, AliasGap: 1, TruthErrors: 1}) {
		t.Fatalf("%+v", totals)
	}
}

func TestRetrievalRankingAndAbsenceStayDistinct(t *testing.T) {
	c, suite, found := scoringFixture()
	all := scoreSuite(c, suite, found, pathEngine).Categories[0]
	if all.Category != "all" {
		t.Fatalf("first row is %s", all.Category)
	}
	// Ranked: alpha (rank 1), beta (rank 6), gamma (missed), hotd (rank 1).
	if all.Top1 != 2 || all.Top5 != 2 || all.Retrieved != 3 || all.Counts.Ranked != 4 {
		t.Fatalf("top-1 %d, top-5 %d, retrieved %d of %d", all.Top1, all.Top5, all.Retrieved, all.Counts.Ranked)
	}
	if *all.Top1Pct != 50 || *all.RetrievedPct != 75 {
		t.Fatalf("percentages %v %v", *all.Top1Pct, *all.RetrievedPct)
	}
	// The absent title lowers coverage and leaves accuracy alone.
	if *all.CoveragePct != 80 {
		t.Fatalf("coverage %v", *all.CoveragePct)
	}
}

func TestFalsePositiveDenominatorIsOnlyCasesThatCanHaveOne(t *testing.T) {
	c, suite, found := scoringFixture()
	result := scoreSuite(c, suite, found, pathEngine)
	all := categoryRow(t, result, "all")
	// Checks: quiet, noisy, forbid. Cases with an answer and no forbid list cannot
	// show a false positive, so they must not dilute the share.
	if all.FalsePositiveChecks != 3 || all.FalsePositiveCases != 2 {
		t.Fatalf("%d of %d", all.FalsePositiveCases, all.FalsePositiveChecks)
	}
	if negative := categoryRow(t, result, "negative"); negative.FalsePositiveChecks != 2 || negative.FalsePositiveCases != 1 || negative.Top1Pct != nil {
		t.Fatalf("%+v", negative)
	}
}

func TestEmptyCategoryHasNoPercentage(t *testing.T) {
	if percent(0, 0) != nil {
		t.Fatal("a share of nothing must be absent, not zero")
	}
}

func TestLooksLikeWord(t *testing.T) {
	for word, expected := range map[string]bool{"banana": true, "xqzvwkpl": false, "strength": false, "rhythm": true, "bcd": false} {
		if got := looksLikeWord(word); got != expected {
			t.Errorf("%s: %v", word, got)
		}
	}
}

func TestRandomQueriesAreDeterministicAndDistinct(t *testing.T) {
	first, second := randomQueries(), randomQueries()
	if len(first) == 0 || len(first) != len(second) {
		t.Fatalf("%d and %d", len(first), len(second))
	}
	seen := map[string]bool{}
	for i, q := range first {
		if q != second[i] {
			t.Fatal("not deterministic")
		}
		if seen[q.Text] {
			t.Fatalf("%s repeated", q.Text)
		}
		seen[q.Text] = true
	}
}

func TestScoreGenerated(t *testing.T) {
	generated := []benchdata.Query{
		{Kind: "one typo", Text: "alpa", Type: "movie", Want: "tt1"},
		{Kind: "one typo", Text: "beat", Type: "movie", Want: "tt2"},
		{Kind: "exact", Text: "gamma", Type: "movie", Want: "tt3"},
	}
	found := answers{
		{Text: "alpa", Kind: "movie"}:  {{ID: "tt1", Name: "Alpha"}},
		{Text: "beat", Kind: "movie"}:  {{ID: "tt9", Name: "Other"}, {ID: "tt2", Name: "Beta"}},
		{Text: "gamma", Kind: "movie"}: {},
	}
	randoms := randomQueries()
	found[randoms[0]] = []probekit.Hit{{ID: "tt1", Name: "Alpha"}}
	suite := scoreGenerated(generated, found, pathEngine)
	typo := categoryRow(t, suite, "one typo")
	if typo.Top1 != 1 || typo.Top5 != 2 || typo.Counts.Ranked != 2 {
		t.Fatalf("%+v", typo)
	}
	if exact := categoryRow(t, suite, "exact"); exact.Retrieved != 0 || exact.Counts.Ranked != 1 {
		t.Fatalf("%+v", exact)
	}
	if noise := categoryRow(t, suite, "random strings"); noise.FalsePositiveCases != 1 || noise.FalsePositiveChecks != len(randoms) {
		t.Fatalf("%+v", noise)
	}
}

func TestHTTPMismatchesCompareIdentifiersInOrder(t *testing.T) {
	q := probekit.Query{Text: "a", Kind: "movie"}
	r := probekit.Query{Text: "b", Kind: "movie"}
	engine := answers{q: {{ID: "tt1"}, {ID: "tt2"}}, r: {{ID: "tt3"}}}
	server := answers{q: {{ID: "tt2"}, {ID: "tt1"}}, r: {{ID: "tt3"}}}
	different, err := httpMismatches(engine, server)
	if err != nil || len(different) != 1 || different[0] != "a" {
		t.Fatalf("%v %v", different, err)
	}
	if _, err := httpMismatches(engine, answers{q: nil}); err == nil {
		t.Fatal("a query with no HTTP answer must be an error")
	}
}
