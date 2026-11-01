package search

import (
	"context"
	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/textnorm"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

type Engine struct {
	titles     []catalog.Title
	exact      map[string][]int
	compact    map[string][]int
	aliases    map[string][]int // acronyms of canonical names
	words      map[string][]int
	grams      map[string][]string
	vocabulary []string
	// Alternative (regional/translated) titles. Their exact forms and words are
	// kept apart from canonical ones so they can rank slightly lower, but they
	// share the vocabulary and trigram index, and therefore all typo handling.
	alternate      keyIndex
	alternateWords map[string][]int32
	alternateCount int
}

// Score bands. Canonical exact names stay first; an exact alternative title
// ranks above acronym and word matches. Alternative-title word matches are
// discounted slightly so canonical words win ties.
const (
	scoreExact            = 2200
	scoreCompact          = 2100
	scoreAlternate        = 2000
	scoreAlternateCompact = 1900
	scoreAcronym          = 1200
	alternateWordWeight   = .9
)

var stop = map[string]bool{"a": true, "an": true, "the": true, "of": true, "and": true}

// Normalize is shared with the catalogue builder so alias deduplication and
// the index agree exactly.
func Normalize(text string) string { return textnorm.Normalize(text) }

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
	var exactPairs, wordPairs []posting
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
		// Alternative titles: exact, joined and word forms only. No acronyms,
		// which would multiply ambiguous short keys across languages.
		for _, name := range t.Aliases {
			normal := Normalize(name)
			if seenNames[normal] || !textnorm.Meaningful(normal) {
				continue
			}
			seenNames[normal] = true
			e.alternateCount++
			// Short Latin aliases such as "H.O.T.D." or "Oben" behave like
			// acronyms: several titles legitimately share them, so they rank in
			// the acronym band where popularity decides, not above it.
			if compact := strings.ReplaceAll(normal, " ", ""); len(compact) <= 5 && isASCII(compact) {
				if !seenAliases[compact] {
					e.aliases[compact] = append(e.aliases[compact], id)
					seenAliases[compact] = true
				}
			} else {
				exactPairs = append(exactPairs, posting{normal, int32(id)})
			}
			for _, word := range strings.Fields(normal) {
				if !seenWords[word] {
					wordPairs = append(wordPairs, posting{word, int32(id)})
					seenWords[word] = true
				}
			}
		}
		// The engine never returns aliases; drop them so the snapshot's alias
		// strings can be collected once indexed.
		e.titles[id].Aliases = nil
	}
	for word := range e.words {
		e.vocabulary = append(e.vocabulary, word)
	}
	e.alternate = newKeyIndex(exactPairs)
	e.alternateWords = groupPostings(wordPairs)
	for word := range e.alternateWords {
		if _, ok := e.words[word]; !ok {
			e.vocabulary = append(e.vocabulary, word)
		}
	}
	// Build trigram lists in sorted order so bounded candidate scans are
	// deterministic between runs.
	sort.Strings(e.vocabulary)
	for _, word := range e.vocabulary {
		if n := utf8.RuneCountInString(word); n >= 3 && n <= 40 {
			for _, g := range uniqueGrams(word) {
				e.grams[g] = append(e.grams[g], word)
			}
		}
	}
	return e
}

// posting pairs a key with an index into Engine.titles.
type posting struct {
	key string
	id  int32
}

func sortPostings(pairs []posting) {
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key != pairs[j].key {
			return pairs[i].key < pairs[j].key
		}
		return pairs[i].id < pairs[j].id
	})
}

// keyIndex is a sorted, allocation-light exact-key lookup used for alternative
// titles: most aliases map to one title, so a map of slices would spend more
// on headers and buckets than on the data itself. joined orders the multi-word
// keys as if their spaces were removed, so "lareinedesneiges" finds
// "la reine des neiges" without storing a second copy of each key.
type keyIndex struct {
	keys   []string
	ids    []int32
	joined []int32
}

func newKeyIndex(pairs []posting) keyIndex {
	sortPostings(pairs)
	k := keyIndex{keys: make([]string, len(pairs)), ids: make([]int32, len(pairs))}
	for i, p := range pairs {
		k.keys[i], k.ids[i] = p.key, p.id
		if strings.IndexByte(p.key, ' ') >= 0 {
			k.joined = append(k.joined, int32(i))
		}
	}
	k.joined = append([]int32(nil), k.joined...) // drop append slack
	sort.SliceStable(k.joined, func(i, j int) bool {
		return compareJoined(k.keys[k.joined[i]], k.keys[k.joined[j]]) < 0
	})
	return k
}

// compareJoined compares two keys byte-wise, ignoring spaces.
func compareJoined(a, b string) int {
	i, j := 0, 0
	for {
		for i < len(a) && a[i] == ' ' {
			i++
		}
		for j < len(b) && b[j] == ' ' {
			j++
		}
		switch {
		case i == len(a) && j == len(b):
			return 0
		case i == len(a):
			return -1
		case j == len(b):
			return 1
		case a[i] != b[j]:
			if a[i] < b[j] {
				return -1
			}
			return 1
		}
		i++
		j++
	}
}

func (k keyIndex) lookup(key string) []int32 {
	start := sort.SearchStrings(k.keys, key)
	end := start
	for end < len(k.keys) && k.keys[end] == key {
		end++
	}
	return k.ids[start:end]
}

// lookupJoined returns titles whose multi-word key equals joined once spaces
// are removed. The result is freshly allocated (usually empty).
func (k keyIndex) lookupJoined(joined string) []int32 {
	start := sort.Search(len(k.joined), func(i int) bool { return compareJoined(k.keys[k.joined[i]], joined) >= 0 })
	var ids []int32
	for i := start; i < len(k.joined) && compareJoined(k.keys[k.joined[i]], joined) == 0; i++ {
		ids = append(ids, k.ids[k.joined[i]])
	}
	return ids
}

// groupPostings builds word postings that share one exactly sized backing
// array, ordered by title index (popularity) like the canonical postings.
func groupPostings(pairs []posting) map[string][]int32 {
	sortPostings(pairs)
	ids := make([]int32, len(pairs))
	out := map[string][]int32{}
	for start := 0; start < len(pairs); {
		end := start
		for end < len(pairs) && pairs[end].key == pairs[start].key {
			ids[end] = pairs[end].id
			end++
		}
		out[pairs[start].key] = ids[start:end:end]
		start = end
	}
	return out
}

// AlternateTitles reports how many alternative titles the index holds.
func (e *Engine) AlternateTitles() int { return e.alternateCount }

func isASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// forEachEdit visits deletions, adjacent transpositions, and substitutions and
// insertions over ASCII letters (Damerau distance 1). Substitutions and
// insertions at either end are skipped: they leave an end trigram intact, so
// the trigram scan already finds them. The buffer is reused, so visit must
// copy anything it keeps.
func forEachEdit(word string, visit func([]byte)) {
	buf := make([]byte, 0, len(word)+1)
	for i := 0; i <= len(word); i++ {
		head, tail := word[:i], word[i:]
		if len(tail) > 0 {
			visit(append(append(buf[:0], head...), tail[1:]...))
		}
		if len(tail) > 1 && tail[0] != tail[1] {
			visit(append(append(append(buf[:0], head...), tail[1], tail[0]), tail[2:]...))
		}
		if i == 0 || i == len(word) {
			continue
		}
		for c := byte('a'); c <= 'z'; c++ {
			if i < len(word)-1 && tail[0] != c {
				visit(append(append(append(buf[:0], head...), c), tail[1:]...))
			}
			visit(append(append(append(buf[:0], head...), c), tail...))
		}
	}
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
	} else if _, ok := e.alternateWords[query]; ok {
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
	// Short words have only two to four trigrams, so a single central typo
	// ("rene" for "reine") can share none of them. For short ASCII terms, look
	// up every single-edit variant directly: a few hundred allocation-free map
	// probes, plus the alias map only when the catalogue has aliases.
	if length >= 4 && length <= 6 && isASCII(query) {
		forEachEdit(query, func(variant []byte) {
			_, found := e.words[string(variant)]
			if !found && len(e.alternateWords) > 0 {
				_, found = e.alternateWords[string(variant)]
			}
			if found && matches[string(variant)] < .76 {
				matches[string(variant)] = .76
			}
		})
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
	joined := strings.ReplaceAll(q, " ", "")
	seed(e.exact[q], scoreExact)
	seed(e.compact[joined], scoreCompact)
	seedAlternate := func(ids []int32, score float64) {
		for _, id := range ids {
			if e.titles[id].Type == kind {
				scores[int(id)] = math.Max(scores[int(id)], score)
			}
		}
	}
	seedAlternate(e.alternate.lookup(q), scoreAlternate)
	seedAlternate(e.alternate.lookupJoined(joined), scoreAlternateCompact)
	seed(e.aliases[q], scoreAcronym)
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
			for n, id := range e.alternateWords[match.word] {
				if n >= 3000 || ctx.Err() != nil {
					break
				}
				if e.titles[id].Type == kind {
					group[int(id)] = math.Max(group[int(id)], match.weight*alternateWordWeight)
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
