package search

import (
	"context"
	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Engine struct {
	titles     []catalog.Title
	exact      map[string][]int
	compact    map[string][]int
	aliases    map[string][]int
	words      map[string][]int
	grams      map[string][]string
	vocabulary []string
}

var stop = map[string]bool{"a": true, "an": true, "the": true, "of": true, "and": true}

func Normalize(text string) string {
	var out strings.Builder
	space := false
	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Mn, r) || r == '\'' || r == '’' {
			continue
		}
		switch {
		case strings.ContainsRune("àáâãäåāăą", r):
			r = 'a'
		case strings.ContainsRune("èéêëēĕėęě", r):
			r = 'e'
		case strings.ContainsRune("ìíîïīĭįı", r):
			r = 'i'
		case strings.ContainsRune("òóôõöøōŏő", r):
			r = 'o'
		case strings.ContainsRune("ùúûüūŭůűų", r):
			r = 'u'
		case strings.ContainsRune("çćĉċč", r):
			r = 'c'
		case strings.ContainsRune("ñńņň", r):
			r = 'n'
		case strings.ContainsRune("ýÿŷ", r):
			r = 'y'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && out.Len() > 0 {
				out.WriteByte(' ')
			}
			space = false
			out.WriteRune(r)
		} else {
			space = true
		}
	}
	return out.String()
}

func uniqueGrams(word string) []string {
	runes := []rune(word)
	n := 3
	if len(runes) < 4 {
		n = 2
	}
	seen := map[string]bool{}
	result := []string{}
	for i := 0; i+n <= len(runes); i++ {
		g := string(runes[i : i+n])
		if !seen[g] {
			seen[g] = true
			result = append(result, g)
		}
	}
	return result
}

func New(titles []catalog.Title) *Engine {
	e := &Engine{titles: append([]catalog.Title(nil), titles...), exact: map[string][]int{}, compact: map[string][]int{}, aliases: map[string][]int{}, words: map[string][]int{}, grams: map[string][]string{}}
	sort.SliceStable(e.titles, func(i, j int) bool {
		if e.titles[i].Votes != e.titles[j].Votes {
			return e.titles[i].Votes > e.titles[j].Votes
		}
		return e.titles[i].ID < e.titles[j].ID
	})
	for id, t := range e.titles {
		names := []string{t.Name}
		if t.Original != "" && t.Original != t.Name {
			names = append(names, t.Original)
		}
		seenNames, seenWords, seenAliases := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, name := range names {
			normal := Normalize(name)
			if normal == "" || seenNames[normal] {
				continue
			}
			seenNames[normal] = true
			e.exact[normal] = append(e.exact[normal], id)
			compact := strings.ReplaceAll(normal, " ", "")
			e.compact[compact] = append(e.compact[compact], id)
			for _, word := range strings.Fields(normal) {
				if !seenWords[word] {
					e.words[word] = append(e.words[word], id)
					seenWords[word] = true
				}
			}
			variants := []string{normal, Normalize(strings.SplitN(name, ":", 2)[0])}
			for _, variant := range variants {
				parts := strings.Fields(variant)
				if len(parts) < 2 {
					continue
				}
				all, withoutArticle, significant := "", "", ""
				for p, part := range parts {
					first := []rune(part)[0]
					if first < 'a' || first > 'z' {
						all = ""
						withoutArticle = ""
						significant = ""
						break
					}
					all += string(first)
					if p > 0 || !stop[part] {
						withoutArticle += string(first)
					}
					if !stop[part] {
						significant += string(first)
					}
				}
				for _, alias := range []string{all, withoutArticle, significant} {
					if len(alias) >= 3 && len(alias) <= 8 && !stop[alias] && !seenAliases[alias] {
						e.aliases[alias] = append(e.aliases[alias], id)
						seenAliases[alias] = true
					}
				}
			}
		}
	}
	for word := range e.words {
		e.vocabulary = append(e.vocabulary, word)
		if n := utf8.RuneCountInString(word); n >= 3 && n <= 40 {
			for _, g := range uniqueGrams(word) {
				e.grams[g] = append(e.grams[g], word)
			}
		}
	}
	sort.Strings(e.vocabulary)
	return e
}

// Damerau-Levenshtein with adjacent transpositions; inputs are bounded title words.
func distance(a, b string, limit int) int {
	x, y := []rune(a), []rune(b)
	if abs(len(x)-len(y)) > limit {
		return limit + 1
	}
	previousPrevious := make([]int, len(y)+1)
	previous := make([]int, len(y)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(x); i++ {
		current := make([]int, len(y)+1)
		current[0] = i
		rowMin := i
		for j := 1; j <= len(y); j++ {
			cost := 0
			if x[i-1] != y[j-1] {
				cost = 1
			}
			current[j] = min(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				current[j] = min(current[j], previousPrevious[j-2]+1)
			}
			rowMin = min(rowMin, current[j])
		}
		if rowMin > limit {
			return limit + 1
		}
		previousPrevious, previous = previous, current
	}
	return previous[len(y)]
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type termMatch struct {
	word   string
	weight float64
}

func (e *Engine) terms(ctx context.Context, query string) []termMatch {
	matches := map[string]float64{}
	if _, ok := e.words[query]; ok {
		matches[query] = 1
	}
	length := utf8.RuneCountInString(query)
	if length >= 3 {
		start := sort.SearchStrings(e.vocabulary, query)
		for i := start; i < len(e.vocabulary) && i < start+20 && strings.HasPrefix(e.vocabulary[i], query); i++ {
			if _, ok := matches[e.vocabulary[i]]; !ok {
				matches[e.vocabulary[i]] = .88
			}
		}
	}
	if length >= 4 && length <= 40 {
		counts := map[string]int{}
		scanned := 0
		for _, gram := range uniqueGrams(query) {
			for _, word := range e.grams[gram] {
				if scanned++; scanned > 20000 {
					break
				}
				if abs(utf8.RuneCountInString(word)-length) <= 2 {
					counts[word]++
				}
			}
			if scanned > 20000 {
				break
			}
		}
		type candidate struct {
			word    string
			overlap int
		}
		candidates := make([]candidate, 0, len(counts))
		for word, count := range counts {
			candidates = append(candidates, candidate{word, count})
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].overlap != candidates[j].overlap {
				return candidates[i].overlap > candidates[j].overlap
			}
			return candidates[i].word < candidates[j].word
		})
		limit := 1
		if length >= 7 {
			limit = 2
		}
		for i, c := range candidates {
			if i >= 96 || ctx.Err() != nil {
				break
			}
			d := distance(query, c.word, limit)
			weight := .76
			if d == 2 {
				weight = .6
			}
			if d <= limit && weight > matches[c.word] {
				matches[c.word] = weight
			}
		}
	}
	result := make([]termMatch, 0, len(matches))
	for word, weight := range matches {
		result = append(result, termMatch{word, weight})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].weight != result[j].weight {
			return result[i].weight > result[j].weight
		}
		return result[i].word < result[j].word
	})
	if len(result) > 32 {
		result = result[:32]
	}
	return result
}

func (e *Engine) Search(ctx context.Context, query, kind string, limit int) []catalog.Title {
	if utf8.RuneCountInString(query) > 120 || (kind != "movie" && kind != "series") {
		return nil
	}
	q := Normalize(query)
	if q == "" {
		return nil
	}
	parts := strings.Fields(q)
	if len(parts) > 8 {
		return nil
	}
	scores := map[int]float64{}
	seed := func(ids []int, score float64) {
		for _, id := range ids {
			if e.titles[id].Type == kind {
				scores[id] = math.Max(scores[id], score)
			}
		}
	}
	seed(e.exact[q], 2200)
	seed(e.compact[strings.ReplaceAll(q, " ", "")], 2100)
	seed(e.aliases[q], 1200)
	unique := map[string]bool{}
	groups := []map[int]float64{}
	for _, part := range parts {
		if stop[part] || unique[part] {
			continue
		}
		unique[part] = true
		if utf8.RuneCountInString(part) < 3 {
			continue
		}
		group := map[int]float64{}
		for _, match := range e.terms(ctx, part) {
			for n, id := range e.words[match.word] {
				if n >= 3000 || ctx.Err() != nil {
					break
				}
				if e.titles[id].Type == kind {
					group[id] = math.Max(group[id], match.weight)
				}
			}
		}
		groups = append(groups, group)
	}
	if len(groups) > 0 && ctx.Err() == nil {
		sort.Slice(groups, func(i, j int) bool { return len(groups[i]) < len(groups[j]) })
		for id, weight := range groups[0] {
			total := weight
			matches := true
			for _, group := range groups[1:] {
				w, ok := group[id]
				if !ok {
					matches = false
					break
				}
				total += w
			}
			if matches {
				scores[id] = math.Max(scores[id], 100*total/float64(len(groups)))
			}
		}
	}
	type hit struct {
		id    int
		score float64
	}
	hits := make([]hit, 0, len(scores))
	for id, score := range scores {
		hits = append(hits, hit{id, score + math.Log1p(float64(max(0, e.titles[id].Votes)))*5})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return e.titles[hits[i].id].ID < e.titles[hits[j].id].ID
	})
	limit = min(max(limit, 0), 50)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	result := make([]catalog.Title, 0, len(hits))
	for _, h := range hits {
		result = append(result, e.titles[h.id])
	}
	return result
}
