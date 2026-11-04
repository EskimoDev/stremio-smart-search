package search

import (
	"cmp"
	"context"
	"math"
	"runtime"
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
	wide   map[int32]wide // the few titles whose number or rating an entry cannot hold
	kinds  []uint8        // kindMovie or kindSeries per title, kept apart for cache-friendly filtering
	// Display names share one immutable backing string. The catalogue bounds
	// them to 500,000 names of at most 1,000 bytes, so 32-bit offsets suffice.
	displayNames string
	nameStarts   []uint32
	// names holds every normalized canonical name (primary and original). It
	// answers exact queries and, through its joined order, queries typed
	// without spaces.
	names    keyIndex
	acronyms shortIndex // acronyms of canonical names, and short alternative titles
	// vocabulary is every indexed word, sorted; a word is referred to by its
	// position. words and alternateWords give each word's titles in popularity
	// order; gramWords gives each trigram's (or short word's bigram's) words.
	vocabulary []string
	// starts[k] is the first vocabulary position whose leading two bytes
	// give bucket k (see bucketOf), so a word lookup binary searches a few
	// hundred words instead of the whole vocabulary.
	starts []int32
	// filter is a Bloom filter over the vocabulary. Typo probes look up
	// hundreds of variants per query word and almost none are words, so most
	// lookups end here without a binary search.
	filter         wordFilter
	runeLengths    []uint8
	words          postingList
	alternateWords postingList
	grams          []string
	gramWords      postingList
	// gramLengths is set when each gramWords entry carries its word's
	// length in the bits above gramLengthShift (see gramEntry).
	gramLengths bool
	// Alternative (regional/translated) titles. Their exact forms and words are
	// kept apart from canonical ones so they can rank slightly lower, but they
	// share the vocabulary and trigram index, and therefore all typo handling.
	alternate      keyIndex
	alternateCount int
	// sounds files vocabulary words of ASCII letters by how they sound, for
	// the last-resort sound-alike lookup (see phonetic.go).
	sounds soundIndex
	// skeletons lists vocabulary words in consonant-skeleton order, for
	// queries typed without vowels (see skeleton.go).
	skeletons []int32

	scratchMu sync.Mutex
	idle      []*scratch
}

// Score bands. An exact alternative title is as exact as a canonical one, so
// they share a band and popularity decides between them: with Spanish titles
// on, "cadena perpetua" means The Shawshank Redemption before an obscure
// film whose original title it is. Short alternative titles still rank with
// acronyms, and alternative-title word matches are discounted slightly so
// canonical words win ties.
const (
	scoreExact            = 2200
	scoreCompact          = 2100
	scoreAlternate        = scoreExact
	scoreAlternateCompact = scoreCompact
	scoreAcronym          = 1200
	alternateWordWeight   = .9
	// coverageBonus is added in full to a word match that covers every
	// significant word of the title, and in proportion otherwise. Word matches
	// score 0–100, so it reorders close matches, not exact or acronym bands.
	coverageBonus = 10
)

var stop = map[string]bool{"a": true, "an": true, "the": true, "of": true, "and": true}

// Normalize is shared with the catalogue builder so alias deduplication and
// the index agree exactly.
func Normalize(text string) string { return textnorm.Normalize(text) }

func uniqueGrams(word string) []string {
	if isASCII(word) {
		// Grams are substrings of the word, so nothing is copied.
		n := 3
		if len(word) < 4 {
			n = 2
		}
		result := make([]string, 0, max(0, len(word)-n+1))
		for i := 0; i+n <= len(word); i++ {
			if g := word[i : i+n]; !slices.Contains(result, g) {
				result = append(result, g)
			}
		}
		return result
	}
	runes := []rune(word)
	n := 3
	if len(runes) < 4 {
		n = 2
	}
	result := []string{}
	for i := 0; i+n <= len(runes); i++ {
		if g := string(runes[i : i+n]); !slices.Contains(result, g) {
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
	nameBytes := 0
	for _, i := range order {
		nameBytes += len(titles[i].Name)
	}
	var names strings.Builder
	names.Grow(nameBytes)
	e.nameStarts = make([]uint32, len(order)+1)
	for position, i := range order {
		e.nameStarts[position] = uint32(names.Len())
		names.WriteString(titles[i].Name)
	}
	e.nameStarts[len(order)] = uint32(names.Len())
	e.displayNames = names.String()
	var namePairs, alternatePairs []posting
	var shortPairs []shortPosting
	table := wordTable{numbers: make(map[string]int32, len(titles)/2)}
	// Word postings as parallel (word, title) arrays, appended in title order.
	var wordKeys, wordIDs, altKeys, altIDs []int32
	// Everything about one title except numbering its words needs nothing
	// shared, so it is prepared on every core a block of titles at a time.
	// The block is then added in title order, which keeps the index the same
	// on any machine.
	block := make([]prepared, min(len(order), prepareBlock))
	for first := 0; first < len(order); first += prepareBlock {
		part := order[first:min(len(order), first+prepareBlock)]
		e.prepareBlock(titles, part, block, first)
		for i := range part {
			p := &block[i]
			id := int32(first + i)
			for _, name := range p.names {
				namePairs = append(namePairs, posting{name, id})
			}
			for _, word := range p.words {
				wordKeys = append(wordKeys, table.number(word))
				wordIDs = append(wordIDs, id)
			}
			for _, key := range p.shorts {
				shortPairs = append(shortPairs, shortPosting{key, id})
			}
			for _, name := range p.alternates {
				alternatePairs = append(alternatePairs, posting{name, id})
			}
			for _, word := range p.alternateWords {
				altKeys = append(altKeys, table.number(word))
				altIDs = append(altIDs, id)
			}
			e.alternateCount += p.alternateCount
		}
	}
	// The exact-name indexes and the word indexes share nothing from here
	// on, so they are built side by side.
	var wg sync.WaitGroup
	parallel := func(build func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			build()
		}()
	}
	parallel(func() { e.names = newKeyIndex(namePairs) })
	parallel(func() { e.alternate = newKeyIndex(alternatePairs) })
	parallel(func() { e.acronyms = newShortIndex(shortPairs) })

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
	e.starts = newStarts(e.vocabulary)
	e.filter = newWordFilter(e.vocabulary)
	for i := range wordKeys {
		wordKeys[i] = final[wordKeys[i]]
	}
	for i := range altKeys {
		altKeys[i] = final[altKeys[i]]
	}
	e.words = buildPostings(wordKeys, wordIDs, len(e.vocabulary))
	e.alternateWords = buildPostings(altKeys, altIDs, len(e.vocabulary))
	parallel(func() { e.sounds = newSoundIndex(e.vocabulary) })
	parallel(func() { e.skeletons = newSkeletons(e.vocabulary) })

	// Trigram lists, in vocabulary order so bounded candidate scans are
	// deterministic between runs.
	gramNumbers := map[string]int32{}
	var gramKeys, gramWords []int32
	e.gramLengths = len(e.vocabulary) <= gramWordMask+1
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
				gramWords = append(gramWords, e.gramEntry(int32(position), n))
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
	wg.Wait()
	return e
}

// prepared is what one title adds to the index, apart from word numbers.
type prepared struct {
	names          []string // normalized canonical names, distinct
	words          []string // their distinct words, in order
	shorts         []uint64 // packed acronyms and short alternative titles
	alternates     []string // other normalized alternative titles
	alternateWords []string // alternative-title words not already in words
	alternateCount int
	// seenNames (canonical and alternative) and seenShorts deduplicate.
	seenNames, seenShorts []string
}

// prepareBlock is how many titles are prepared at a time: enough to keep
// every core busy, few enough that the strings held at once stay small.
const prepareBlock = 4096

// prepareBlock prepares titles[order[i]] into block[i], and sets their
// entries and kinds from position first on, using every core.
func (e *Engine) prepareBlock(titles []catalog.Title, order []int32, block []prepared, first int) {
	wides := make([]*wide, len(order)) // nil for almost every title
	defer func() {
		for i, w := range wides {
			if w != nil {
				if e.wide == nil {
					e.wide = map[int32]wide{}
				}
				e.wide[int32(first+i)] = *w
			}
		}
	}()
	workers := min(runtime.GOMAXPROCS(0), 8)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := w; i < len(order); i += workers {
				t := &titles[order[i]]
				var w *wide
				e.titles[first+i], w = newEntry(*t)
				if w != nil {
					wides[i] = w
				}
				e.kinds[first+i] = kindOf(t.Type)
				e.titles[first+i].words = block[i].prepare(t)
			}
		}()
	}
	wg.Wait()
}

// prepare fills p from t and returns how many significant words its primary
// name has.
func (p *prepared) prepare(t *catalog.Title) uint8 {
	p.names, p.words, p.shorts = p.names[:0], p.words[:0], p.shorts[:0]
	p.alternates, p.alternateWords = p.alternates[:0], p.alternateWords[:0]
	p.seenNames, p.seenShorts = p.seenNames[:0], p.seenShorts[:0]
	p.alternateCount = 0
	words := uint8(0)
	addShort := func(key string) {
		if !slices.Contains(p.seenShorts, key) {
			if packed, ok := packShort(key); ok {
				p.shorts = append(p.shorts, packed)
			}
			p.seenShorts = append(p.seenShorts, key)
		}
	}
	for n, name := range [2]string{t.Name, t.Original} {
		if n == 1 && (name == "" || name == t.Name) {
			continue
		}
		normal := Normalize(name)
		if normal == "" || slices.Contains(p.seenNames, normal) {
			continue
		}
		p.seenNames = append(p.seenNames, normal)
		p.names = append(p.names, normal)
		parts := strings.Fields(normal)
		if n == 0 {
			words = significantWords(parts)
		}
		for _, word := range parts {
			if !slices.Contains(p.words, word) {
				p.words = append(p.words, word)
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
			for i, part := range variant {
				first := part[0] // normalized words start with a letter, digit or non-Latin rune
				if first < 'a' || first > 'z' {
					latin = false
					break
				}
				if na < len(all) {
					all[na] = first
					na++
				}
				if (i > 0 || !stop[part]) && nw < len(withoutArticle) {
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
					addShort(string(alias))
				}
			}
		}
	}
	// Alternative titles: exact, joined and word forms only. No acronyms,
	// which would multiply ambiguous short keys across languages.
	for _, name := range t.Aliases {
		normal := Normalize(name)
		if slices.Contains(p.seenNames, normal) || !textnorm.Meaningful(normal) {
			continue
		}
		p.seenNames = append(p.seenNames, normal)
		p.alternateCount++
		// Short Latin aliases such as "H.O.T.D." or "Oben" behave like
		// acronyms: several titles legitimately share them, so they rank in
		// the acronym band where popularity decides, not above it.
		if compact := strings.ReplaceAll(normal, " ", ""); len(compact) <= 5 && isASCII(compact) {
			addShort(compact)
		} else {
			p.alternates = append(p.alternates, normal)
		}
		for _, word := range strings.Fields(normal) {
			if !slices.Contains(p.words, word) && !slices.Contains(p.alternateWords, word) {
				p.alternateWords = append(p.alternateWords, word)
			}
		}
	}
	return words
}

// word returns a word's position in the vocabulary, or -1.
func (e *Engine) word(text string) int32 {
	if !e.filter.mayContain(text) {
		return -1
	}
	i, found := e.find(text)
	if !found {
		return -1
	}
	return int32(i)
}

// find is slices.BinarySearch over the vocabulary, narrowed to text's bucket.
func (e *Engine) find(text string) (int, bool) {
	if text == "" {
		return slices.BinarySearch(e.vocabulary, text)
	}
	k := bucketOf(text)
	lo, hi := int(e.starts[k]), int(e.starts[k+1])
	i, found := slices.BinarySearch(e.vocabulary[lo:hi], text)
	return lo + i, found
}

// wordFilter is a Bloom filter with two probes per word and at least 16 bits
// per word, so about one absent word in seventy gets past it.
type wordFilter struct {
	bits []uint64
	mask uint64
}

func newWordFilter(vocabulary []string) wordFilter {
	n := uint64(64)
	for n < 16*uint64(len(vocabulary)) {
		n *= 2
	}
	f := wordFilter{bits: make([]uint64, n/64), mask: n - 1}
	for _, word := range vocabulary {
		a, b := filterHash(word)
		f.bits[(a&f.mask)/64] |= 1 << (a % 64)
		f.bits[(b&f.mask)/64] |= 1 << (b % 64)
	}
	return f
}

func (f wordFilter) mayContain(word string) bool {
	if len(f.bits) == 0 {
		return true
	}
	a, b := filterHash(word)
	return f.bits[(a&f.mask)/64]&(1<<(a%64)) != 0 && f.bits[(b&f.mask)/64]&(1<<(b%64)) != 0
}

// filterHash is 64-bit FNV-1a, split into two probe positions.
func filterHash(word string) (uint64, uint64) {
	h := uint64(14695981039346656037)
	for i := 0; i < len(word); i++ {
		h ^= uint64(word[i])
		h *= 1099511628211
	}
	h ^= h >> 29 // FNV's high bits are weak for short keys
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 32
	return h, h>>32 | h<<32
}

// bucketOf numbers a non-empty word by its first two bytes. It never
// decreases along sorted string order, so each bucket is one run of the
// vocabulary.
func bucketOf(word string) int {
	if len(word) == 1 {
		return int(word[0]) << 8
	}
	return int(word[0])<<8 | int(word[1])
}

func newStarts(vocabulary []string) []int32 {
	starts := make([]int32, 1<<16+1)
	w := 0
	for k := range starts {
		for w < len(vocabulary) && (vocabulary[w] == "" || bucketOf(vocabulary[w]) < k) {
			w++
		}
		starts[k] = int32(w)
	}
	return starts
}

func (e *Engine) gram(text string) []int32 {
	i, found := slices.BinarySearch(e.grams, text)
	if !found {
		return nil
	}
	return e.gramWords.get(int32(i))
}

// A trigram list entry is a word number with the word's length (3 to 40
// letters) in its top bits, so the typo scan can skip words of the wrong
// length without reading runeLengths at a random place for each of them.
// A vocabulary too large for the packing keeps plain word numbers.
const (
	gramLengthShift = 25
	gramWordMask    = 1<<gramLengthShift - 1
)

func (e *Engine) gramEntry(word int32, length uint8) int32 {
	if e.gramLengths {
		return word | int32(length)<<gramLengthShift
	}
	return word
}

// gramWord splits a trigram list entry into its word number and length.
func (e *Engine) gramWord(entry int32) (int32, int) {
	if e.gramLengths {
		return entry & gramWordMask, int(entry >> gramLengthShift)
	}
	return entry, int(e.runeLengths[entry])
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

// distance is the Damerau-Levenshtein distance with adjacent transpositions
// (optimal string alignment) between two bounded title words, up to limit: the
// distance when it is at most limit, else limit+1. Two ASCII words, the usual
// case, are compared bit-parallel without copying; others rune by rune, so a
// typo lookup that compares a hundred candidates allocates nothing.
func distance(a, b string, limit int) int {
	if len(a) <= 64 && len(b) <= 64 {
		if abs(len(a)-len(b)) > limit && isASCII(a) && isASCII(b) {
			return limit + 1
		}
		if d, ok := asciiDistance(a, b); ok {
			return min(d, limit+1)
		}
	}
	return editDistance([]rune(a), []rune(b), limit)
}

// asciiDistance is the optimal string alignment distance between two ASCII
// words of at most 64 letters, by Hyyrö's bit-vector algorithm ("A bit-vector
// algorithm for computing Levenshtein and Damerau edit distances", 2003): one
// column of the edit table per letter of b, in a few word operations. ok is
// false, and nothing is computed, when either word is not ASCII.
func asciiDistance(a, b string) (d int, ok bool) {
	if len(a) == 0 {
		return len(b), isASCII(b)
	}
	var peq [utf8.RuneSelf]uint64 // per letter, its positions in a
	for i := 0; i < len(a); i++ {
		if a[i] >= utf8.RuneSelf {
			return 0, false
		}
		peq[a[i]] |= 1 << i
	}
	top := uint64(1) << (len(a) - 1)
	vp, vn := ^uint64(0), uint64(0)
	var d0prev, pmprev uint64
	d = len(a)
	for i := 0; i < len(b); i++ {
		if b[i] >= utf8.RuneSelf {
			return 0, false
		}
		pm := peq[b[i]]
		tr := (^d0prev & pm) << 1 & pmprev // a transposition ends here
		d0 := (pm&vp + vp) ^ vp | pm | vn | tr
		hp := vn | ^(d0 | vp)
		hn := d0 & vp
		if hp&top != 0 {
			d++
		} else if hn&top != 0 {
			d--
		}
		x := hp<<1 | 1
		vn = x & d0
		vp = hn<<1 | ^(x | d0)
		d0prev, pmprev = d0, pm
	}
	return d, true
}

const maxStackWord = 48

func editDistance[T byte | rune](x, y []T, limit int) int {
	if abs(len(x)-len(y)) > limit {
		return limit + 1
	}
	// Any alignment within limit edits keeps |i-j| <= limit, so only that
	// band of each row is computed; cells outside it count as over the limit.
	// Values within the limit are exact, and anything over it is limit+1.
	over := limit + 1
	var rows [3][maxStackWord + 1]int
	var previousPrevious, previous, current []int
	if len(y) <= maxStackWord {
		previousPrevious, previous, current = rows[0][:len(y)+1], rows[1][:len(y)+1], rows[2][:len(y)+1]
	} else {
		previousPrevious, previous, current = make([]int, len(y)+1), make([]int, len(y)+1), make([]int, len(y)+1)
	}
	for j := range previous {
		previous[j] = min(j, over)
		previousPrevious[j] = over
		current[j] = over
	}
	for i := 1; i <= len(x); i++ {
		low, high := max(1, i-limit), min(len(y), i+limit)
		// The cell left of the band is over the limit, or the first column.
		if low == 1 {
			current[0] = min(i, over)
		} else {
			current[low-1] = over
		}
		rowMin := current[low-1]
		for j := low; j <= high; j++ {
			cost := 0
			if x[i-1] != y[j-1] {
				cost = 1
			}
			c := min(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				c = min(c, previousPrevious[j-2]+1)
			}
			c = min(c, over)
			current[j] = c
			rowMin = min(rowMin, c)
		}
		if high < len(y) {
			current[high+1] = over // the cell right of the band
		}
		if rowMin > limit {
			return over
		}
		previousPrevious, previous, current = previous, current, previousPrevious
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

// terms finds the vocabulary words a query word may stand for. With typos
// false it keeps exact and prefix matches only.
func (e *Engine) terms(ctx context.Context, work *scratch, query string, typos, alone bool) []termMatch {
	return e.lookupTerms(ctx, work, query, typos, alone, false)
}

// quickTerms is terms without the trigram scan, vowel skeletons and
// sound-alikes: exact and prefix matches and the direct one-edit probes. It
// is for joined fragments ("gma e" as "gmae"), which are only worth a few
// lookups.
func (e *Engine) quickTerms(ctx context.Context, work *scratch, query string) []termMatch {
	return e.lookupTerms(ctx, work, query, true, false, true)
}

// work is the search's scratch area; with nil, lookupTerms borrows one.
func (e *Engine) lookupTerms(ctx context.Context, work *scratch, query string, typos, alone, quick bool) []termMatch {
	if work == nil {
		work = e.scratch()
		defer e.release(work)
	}
	matches := map[int32]float64{}
	exact := e.word(query)
	if exact >= 0 {
		matches[exact] = 1
	}
	length := utf8.RuneCountInString(query)
	if length >= 3 {
		for _, w := range e.prefixWords(query, exact) {
			if _, ok := matches[w]; !ok {
				matches[w] = prefixWeight(length, int(e.runeLengths[w]))
			}
		}
	}
	// Short words have only two to four trigrams, so a single central typo
	// ("rene" for "reine") can share none of them. For short ASCII terms, look
	// up every single-edit variant directly: a few hundred allocation-free
	// binary searches in the vocabulary.
	if !typos {
		return sortTerms(matches)
	}
	if length >= 4 && length <= 6 && isASCII(query) {
		forEachEdit(query, func(variant []byte) {
			if w := e.word(string(variant)); w >= 0 && matches[w] < .76 {
				matches[w] = .76
			}
		})
	}
	// A transposed or doubled letter in a longer word breaks up to four of its
	// trigrams, so "thornes" shares only one with "thrones" and can lose the
	// bounded candidate scan to thousands of unrelated words. Probe those two
	// edit kinds directly: about two lookups per letter.
	if length >= 7 && length <= 20 && isASCII(query) {
		forEachSwapOrDrop(query, func(variant []byte) {
			if w := e.word(string(variant)); w >= 0 && matches[w] < .76 {
				matches[w] = .76
			}
		})
	}
	if quick {
		return sortTerms(matches)
	}
	if length >= 4 && length <= 40 {
		// Count shared trigrams per word in the scratch area: a map here was
		// most of a search's garbage.
		gen := work.nextWordGen(len(e.vocabulary))
		touched := work.touched[:0]
		scanned := 0
		for _, gram := range uniqueGrams(query) {
			for _, entry := range e.gram(gram) {
				if scanned++; scanned > 20000 {
					break
				}
				if word, n := e.gramWord(entry); abs(n-length) <= 2 {
					if mark := work.wordMark[word]; mark>>wordCountBits != gen {
						work.wordMark[word] = gen<<wordCountBits | 1
						touched = append(touched, word)
					} else {
						work.wordMark[word] = mark + 1
					}
				}
			}
			if scanned > 20000 {
				break
			}
		}
		work.touched = touched
		candidates := work.candidates[:0]
		for _, word := range touched {
			candidates = append(candidates, gramCandidate{word, int32(work.wordMark[word] & wordCountMask)})
		}
		work.candidates = candidates
		// Only the 96 best candidates are compared; select them rather than
		// sorting thousands. Word number order is string order.
		better := func(a, b gramCandidate) int {
			if a.overlap != b.overlap {
				return cmp.Compare(b.overlap, a.overlap)
			}
			return cmp.Compare(a.word, b.word)
		}
		candidates = smallest(bestOverlaps(candidates, 96, &work.ties), 96, better)
		limit := 1
		if length >= 7 {
			limit = 2
		}
		for _, c := range candidates {
			if ctx.Err() != nil {
				break
			}
			// The scan admits words two letters longer or shorter, but a
			// short word is compared within one edit: skip reading those.
			if abs(int(e.runeLengths[c.word])-length) > limit {
				continue
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
	result := sortTerms(matches)
	// Words the query could be with its vowels left out come after the
	// spelling matches and, like sound-alikes, are never cut by the term
	// limit.
	if length >= 3 {
		for _, m := range e.skeletonTerms(query, matches) {
			matches[m.word] = m.weight
			result = append(result, termMatch{m.word, m.weight})
		}
	}
	// Sound-alikes come last and are never cut by the term limit; a word
	// already matched by spelling keeps its higher weight.
	sounds := e.soundsLike(work, query, alone)
	for _, w := range sounds {
		if _, ok := matches[w]; !ok {
			result = append(result, termMatch{w, soundWeight})
		}
	}
	if ctx.Err() != nil {
		return result
	}
	for _, w := range e.nearSounds(work, query, sounds, alone) {
		if _, ok := matches[w]; !ok {
			result = append(result, termMatch{w, nearWeight})
		}
	}
	return result
}

// shortTypos returns the words a three-letter query word may be with a typo,
// when it is not a word many titles use ("war", "man"). They are too short
// for the trigram scan, and most of their one-letter variants are unrelated
// words, so only a swapped pair ("gyu" for "guy") or one missing letter
// ("bak" for "back") is tried. Stop words are left out: "teh" adds nothing
// as "the".
func (e *Engine) shortTypos(query string) []termMatch {
	if len(query) != 3 || !isASCII(query) || !e.rareWord(query) {
		return nil
	}
	var found []termMatch
	probe := func(variant []byte, weight float64) {
		if stop[string(variant)] {
			return
		}
		if w := e.word(string(variant)); w >= 0 {
			found = append(found, termMatch{w, weight})
		}
	}
	forEachSwapOrDrop(query, func(variant []byte) {
		if len(variant) == 3 {
			probe(variant, shortSwapWeight)
		}
	})
	var buf [4]byte
	for i := 1; i <= 3; i++ {
		for c := byte('a'); c <= 'z'; c++ {
			probe(append(append(append(buf[:0], query[:i]...), c), query[i:]...), shortInsertWeight)
		}
	}
	return found
}

const (
	// shortSwapWeight and shortInsertWeight are a three-letter word's typo
	// weights, a little under a longer word's .76 since short words have
	// more accidental neighbours.
	shortSwapWeight   = .72
	shortInsertWeight = .68
	// prefixTerms is how many longer words a query word may stand for as a
	// prefix ("war" for "wars", "thr" for "thrones").
	prefixTerms = 20
	// prefixScan bounds the words considered for them. The longest three-letter
	// prefix run in the Expanded French vocabulary is about 1,000 words.
	prefixScan = 4096
)

// prefixWords returns up to prefixTerms vocabulary words that start with
// query and are longer than it, preferring words of more popular titles.
// Taking the first words alphabetically instead made short query words
// useless: "wars" is the 89th word starting with "war", so "star war" found
// nothing. A word's postings are in popularity order, so its first title is
// its most popular one, and ranking words by it costs two reads per word.
func (e *Engine) prefixWords(query string, exact int32) []int32 {
	start := exact + 1
	if exact < 0 {
		i, _ := e.find(query)
		start = int32(i)
	}
	rest := e.vocabulary[start:]
	n := sort.Search(len(rest), func(i int) bool { return !strings.HasPrefix(rest[i], query) })
	if n <= prefixTerms {
		words := make([]int32, n)
		for i := range words {
			words[i] = start + int32(i)
		}
		return words
	}
	type candidate struct{ best, word int32 }
	candidates := make([]candidate, 0, min(n, prefixScan))
	for w := start; w < start+int32(min(n, prefixScan)); w++ {
		best := int32(math.MaxInt32)
		if ids := e.words.get(w); len(ids) > 0 {
			best = ids[0]
		}
		if ids := e.alternateWords.get(w); len(ids) > 0 && ids[0] < best {
			best = ids[0]
		}
		candidates = append(candidates, candidate{best, w})
	}
	candidates = smallest(candidates, prefixTerms, func(a, b candidate) int {
		if a.best != b.best {
			return cmp.Compare(a.best, b.best)
		}
		return cmp.Compare(a.word, b.word)
	})
	words := make([]int32, len(candidates))
	for i, c := range candidates {
		words[i] = c.word
	}
	return words
}

// prefixWeight is a completion's weight: .88 for one more letter, a little
// less for each further one, down to a typo's .76. A query word is more likely
// cut short by a letter or two ("hom" in "spider man no way hom" is "home",
// not "homecoming").
func prefixWeight(query, word int) float64 {
	return max(.76, .88-.02*float64(word-query-1))
}

func sortTerms(matches map[int32]float64) []termMatch {
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

// bestOverlaps returns the n candidates with the most shared trigrams, ties
// going to the lower word number, in no particular order; smallest then has
// at most n to order instead of thousands. Overlaps are below
// 1<<wordCountBits, so a count per overlap finds the cut, and only the words
// tied at it need comparing. ties is scratch space.
func bestOverlaps(candidates []gramCandidate, n int, ties *[]int32) []gramCandidate {
	if len(candidates) <= n {
		return candidates
	}
	var counts [1 << wordCountBits]int
	for _, c := range candidates {
		counts[c.overlap]++
	}
	cut, above := len(counts)-1, 0 // above counts overlaps over cut
	for ; cut > 0 && above+counts[cut] < n; cut-- {
		above += counts[cut]
	}
	tied := (*ties)[:0]
	kept := candidates[:0]
	for _, c := range candidates {
		if c.overlap > int32(cut) {
			kept = append(kept, c)
		} else if c.overlap == int32(cut) {
			tied = append(tied, c.word)
		}
	}
	*ties = tied
	for _, w := range smallest(tied, n-above, cmp.Compare[int32]) {
		kept = append(kept, gramCandidate{w, int32(cut)})
	}
	return kept
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
// queries do not allocate large maps each time. A title's stamp marks which
// generation its slot belongs to, so nothing needs clearing between uses.
type scratch struct {
	marks  []mark // per title
	gen    uint32
	groups [][]member
	// tallies is matchWords' per-title count of matched query words.
	tallies []tally
	// wordMark counts, per vocabulary word, the trigrams it shares with a
	// query word: the low wordCountBits bits are the count and the rest is
	// wordGen, so nothing needs clearing between lookups. touched lists the
	// words counted in the current lookup.
	wordMark   []uint16
	wordGen    uint16
	touched    []int32
	candidates []gramCandidate
	ties       []int32 // bestOverlaps' words tied at its cut
	sounds     []soundCandidate
}

// mark is a title's slot in the current generation. Stamp and slot sit
// together so that looking a title up touches one cache line, not two.
type mark struct {
	stamp uint32
	slot  int32
}

// gramCandidate is a vocabulary word and how many trigrams it shares with a
// query word.
type gramCandidate struct {
	word    int32
	overlap int32
}

const (
	// wordCountBits holds a trigram count: a query word of up to 40 letters
	// has at most 38 distinct trigrams.
	wordCountBits = 6
	wordCountMask = 1<<wordCountBits - 1
)

// nextWordGen starts a new trigram count over a vocabulary of n words. The
// generation wraps every thousand lookups, and clearing then is cheaper than
// wider marks would be to keep.
func (w *scratch) nextWordGen(n int) uint16 {
	if len(w.wordMark) != n {
		w.wordMark = make([]uint16, n)
		w.wordGen = 0
	}
	if w.wordGen++; w.wordGen >= 1<<(16-wordCountBits) {
		clear(w.wordMark)
		w.wordGen = 1
	}
	return w.wordGen
}

func (w *scratch) next() uint32 {
	if w.gen++; w.gen == 0 {
		clear(w.marks)
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
	return &scratch{marks: make([]mark, len(e.titles))}
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
	// Bound the work by the words that are actually looked up: short words
	// and articles are skipped below, so a long translated title such as "el
	// señor de los anillos la comunidad del anillo" still gets its exact match.
	if len(parts) > 16 || indexedWords(parts) > 8 {
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
	guesses := e.respelled(q, parts)
	// "it 2017": the name without its year, released that year, is exact.
	if rest, year := splitYear(parts); year != 0 && rest != "" {
		for _, id := range e.names.lookup(rest) {
			if e.titles[id].year == year {
				seedOne(id, scoreExact)
			}
		}
	}
	// A single short word that is some title's acronym ("hotd") is taken as
	// that acronym: one-letter variants of it ("hot", "hood", "hota") are
	// unrelated titles, not typos, and would otherwise fill the results.
	typos := true
	if len(parts) == 1 && utf8.RuneCountInString(q) <= 5 {
		for _, id := range e.acronyms.lookup(q) {
			if e.kinds[id] == want {
				typos = false
				break
			}
		}
	}
	hits := e.matchWords(ctx, parts, typos, want, len(scores)+len(guesses))
	if len(guesses) > 0 {
		e.scoreGuesses(hits, guesses, seedOne)
	}
	if len(scores) > 0 {
		for i := range hits {
			if old, ok := scores[hits[i].id]; ok {
				hits[i].score = max(hits[i].score, old)
				delete(scores, hits[i].id)
			}
		}
	}
	for id, score := range scores {
		hits = append(hits, hit{id, score})
	}
	e.promoteFamous(hits, parts)
	for i := range hits {
		hits[i].score += math.Log1p(float64(max(0, e.titles[hits[i].id].votes))) * 5
	}
	limit = min(max(limit, 0), 50)
	hits = smallest(hits, limit, func(a, b hit) int {
		if a.score != b.score {
			return cmp.Compare(b.score, a.score)
		}
		return e.compareIDs(a.id, b.id)
	})
	result := make([]catalog.Title, 0, len(hits))
	for _, h := range hits {
		t := &e.titles[h.id]
		result = append(result, catalog.Title{ID: e.id(h.id), Name: e.nameOf(h.id), Type: kind, Year: int(t.year), Votes: int(t.votes), Rating: e.ratingOf(h.id)})
	}
	return result
}

// yearOf returns the year a query word names, or 0. Four digits from 1880 to
// 2099 cover IMDb's feature-film and series start years.
func yearOf(part string) int32 {
	if len(part) != 4 {
		return 0
	}
	n := int32(0)
	for i := 0; i < 4; i++ {
		if part[i] < '0' || part[i] > '9' {
			return 0
		}
		n = n*10 + int32(part[i]-'0')
	}
	if n < 1880 || n > 2099 {
		return 0
	}
	return n
}

// filterYear keeps candidates released in year or with the year as a title
// word ("Blade Runner 2049"), adding a full word's weight. If none qualify the
// year is treated as unmatched rather than emptying the results, since a
// remembered year is often off by one.
func (e *Engine) filterYear(work *scratch, candidates []member, year int32) []member {
	gen := work.next()
	var digits [4]byte
	for i, n := 3, year; i >= 0; i, n = i-1, n/10 {
		digits[i] = byte('0' + n%10)
	}
	if w := e.word(string(digits[:])); w >= 0 {
		for _, id := range e.words.get(w) {
			work.marks[id].stamp = gen
		}
	}
	matched := 0
	for _, c := range candidates {
		if e.titles[c.id].year == year || work.marks[c.id].stamp == gen {
			matched++
		}
	}
	if matched == 0 {
		return candidates
	}
	kept := candidates[:0]
	for _, c := range candidates {
		if e.titles[c.id].year == year || work.marks[c.id].stamp == gen {
			c.weight++
			kept = append(kept, c)
		}
	}
	return kept
}

// splitYear returns the query without its last year-like word, and that year.
func splitYear(parts []string) (string, int32) {
	for i := len(parts) - 1; i > 0; i-- {
		if y := yearOf(parts[i]); y != 0 {
			return strings.Join(append(append([]string(nil), parts[:i]...), parts[i+1:]...), " "), y
		}
	}
	return "", 0
}

// indexedWords counts the distinct query words that get a term lookup.
func indexedWords(parts []string) int {
	n := 0
	for i, part := range parts {
		if !stop[part] && utf8.RuneCountInString(part) >= 3 && !slices.Contains(parts[:i], part) {
			n++
		}
	}
	return n
}

// forEachSwapOrDrop visits every deletion and adjacent transposition of word.
// The buffer is reused, so visit must copy anything it keeps.
func forEachSwapOrDrop(word string, visit func([]byte)) {
	buf := make([]byte, 0, len(word))
	for i := 0; i < len(word); i++ {
		head, tail := word[:i], word[i:]
		visit(append(append(buf[:0], head...), tail[1:]...))
		if len(tail) > 1 && tail[0] != tail[1] {
			visit(append(append(append(buf[:0], head...), tail[1], tail[0]), tail[2:]...))
		}
	}
}
