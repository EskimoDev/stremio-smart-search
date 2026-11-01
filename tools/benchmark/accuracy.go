package main

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/search"
	"github.com/EskimoDev/stremio-smart-search/tools/internal/benchdata"
)

// accuracyKind summarises one query form, e.g. a single typo in one word.
type accuracyKind struct {
	Kind    string  `json:"kind"`
	Queries int     `json:"queries"`
	Top1    float64 `json:"top1"`
	Top5    float64 `json:"top5"`
	Top20   float64 `json:"top20"`
	MeanMS  float64 `json:"meanMS"`
}

type accuracyReport struct {
	Phase   string         `json:"phase"`
	Profile string         `json:"profile"`
	Titles  int            `json:"titles"`
	Aliases int            `json:"aliases,omitempty"`
	Seed    uint64         `json:"seed"`
	Kinds   []accuracyKind `json:"kinds"`
	Probes  []probe        `json:"probes"`
}

// probe records the first results of a fixed query, for cases judged by eye,
// such as an acronym that also fuzzily matches an unrelated word ("hotd" and
// "Hota Hai" on the movie row).
type probe struct {
	Query string   `json:"query"`
	Kind  string   `json:"kind"`
	First []string `json:"first"`
}

// runAccuracy measures how often a deliberately mistyped or partial title
// still finds the intended entry, directly against the search engine (no
// HTTP). Queries are derived deterministically from the catalogue itself, so
// results are comparable between builds and data sets. When dump is set, every
// query and its result IDs are written there to check that an optimisation
// leaves results unchanged.
func runAccuracy(path, profile string, samples int, seed uint64, dump string) {
	c, err := catalog.Load(path)
	must(err)
	r := accuracyReport{Phase: "accuracy", Profile: profile, Titles: len(c.Titles), Aliases: c.AliasCount(), Seed: seed}
	queries := benchdata.GenerateQueries(c.Titles, samples, seed)
	engine := search.New(c.Titles)
	c = nil
	type tally struct {
		n, top1, top5, top20 int
		elapsed              time.Duration
	}
	tallies := map[string]*tally{}
	var order []string
	var out *json.Encoder
	if dump != "" {
		f, err := os.Create(dump)
		must(err)
		defer f.Close()
		out = json.NewEncoder(f)
	}
	for _, q := range queries {
		started := time.Now()
		results := engine.Search(context.Background(), q.Text, q.Type, 20)
		elapsed := time.Since(started)
		ids := make([]string, len(results))
		for i, t := range results {
			ids[i] = t.ID
		}
		if out != nil {
			must(out.Encode(map[string]any{"q": q.Text, "type": q.Type, "ids": ids}))
		}
		t := tallies[q.Kind]
		if t == nil {
			t = &tally{}
			tallies[q.Kind] = t
			order = append(order, q.Kind)
		}
		t.n++
		t.elapsed += elapsed
		if rank := slices.Index(ids, q.Want); rank >= 0 {
			t.top20++
			if rank < 5 {
				t.top5++
			}
			if rank == 0 {
				t.top1++
			}
		}
	}
	for _, kind := range order {
		t := tallies[kind]
		r.Kinds = append(r.Kinds, accuracyKind{kind, t.n, ratio(t.top1, t.n), ratio(t.top5, t.n), ratio(t.top20, t.n), float64(t.elapsed.Microseconds()) / 1000 / float64(t.n)})
	}
	for _, p := range []probe{{Query: "hotd", Kind: "movie"}, {Query: "hotd", Kind: "series"}, {Query: "got", Kind: "series"}, {Query: "lotr", Kind: "movie"}} {
		for _, t := range engine.Search(context.Background(), p.Query, p.Kind, 10) {
			p.First = append(p.First, t.Name)
		}
		r.Probes = append(r.Probes, p)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	must(encoder.Encode(r))
}

func ratio(a, b int) float64 { return float64(int(float64(a)/float64(b)*10000)) / 10000 }
