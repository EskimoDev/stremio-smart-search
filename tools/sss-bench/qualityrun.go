package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/quality"
	"github.com/EskimoDev/stremio-smart-search/tools/internal/benchdata"
	"github.com/EskimoDev/stremio-smart-search/tools/sss-bench/probes/probekit"
)

const (
	generatedSamples = 3000 // titles sampled, as in tools/benchmark -phase accuracy
	generatedSeed    = 1
	randomDraws      = 6000 // random strings drawn; those that look like words are kept
	randomSeed       = 42
	pathEngine       = "engine"
	pathHTTP         = "http"
)

// answers serves quality.Searcher from results already fetched, so scoring is the
// same code whichever path produced them.
type answers map[probekit.Query][]probekit.Hit

func (a answers) Search(_ context.Context, query, kind string, _ int) []catalog.Title {
	hits := a[probekit.Query{Text: query, Kind: kind}]
	titles := make([]catalog.Title, len(hits))
	for i, hit := range hits {
		titles[i] = catalog.Title{ID: hit.ID, Name: hit.Name}
	}
	return titles
}

// fetchHTTP asks the running application, for targets with no engine probe. The
// result depends on the server's time budget, so the suite is labelled http and
// compare will not treat it as equivalent to an engine-path suite.
func fetchHTTP(a *instance, queries []probekit.Query) (answers, error) {
	found := answers{}
	s := a.searcher()
	for _, q := range queries {
		_, result, err := s.timed(a.searchPath(q.Kind, q.Text))
		if err != nil {
			return nil, err
		}
		if result == shed {
			return nil, fmt.Errorf("search %q was shed while nothing else was searching", q.Text)
		}
		var body struct {
			Metas []probekit.Hit `json:"metas"`
		}
		if err := json.Unmarshal(s.body, &body); err != nil {
			return nil, err
		}
		found[q] = body.Metas
	}
	return found, nil
}

// suiteQueries lists every query a suite will ask.
func suiteQueries(suites []suiteFile) []probekit.Query {
	seen := map[probekit.Query]bool{}
	var queries []probekit.Query
	for _, suite := range suites {
		for _, c := range suite.cases {
			q := probekit.Query{Text: c.Query, Kind: c.Kind}
			if !seen[q] {
				seen[q] = true
				queries = append(queries, q)
			}
		}
	}
	return queries
}

// scoreSuite resolves a curated suite against the catalogue and scores it.
func scoreSuite(c *catalog.Catalog, suite suiteFile, source quality.Searcher, path string) SuiteResult {
	prepared := quality.Verify(c, suite.cases)
	results := quality.Run(context.Background(), source, prepared)
	return summarizeSuite(suite.name, suite.sha256, path, prepared, results)
}

func addStatus(counts *CaseCounts, r quality.Result) {
	counts.Total++
	switch r.Status {
	case quality.Scored:
		if len(r.Want) > 0 {
			counts.Ranked++
		} else {
			counts.NoAnswer++
		}
	case quality.Uncovered:
		counts.Absent++
	case quality.OtherLang:
		counts.OtherLanguage++
	case quality.NoAlias:
		counts.AliasGap++
	case quality.TruthError:
		counts.TruthErrors++
	}
}

// tally accumulates one row of the quality table.
type tally struct {
	counts                             CaseCounts
	top1, top5, retrieved, fpCases, fp int
	reciprocal                         float64
}

func (t *tally) add(r quality.Result, canHaveFalsePositive bool) {
	addStatus(&t.counts, r)
	if r.Status != quality.Scored {
		return
	}
	if canHaveFalsePositive {
		t.fp++
		if len(r.FalsePositives) > 0 {
			t.fpCases++
		}
	}
	if len(r.Want) == 0 {
		return
	}
	if r.Rank == 1 {
		t.top1++
	}
	if r.Rank >= 1 && r.Rank <= quality.K {
		t.top5++
	}
	if r.Rank > 0 {
		t.retrieved++
		t.reciprocal += 1 / float64(r.Rank)
	}
}

func percent(part, whole int) *float64 {
	if whole == 0 {
		return nil
	}
	value := 100 * float64(part) / float64(whole)
	return &value
}

func (t *tally) row(category string) CategoryResult {
	row := CategoryResult{Category: category, Counts: t.counts, Top1: t.top1, Top5: t.top5, Retrieved: t.retrieved, FalsePositiveCases: t.fpCases, FalsePositiveChecks: t.fp}
	row.Top1Pct, row.Top5Pct, row.RetrievedPct = percent(t.top1, t.counts.Ranked), percent(t.top5, t.counts.Ranked), percent(t.retrieved, t.counts.Ranked)
	row.CoveragePct = percent(t.counts.Ranked, t.counts.Ranked+t.counts.Absent)
	if t.counts.Ranked > 0 {
		mrr := t.reciprocal / float64(t.counts.Ranked)
		row.MRR = &mrr
	}
	return row
}

// summarizeSuite totals results by category with an "all" row first. Every case
// lands in exactly one of the counts, so the denominators can be audited.
func summarizeSuite(name, sha, path string, prepared []quality.Prepared, results []quality.Result) SuiteResult {
	rows := map[string]*tally{"all": {}}
	suite := SuiteResult{Name: name, SHA256: sha, Path: path}
	for i, r := range results {
		canHaveFalsePositive := len(r.Want) == 0 || len(prepared[i].Forbid) > 0
		for _, key := range []string{"all", r.Category} {
			if rows[key] == nil {
				rows[key] = &tally{}
			}
			rows[key].add(r, canHaveFalsePositive)
		}
		suite.Cases = append(suite.Cases, CaseResult{Name: r.Name, Query: r.Query, Kind: r.Kind, Category: r.Category, Status: r.Status, Rank: r.Rank, FalsePositives: r.FalsePositives})
	}
	suite.Totals = rows["all"].counts
	for _, key := range append([]string{"all"}, quality.Categories...) {
		if t := rows[key]; t != nil {
			suite.Categories = append(suite.Categories, t.row(key))
		}
	}
	return suite
}

// looksLikeWord matches the engine's own gate for sound-alike matching: it has a
// vowel and no run of more than three consonants.
func looksLikeWord(word string) bool {
	run, vowel := 0, false
	for _, ch := range word {
		if strings.ContainsRune("aeiouy", ch) {
			run, vowel = 0, true
			continue
		}
		if run++; run > 3 {
			return false
		}
	}
	return vowel
}

// randomQueries draws random strings that pass that gate. They have no right
// answer, so the share that return anything measures over-matching.
func randomQueries() []probekit.Query {
	r := rand.New(rand.NewPCG(randomSeed, 11))
	var queries []probekit.Query
	seen := map[string]bool{}
	for i := 0; i < randomDraws; i++ {
		word := make([]byte, 5+r.IntN(6))
		for j := range word {
			word[j] = byte('a' + r.IntN(26))
		}
		if text := string(word); looksLikeWord(text) && !seen[text] {
			seen[text] = true
			queries = append(queries, probekit.Query{Text: text, Kind: "movie"})
		}
	}
	return queries
}

// generatedQueries derives the catalogue-specific suite. It complements the
// curated cases: thousands of mechanical forms, none judged by a person.
func generatedQueries(c *catalog.Catalog) ([]benchdata.Query, []probekit.Query) {
	generated := benchdata.GenerateQueries(c.Titles, generatedSamples, generatedSeed)
	queries := make([]probekit.Query, 0, len(generated)+randomDraws/2)
	seen := map[probekit.Query]bool{}
	add := func(q probekit.Query) {
		if !seen[q] {
			seen[q] = true
			queries = append(queries, q)
		}
	}
	for _, q := range generated {
		add(probekit.Query{Text: q.Text, Kind: q.Type})
	}
	for _, q := range randomQueries() {
		add(q)
	}
	return generated, queries
}

// scoreGenerated tallies the generated forms. Only aggregates are kept: per-query
// records would dwarf the report, and the queries change with the catalogue.
func scoreGenerated(generated []benchdata.Query, found answers, path string) SuiteResult {
	rows := map[string]*tally{}
	var order []string
	total := &tally{}
	for _, q := range generated {
		rank := 0
		for i, hit := range found[probekit.Query{Text: q.Text, Kind: q.Type}] {
			if hit.ID == q.Want {
				rank = i + 1
				break
			}
		}
		if rows[q.Kind] == nil {
			rows[q.Kind] = &tally{}
			order = append(order, q.Kind)
		}
		r := quality.Result{Status: quality.Scored, Want: []string{q.Want}, Rank: rank}
		rows[q.Kind].add(r, false)
		total.add(r, false)
	}
	nonsense := &tally{}
	for _, q := range randomQueries() {
		r := quality.Result{Status: quality.Scored}
		if len(found[q]) > 0 {
			r.FalsePositives = []string{found[q][0].ID}
		}
		nonsense.add(r, true)
		total.add(r, true)
	}
	suite := SuiteResult{Name: "generated", Path: path, SHA256: fmt.Sprintf("samples=%d seed=%d random=%d/%d", generatedSamples, generatedSeed, randomDraws, randomSeed), Totals: total.counts}
	suite.Categories = append(suite.Categories, total.row("all"))
	slices.Sort(order)
	for _, kind := range order {
		suite.Categories = append(suite.Categories, rows[kind].row(kind))
	}
	suite.Categories = append(suite.Categories, nonsense.row("random strings"))
	return suite
}

// httpMismatches counts queries whose result lists differ between the engine and
// the running server. Any difference means the server layer changed what a user
// sees, or a search hit its time budget.
func httpMismatches(engine, server answers) ([]string, error) {
	var different []string
	for q, want := range engine {
		got, ok := server[q]
		if !ok {
			return nil, fmt.Errorf("no HTTP answer for %q", q.Text)
		}
		same := len(got) == len(want)
		for i := 0; same && i < len(got); i++ {
			same = got[i].ID == want[i].ID
		}
		if !same {
			different = append(different, q.Text)
		}
	}
	slices.Sort(different)
	return different, nil
}
