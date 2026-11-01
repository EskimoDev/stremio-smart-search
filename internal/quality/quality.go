// Package quality measures search accuracy against curated queries whose
// expected answers are IMDb identifiers. It is a developer tool: nothing here
// is used by the shipped helper.
//
// Ground truth is never inferred from search results. Every expected title in
// a case carries the IMDb title (and usually year) it is meant to be, and
// Verify checks those against the catalogue being measured, which is built
// from IMDb's own datasets. A case whose identifier does not match its stated
// title is a ground-truth error and stops the run; a case whose title is simply
// not in a smaller catalogue is reported as out of coverage and not scored.
package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/textnorm"
)

// K is the depth used for recall and false-positive checks: roughly what one
// Stremio search row shows without scrolling.
const K = 5

// Depth is how many results are requested per query, so a rank beyond K is
// still known for regression reports.
const Depth = 20

// Want is one acceptable answer. Title is IMDb's primary or original title for
// ID; Year, when set, is its start year.
type Want struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Year  int    `json:"year,omitempty"`
}

// Case is one curated query.
type Case struct {
	Name     string `json:"name"`
	Query    string `json:"query"`
	Kind     string `json:"kind"`
	Category string `json:"category"`
	// Expect lists acceptable answers, the preferred one first. Top-1 counts
	// any of them in first place; recall@K counts any in the top K. Empty
	// means the query has no right answer and any result is a false positive.
	Expect []Want `json:"expect,omitempty"`
	// Language and Alias mark a regional-title case. It is only scored on a
	// catalogue built with that alternative-title language, and Alias (the
	// correctly spelled regional title the query is based on) must be found
	// among the expected title's stored alternative titles.
	Language string `json:"language,omitempty"`
	Alias    string `json:"alias,omitempty"`
	// Forbid lists title fragments that must not appear in the top K: results
	// known to be wrong for this query, such as "Hota Hai" for "hotd".
	Forbid []string `json:"forbid,omitempty"`
	Source string   `json:"source,omitempty"`
}

// Categories, in report order.
var Categories = []string{
	"exact", "original", "fuzzy", "transposition", "acronym", "joined", "punctuation", "accent",
	"short", "similar", "year", "alias", "alias-fuzzy", "non-latin", "phonetic", "negative",
}

// LoadCases reads a case file and checks its shape.
func LoadCases(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Cases []Case `json:"cases"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, c := range file.Cases {
		switch {
		case c.Name == "" || names[c.Name]:
			return nil, fmt.Errorf("case %q: missing or duplicate name", c.Name)
		case c.Kind != "movie" && c.Kind != "series":
			return nil, fmt.Errorf("case %q: kind must be movie or series", c.Name)
		case !slices.Contains(Categories, c.Category):
			return nil, fmt.Errorf("case %q: unknown category %q", c.Name, c.Category)
		case (c.Language == "") != (c.Alias == ""):
			return nil, fmt.Errorf("case %q: language and alias go together", c.Name)
		case len(c.Expect) == 0 && c.Category != "negative" && len(c.Forbid) == 0:
			return nil, fmt.Errorf("case %q: no expected answer", c.Name)
		}
		for _, w := range c.Expect {
			if !strings.HasPrefix(w.ID, "tt") || w.Title == "" {
				return nil, fmt.Errorf("case %q: every answer needs an IMDb id and title", c.Name)
			}
		}
		names[c.Name] = true
	}
	return file.Cases, nil
}

// Status of a case against one catalogue.
const (
	Scored     = "scored"
	Uncovered  = "uncovered"   // no expected title is in this catalogue
	OtherLang  = "language"    // regional case for another alias language
	NoAlias    = "alias-gap"   // the catalogue kept other regional titles for it
	TruthError = "truth-error" // identifier does not match the stated title
)

// Prepared is a case resolved against a catalogue: only answers present in it
// remain, so recall is not penalised for titles a smaller size never holds.
type Prepared struct {
	Case
	Status string
	Detail string
	Wants  []string
}

func norm(s string) string { return textnorm.Normalize(s) }

// containsWords reports whether needle occurs in haystack on word boundaries.
func containsWords(haystack, needle string) bool {
	return needle != "" && strings.Contains(" "+haystack+" ", " "+needle+" ")
}

// Verify resolves every case against the catalogue it will be scored on.
func Verify(c *catalog.Catalog, cases []Case) []Prepared {
	byID := make(map[string]*catalog.Title, len(c.Titles))
	for i := range c.Titles {
		byID[c.Titles[i].ID] = &c.Titles[i]
	}
	out := make([]Prepared, 0, len(cases))
	for _, tc := range cases {
		p := Prepared{Case: tc, Status: Scored}
		if tc.Language != "" && tc.Language != c.AliasLanguage {
			p.Status = OtherLang
			out = append(out, p)
			continue
		}
		for _, w := range tc.Expect {
			t, ok := byID[w.ID]
			if !ok {
				continue
			}
			if t.Type != tc.Kind || (norm(t.Name) != norm(w.Title) && norm(t.Original) != norm(w.Title)) || (w.Year != 0 && t.Year != w.Year) {
				p.Status = TruthError
				p.Detail = fmt.Sprintf("%s is %q (%s, %d), case says %q (%d)", w.ID, t.Name, t.Type, t.Year, w.Title, w.Year)
				break
			}
			p.Wants = append(p.Wants, w.ID)
		}
		if p.Status == Scored && len(tc.Expect) > 0 && len(p.Wants) == 0 {
			p.Status = Uncovered
		}
		if p.Status == Scored && tc.Alias != "" {
			t := byID[tc.Expect[0].ID]
			found := false
			if t != nil {
				for _, a := range t.Aliases {
					if containsWords(norm(a), norm(tc.Alias)) {
						found = true
					}
				}
			}
			if !found {
				p.Status = NoAlias
				if t != nil {
					p.Detail = fmt.Sprintf("stored: %q", t.Aliases)
				}
			}
		}
		out = append(out, p)
	}
	return out
}

// Searcher is the search engine's query method.
type Searcher interface {
	Search(ctx context.Context, query, kind string, limit int) []catalog.Title
}

// Result is one scored case.
type Result struct {
	Name     string   `json:"name"`
	Query    string   `json:"query"`
	Kind     string   `json:"kind"`
	Category string   `json:"category"`
	Status   string   `json:"status"`
	Detail   string   `json:"detail,omitempty"`
	Want     []string `json:"want,omitempty"`
	// Rank is the 1-based position of the best acceptable answer within
	// Depth results, 0 if absent.
	Rank int `json:"rank"`
	// FalsePositives are top-K results that are known to be wrong: any result
	// for a negative case, or a forbidden title.
	FalsePositives []string `json:"falsePositives,omitempty"`
	Top            []string `json:"top,omitempty"` // first K results, "id name"
}

// Summary aggregates scored cases.
type Summary struct {
	Category       string  `json:"category"`
	Cases          int     `json:"cases"`
	Ranked         int     `json:"ranked"` // cases with an expected answer; top-1, recall and MRR are out of these
	Top1           int     `json:"top1"`
	RecallK        int     `json:"recallAt5"`
	MRR            float64 `json:"mrr"`
	Missing        int     `json:"missing"` // expected title absent from the top Depth
	FalsePositives int     `json:"falsePositives"`
	FPCases        int     `json:"fpCases"` // cases with a false positive or a negative case
}

type Report struct {
	Catalogue string    `json:"catalogue"`
	Profile   string    `json:"profile,omitempty"`
	Language  string    `json:"aliasLanguage,omitempty"`
	Titles    int       `json:"titles"`
	Summary   []Summary `json:"summary"`
	Results   []Result  `json:"results"`
}

// Run scores prepared cases. It is deterministic for a given engine.
func Run(ctx context.Context, s Searcher, prepared []Prepared) []Result {
	results := make([]Result, 0, len(prepared))
	for _, p := range prepared {
		r := Result{Name: p.Name, Query: p.Query, Kind: p.Kind, Category: p.Category, Status: p.Status, Detail: p.Detail, Want: p.Wants}
		if p.Status != Scored {
			results = append(results, r)
			continue
		}
		hits := s.Search(ctx, p.Query, p.Kind, Depth)
		for i, h := range hits {
			if r.Rank == 0 && slices.Contains(p.Wants, h.ID) {
				r.Rank = i + 1
			}
			if i >= K {
				continue
			}
			r.Top = append(r.Top, h.ID+" "+h.Name)
			if len(p.Expect) == 0 && len(p.Forbid) == 0 {
				r.FalsePositives = append(r.FalsePositives, h.ID+" "+h.Name)
				continue
			}
			for _, f := range p.Forbid {
				if containsWords(norm(h.Name), norm(f)) {
					r.FalsePositives = append(r.FalsePositives, h.ID+" "+h.Name)
					break
				}
			}
		}
		results = append(results, r)
	}
	return results
}

// Summarize totals results by category, with an "all" row first.
func Summarize(results []Result) []Summary {
	rows := map[string]*Summary{"all": {Category: "all"}}
	for _, r := range results {
		if r.Status != Scored {
			continue
		}
		for _, key := range []string{"all", r.Category} {
			s := rows[key]
			if s == nil {
				s = &Summary{Category: key}
				rows[key] = s
			}
			s.Cases++
			if len(r.FalsePositives) > 0 {
				s.FalsePositives += len(r.FalsePositives)
				s.FPCases++
			}
			if len(r.Want) == 0 {
				continue // negative / forbid-only cases have no rank to score
			}
			s.Ranked++
			switch {
			case r.Rank == 1:
				s.Top1++
			case r.Rank == 0:
				s.Missing++
			}
			if r.Rank >= 1 && r.Rank <= K {
				s.RecallK++
			}
			if r.Rank > 0 {
				s.MRR += 1 / float64(r.Rank)
			}
		}
	}
	out := []Summary{}
	for _, key := range append([]string{"all"}, Categories...) {
		if s := rows[key]; s != nil {
			if s.Ranked > 0 {
				s.MRR /= float64(s.Ranked)
			}
			out = append(out, *s)
		}
	}
	return out
}

// Change is a per-case difference between two reports on the same catalogue.
type Change struct {
	Name   string `json:"name"`
	Query  string `json:"query"`
	Before int    `json:"before"`
	After  int    `json:"after"`
	FPDiff int    `json:"fpDiff"`
}

// Compare lists cases whose rank or false positives changed, regressions
// (worse rank, lost answer or new false positives) first.
func Compare(before, after []Result) (regressions, improvements []Change) {
	old := map[string]Result{}
	for _, r := range before {
		old[r.Name] = r
	}
	worse := func(a, b int) bool { // is rank b worse than rank a?
		if a == 0 {
			return false
		}
		return b == 0 || b > a
	}
	for _, r := range after {
		o, ok := old[r.Name]
		if !ok || o.Status != Scored || r.Status != Scored {
			continue
		}
		c := Change{Name: r.Name, Query: r.Query, Before: o.Rank, After: r.Rank, FPDiff: len(r.FalsePositives) - len(o.FalsePositives)}
		switch {
		case worse(o.Rank, r.Rank) || c.FPDiff > 0:
			regressions = append(regressions, c)
		case worse(r.Rank, o.Rank) || c.FPDiff < 0:
			improvements = append(improvements, c)
		}
	}
	sort.Slice(regressions, func(i, j int) bool { return regressions[i].Name < regressions[j].Name })
	sort.Slice(improvements, func(i, j int) bool { return improvements[i].Name < improvements[j].Name })
	return
}
