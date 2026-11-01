package main

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/search"
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
	type query struct{ kind, text, typ, want string }
	titles := append([]catalog.Title(nil), c.Titles...)
	sort.Slice(titles, func(i, j int) bool {
		if titles[i].Votes != titles[j].Votes {
			return titles[i].Votes > titles[j].Votes
		}
		return titles[i].ID < titles[j].ID
	})
	rng := rand.New(rand.NewPCG(seed, 7))
	var queries []query
	// Sample titles across popularity: half from the top 5%, half from all.
	for i := 0; i < samples; i++ {
		var t catalog.Title
		if i%2 == 0 {
			t = titles[rng.IntN(max(1, len(titles)/20))]
		} else {
			t = titles[rng.IntN(len(titles))]
		}
		normal := search.Normalize(t.Name)
		words := strings.Fields(normal)
		if len(words) == 0 || !asciiWords(words) {
			continue
		}
		add := func(kind, text string) { queries = append(queries, query{kind, text, t.Type, t.ID}) }
		add("exact", normal)
		longest := 0
		for w := range words {
			if len(words[w]) > len(words[longest]) {
				longest = w
			}
		}
		if len(words[longest]) >= 4 {
			add("one typo", strings.Join(replaced(words, longest, typo(rng, words[longest])), " "))
		}
		if len(words[longest]) >= 8 {
			add("two typos", strings.Join(replaced(words, longest, typo(rng, typo(rng, words[longest]))), " "))
		}
		if last := words[len(words)-1]; len(last) >= 6 {
			add("prefix", strings.Join(replaced(words, len(words)-1, last[:4]), " "))
		}
		if len(words) >= 2 {
			add("joined", strings.Join(words, ""))
		}
	}
	for _, t := range titles {
		if len(t.Aliases) > 0 && len(queries) < samples*8 && rng.IntN(8) == 0 {
			normal := search.Normalize(t.Aliases[0])
			queries = append(queries, query{"alternative title", normal, t.Type, t.ID})
			if words := strings.Fields(normal); asciiWords(words) {
				longest := 0
				for w := range words {
					if len(words[w]) > len(words[longest]) {
						longest = w
					}
				}
				if len(words[longest]) >= 4 {
					queries = append(queries, query{"alternative title, one typo", strings.Join(replaced(words, longest, typo(rng, words[longest])), " "), t.Type, t.ID})
				}
			}
		}
	}
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
		results := engine.Search(context.Background(), q.text, q.typ, 20)
		elapsed := time.Since(started)
		ids := make([]string, len(results))
		for i, t := range results {
			ids[i] = t.ID
		}
		if out != nil {
			must(out.Encode(map[string]any{"q": q.text, "type": q.typ, "ids": ids}))
		}
		t := tallies[q.kind]
		if t == nil {
			t = &tally{}
			tallies[q.kind] = t
			order = append(order, q.kind)
		}
		t.n++
		t.elapsed += elapsed
		if rank := slices.Index(ids, q.want); rank >= 0 {
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
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	must(encoder.Encode(r))
}

func ratio(a, b int) float64 { return float64(int(float64(a)/float64(b)*10000)) / 10000 }

func asciiWords(words []string) bool {
	for _, w := range words {
		for i := 0; i < len(w); i++ {
			if w[i] < '0' || w[i] > 'z' || (w[i] > '9' && w[i] < 'a') {
				return false
			}
		}
	}
	return true
}

func replaced(words []string, i int, word string) []string {
	out := slices.Clone(words)
	out[i] = word
	return out
}

// typo applies one random deletion, insertion, substitution or adjacent
// transposition to an ASCII word, never at the first letter.
func typo(rng *rand.Rand, word string) string {
	if len(word) < 3 {
		return word
	}
	b := []byte(word)
	i := 1 + rng.IntN(len(b)-1)
	letter := byte('a' + rng.IntN(26))
	switch rng.IntN(4) {
	case 0:
		return string(append(b[:i:i], b[i+1:]...))
	case 1:
		return string(append(append(b[:i:i], letter), b[i:]...))
	case 2:
		if b[i] == letter {
			letter = 'a' + (letter-'a'+1)%26
		}
		b[i] = letter
		return string(b)
	default:
		if i == len(b)-1 {
			i--
		}
		if i == 0 || b[i] == b[i+1] {
			return string(append(b[:i:i], b[i+1:]...))
		}
		b[i], b[i+1] = b[i+1], b[i]
		return string(b)
	}
}
