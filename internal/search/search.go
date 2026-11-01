package search

import (
	"cmp"
	"context"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/textnorm"
)

type Engine struct {
	titles []entry
	kinds  []uint8 // kindMovie or kindSeries per title, kept apart for cache-friendly filtering
	// names holds every normalized canonical name (primary and original). It
	// answers exact queries and, through its joined order, queries typed
	// without spaces.
	names    keyIndex
	acronyms shortIndex // acronyms of canonical names, and short alternative titles
	// vocabulary is every indexed word, sorted; a word is referred to by its
	// position. words and alternateWords give each word's titles in popularity
	// order; gramWords gives each trigram's (or short word's bigram's) words.
	vocabulary     []string
	runeLengths    []uint8
	words          postingList
	alternateWords postingList
	grams          []string
	gramWords      postingList
	// Alternative (regional/translated) titles. Their exact forms and words are
	// kept apart from canonical ones so they can rank slightly lower, but they
	// share the vocabulary and trigram index, and therefore all typo handling.
	alternate      keyIndex
	alternateCount int

	scratchMu sync.Mutex
	idle      []*scratch
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

// wordTable assigns provisional numbers to words while the index is built.
type wordTable struct {
	numbers map[string]int32
	words   []string
}

func (w *wordTable) number(word string) int32 {
	if n, ok := w.numbers[word]; ok {
		return n
	}
	n := int32(len(w.words))
	w.numbers[word] = n
	w.words = append(w.words, word)
	return n
}

func New(titles []catalog.Title) *Engine {
	// Most-voted first, then by ID, keeping input order for exact ties. Sorting
	// positions avoids moving whole Title structs around.
	order := make([]int32, len(titles))
	for i := range order {
		order[i] = int32(i)
	}
	slices.SortFunc(order, func(i, j int32) int {
		a, b := &titles[i], &titles[j]
		if a.Votes != b.Votes {
			return cmp.Compare(b.Votes, a.Votes)
		}
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return cmp.Compare(i, j)
	})
	e := &Engine{titles: make([]entry, len(titles)), kinds: make([]uint8, len(titles))}
	var namePairs, alternatePairs []posting
	var shortPairs []shortPosting
	table := wordTable{numbers: map[string]int32{}}
	// Word postings as parallel (word, title) arrays, appended in title order.
	var wordKeys, wordIDs, altKeys, altIDs []int32
	var seenNames, seenWords, seenAliases []string
	seen := func(list []string, key string) bool { return slices.Contains(list, key) }
	addShort := func(key string, id int32) {
		if !seen(seenAliases, key) {
			if packed, ok := packShort(key); ok {
				shortPairs = append(shortPairs, shortPosting{packed, id})
			}
			seenAliases = append(seenAliases, key)
		}
	}
	for index, original := range order {
		t := &titles[original]
		id := int32(index)
		e.titles[index] = newEntry(*t)
		e.kinds[index] = kindOf(t.Type)
		seenNames, seenWords, seenAliases = seenNames[:0], seenWords[:0], seenAliases[:0]
		for n, name := range [2]string{t.Name, t.Original} {
			if n == 1 && (name == "" || name == t.Name) {
				continue
			}
			normal := Normalize(name)
			if normal == "" || seen(seenNames, normal) {
				continue
			}
			seenNames = append(seenNames, normal)
			namePairs = append(namePairs, posting{normal, id})
			parts := strings.Fields(normal)
			for _, word := range parts {
				if !seen(seenWords, word) {
					wordKeys = append(wordKeys, table.number(word))
					wordIDs = append(wordIDs, id)
					seenWords = append(seenWords, word)
				}
			}
			// Acronyms of the full name and of the part before any colon.
			for v, variant := range [2][]string{parts, nil} {
				if v == 1 {
					head, _, found := strings.Cut(name, ":")
					if !found {
						variant = parts
					} else {
						variant = strings.Fields(Normalize(head))
					}
				}
				if len(variant) < 2 {
					continue
				}
				var all, withoutArticle, significant [64]byte
				na, nw, ns := 0, 0, 0
				latin := true
				for p, part := range variant {
					first := part[0] // normalized words start with a letter, digit or non-Latin rune
					if first < 'a' || first > 'z' {
						latin = false
						break
					}
					if na < len(all) {
						all[na] = first
						na++
					}
					if (p > 0 || !stop[part]) && nw < len(withoutArticle) {
						withoutArticle[nw] = first
						nw++
					}
					if !stop[part] && ns < len(significant) {
						significant[ns] = first
						ns++
					}
				}
				if !latin {
					continue
				}
				for _, alias := range [3][]byte{all[:na], withoutArticle[:nw], significant[:ns]} {
					if len(alias) >= 3 && len(alias) <= 8 && !stop[string(alias)] {
						addShort(string(alias), id)
					}
				}
			}
		}
		// Alternative titles: exact, joined and word forms only. No acronyms,
		// which would multiply ambiguous short keys across languages.
		for _, name := range t.Aliases {
			normal := Normalize(name)
			if seen(seenNames, normal) || !textnorm.Meaningful(normal) {
				continue
			}
			seenNames = append(seenNames, normal)
			e.alternateCount++
			// Short Latin aliases such as "H.O.T.D." or "Oben" behave like
			// acronyms: several titles legitimately share them, so they rank in
			// the acronym band where popularity decides, not above it.
			if compact := strings.ReplaceAll(normal, " ", ""); len(compact) <= 5 && isASCII(compact) {
				addShort(compact, id)
			} else {
				alternatePairs = append(alternatePairs, posting{normal, id})
			}
			for _, word := range strings.Fields(normal) {
				if !seen(seenWords, word) {
					altKeys = append(altKeys, table.number(word))
					altIDs = append(altIDs, id)
					seenWords = append(seenWords, word)
				}
			}
		}
	}
	e.names = newKeyIndex(namePairs)
	e.alternate = newKeyIndex(alternatePairs)
	e.acronyms = newShortIndex(shortPairs)

	// Number words in sorted order so prefix scans and candidate ties are
	// deterministic and match plain string order.
	order = make([]int32, len(table.words))
	for i := range order {
		order[i] = int32(i)
	}
	slices.SortFunc(order, func(a, b int32) int { return strings.Compare(table.words[a], table.words[b]) })
	final := make([]int32, len(order))
	e.vocabulary = make([]string, len(order))
	e.runeLengths = make([]uint8, len(order))
	for position, provisional := range order {
		final[provisional] = int32(position)
		word := table.words[provisional]
		e.vocabulary[position] = word
		e.runeLengths[position] = uint8(min(255, utf8.RuneCountInString(word)))
	}
	table = wordTable{}
	for i := range wordKeys {
		wordKeys[i] = final[wordKeys[i]]
	}
	for i := range altKeys {
		altKeys[i] = final[altKeys[i]]
	}
	e.words = buildPostings(wordKeys, wordIDs, len(e.vocabulary))
	e.alternateWords = buildPostings(altKeys, altIDs, len(e.vocabulary))

	// Trigram lists, in vocabulary order so bounded candidate scans are
	// deterministic between runs.
	gramNumbers := map[string]int32{}
	var gramKeys, gramWords []int32
	for position, word := range e.vocabulary {
		if n := e.runeLengths[position]; n >= 3 && n <= 40 {
			for _, g := range uniqueGrams(word) {
				number, ok := gramNumbers[g]
				if !ok {
					number = int32(len(e.grams))
					gramNumbers[g] = number
					e.grams = append(e.grams, g)
				}
				gramKeys = append(gramKeys, number)
				gramWords = append(gramWords, int32(position))
			}
		}
	}
	// Renumber grams in sorted order so lookups can binary search.
	gramOrder := make([]int32, len(e.grams))
	for i := range gramOrder {
		gramOrder[i] = int32(i)
	}
	slices.SortFunc(gramOrder, func(a, b int32) int { return strings.Compare(e.grams[a], e.grams[b]) })
	gramFinal := make([]int32, len(gramOrder))
	sortedGrams := make([]string, len(gramOrder))
	for position, provisional := range gramOrder {
		gramFinal[provisional] = int32(position)
		sortedGrams[position] = e.grams[provisional]
	}
	for i := range gramKeys {
		gramKeys[i] = gramFinal[gramKeys[i]]
	}
	e.grams = sortedGrams
	e.gramWords = buildPostings(gramKeys, gramWords, len(e.grams))
	return e
}

// word returns a word's position in the vocabulary, or -1.
func (e *Engine) word(text string) int32 {
	i, found := slices.BinarySearch(e.vocabulary, text)
	if !found {
		return -1
	}
	return int32(i)
}

func (e *Engine) gram(text string) []int32 {
	i, found := slices.BinarySearch(e.grams, text)
	if !found {
		return nil
	}
	return e.gramWords.get(int32(i))
}
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
	word   int32
	weight float64
}

func (e *Engine) terms(ctx context.Context, query string) []termMatch {
	matches := map[int32]float64{}
	exact := e.word(query)
	if exact >= 0 {
		matches[exact] = 1
	}
	length := utf8.RuneCountInString(query)
	if length >= 3 {
		start := exact
		if start < 0 {
			i, _ := slices.BinarySearch(e.vocabulary, query)
			start = int32(i)
		}
		for i := start; int(i) < len(e.vocabulary) && i < start+20 && strings.HasPrefix(e.vocabulary[i], query); i++ {
			if _, ok := matches[i]; !ok {
				matches[i] = .88
			}
		}
	}
	// Short words have only two to four trigrams, so a single central typo
	// ("rene" for "reine") can share none of them. For short ASCII terms, look
	// up every single-edit variant directly: a few hundred allocation-free
	// binary searches in the vocabulary.
	if length >= 4 && length <= 6 && isASCII(query) {
		forEachEdit(query, func(variant []byte) {
			if w := e.word(string(variant)); w >= 0 && matches[w] < .76 {
				matches[w] = .76
			}
		})
	}
	if length >= 4 && length <= 40 {
		counts := map[int32]int{}
		scanned := 0
		for _, gram := range uniqueGrams(query) {
			for _, word := range e.gram(gram) {
				if scanned++; scanned > 20000 {
					break
				}
				if abs(int(e.runeLengths[word])-length) <= 2 {
					counts[word]++
				}
			}
			if scanned > 20000 {
				break
			}
		}
		type candidate struct {
			word    int32
			overlap int
		}
		candidates := make([]candidate, 0, len(counts))
		for word, count := range counts {
			candidates = append(candidates, candidate{word, count})
		}
		// Only the 96 best candidates are compared; select them rather than
		// sorting thousands. Word number order is string order.
		better := func(a, b candidate) int {
			if a.overlap != b.overlap {
				return cmp.Compare(b.overlap, a.overlap)
			}
			return cmp.Compare(a.word, b.word)
		}
		candidates = smallest(candidates, 96, better)
		limit := 1
		if length >= 7 {
			limit = 2
		}
		for _, c := range candidates {
			if ctx.Err() != nil {
				break
			}
			d := distance(query, e.vocabulary[c.word], limit)
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
	slices.SortFunc(result, func(a, b termMatch) int {
		if a.weight != b.weight {
			return cmp.Compare(b.weight, a.weight)
		}
		return cmp.Compare(a.word, b.word)
	})
	if len(result) > 32 {
		result = result[:32]
	}
	return result
}

// smallest returns the n smallest items in ascending order under compare, a
// strict total order. It partially reorders items.
func smallest[T any](items []T, n int, compare func(a, b T) int) []T {
	if n <= 0 {
		return items[:0]
	}
	if len(items) > 4*n {
		// Keep a sorted best-n prefix; most items lose to its last element.
		best := slices.Clone(items[:n])
		slices.SortFunc(best, compare)
		for _, item := range items[n:] {
			if compare(item, best[n-1]) >= 0 {
				continue
			}
			i, _ := slices.BinarySearchFunc(best, item, compare)
			copy(best[i+1:], best[i:n-1])
			best[i] = item
		}
		return best
	}
	slices.SortFunc(items, compare)
	return items[:min(n, len(items))]
}

type member struct {
	id     int32
	weight float64
}

// scratch is per-search working memory, reused between searches so broad
// queries do not allocate large maps each time. stamp marks which generation
// a title's slot belongs to, so nothing needs clearing between uses.
type scratch struct {
	stamp  []uint32
	slot   []int32
	gen    uint32
	groups [][]member
}

func (w *scratch) next() uint32 {
	if w.gen++; w.gen == 0 {
		clear(w.stamp)
		w.gen = 1
	}
	return w.gen
}

// At most this many idle scratch areas are kept (the helper runs two searches
// at a time); extra concurrent searches allocate and drop their own.
const keptScratch = 2

func (e *Engine) scratch() *scratch {
	e.scratchMu.Lock()
	defer e.scratchMu.Unlock()
	if n := len(e.idle); n > 0 {
		w := e.idle[n-1]
		e.idle = e.idle[:n-1]
		return w
	}
	return &scratch{stamp: make([]uint32, len(e.titles)), slot: make([]int32, len(e.titles))}
}

func (e *Engine) release(w *scratch) {
	for i := range w.groups {
		if cap(w.groups[i]) > 1<<16 {
			w.groups[i] = nil // keep only typical sizes between searches
		}
	}
	e.scratchMu.Lock()
	defer e.scratchMu.Unlock()
	if len(e.idle) < keptScratch {
		e.idle = append(e.idle, w)
	}
}

func (e *Engine) Search(ctx context.Context, query, kind string, limit int) []catalog.Title {
	want := kindOf(kind)
	if utf8.RuneCountInString(query) > 120 || want == 0 {
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
	scores := map[int32]float64{}
	seedOne := func(id int32, score float64) {
		if e.kinds[id] == want {
			if old, ok := scores[id]; !ok || score > old {
				scores[id] = score
			}
		}
	}
	seed := func(ids []int32, score float64) {
		for _, id := range ids {
			seedOne(id, score)
		}
	}
	joined := strings.ReplaceAll(q, " ", "")
	seed(e.names.lookup(q), scoreExact)
	// Joined forms: a single-word name equal to the query without spaces, or a
	// multi-word name equal to it once its own spaces are removed.
	seed(e.names.lookup(joined), scoreCompact)
	e.names.eachJoined(joined, func(id int32) { seedOne(id, scoreCompact) })
	seed(e.alternate.lookup(q), scoreAlternate)
	e.alternate.eachJoined(joined, func(id int32) { seedOne(id, scoreAlternateCompact) })
	seed(e.acronyms.lookup(q), scoreAcronym)
	unique := map[string]bool{}
	var work *scratch
	var groups [][]member
	for _, part := range parts {
		if stop[part] || unique[part] {
			continue
		}
		unique[part] = true
		if utf8.RuneCountInString(part) < 3 {
			continue
		}
		if work == nil {
			work = e.scratch()
			defer e.release(work)
		}
		// One group per query word: every title matching any of the word's
		// terms, with its best weight.
		gen := work.next()
		var group []member
		if len(groups) < len(work.groups) {
			group = work.groups[len(groups)][:0]
		}
		add := func(id int32, weight float64) {
			if e.kinds[id] != want {
				return
			}
			if work.stamp[id] != gen {
				work.stamp[id] = gen
				work.slot[id] = int32(len(group))
				group = append(group, member{id, weight})
			} else if m := &group[work.slot[id]]; weight > m.weight {
				m.weight = weight
			}
		}
		for _, match := range e.terms(ctx, part) {
			for n, id := range e.words.get(match.word) {
				if n >= 3000 || ctx.Err() != nil {
					break
				}
				add(id, match.weight)
			}
			for n, id := range e.alternateWords.get(match.word) {
				if n >= 3000 || ctx.Err() != nil {
					break
				}
				add(id, match.weight*alternateWordWeight)
			}
		}
		groups = append(groups, group)
	}
	if work != nil {
		// Hand the (possibly regrown) lists back for reuse.
		for i, group := range groups {
			if i < len(work.groups) {
				work.groups[i] = group
			} else {
				work.groups = append(work.groups, group)
			}
		}
	}
	type hit struct {
		id    int32
		score float64
	}
	var hits []hit
	if len(groups) > 0 && ctx.Err() == nil {
		// A title must match every word. Start from the smallest group and add
		// each other group's weight in turn, in the same order as before.
		sort.Slice(groups, func(i, j int) bool { return len(groups[i]) < len(groups[j]) })
		candidates := groups[0]
		for _, group := range groups[1:] {
			gen := work.next()
			for i, m := range group {
				work.stamp[m.id] = gen
				work.slot[m.id] = int32(i)
			}
			kept := candidates[:0]
			for _, c := range candidates {
				if work.stamp[c.id] == gen {
					c.weight += group[work.slot[c.id]].weight
					kept = append(kept, c)
				}
			}
			candidates = kept
		}
		hits = make([]hit, 0, len(candidates)+len(scores))
		for _, c := range candidates {
			score := 100 * c.weight / float64(len(groups))
			if old, ok := scores[c.id]; ok {
				score = max(score, old)
				delete(scores, c.id)
			}
			hits = append(hits, hit{c.id, score})
		}
	}
	for id, score := range scores {
		hits = append(hits, hit{id, score})
	}
	for i := range hits {
		hits[i].score += math.Log1p(float64(max(0, e.titles[hits[i].id].votes))) * 5
	}
	limit = min(max(limit, 0), 50)
	hits = smallest(hits, limit, func(a, b hit) int {
		if a.score != b.score {
			return cmp.Compare(b.score, a.score)
		}
		return compareIDs(&e.titles[a.id], &e.titles[b.id])
	})
	result := make([]catalog.Title, 0, len(hits))
	for _, h := range hits {
		t := &e.titles[h.id]
		result = append(result, catalog.Title{ID: t.id(), Name: t.name, Type: kind, Year: int(t.year), Votes: int(t.votes), Rating: t.rating})
	}
	return result
}
