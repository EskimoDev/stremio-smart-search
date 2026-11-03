package search

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// The index is built once per catalogue and then only read, so it uses sorted
// arrays and 32-bit title numbers instead of Go maps of slices. A map entry
// costs a key header, a slice header and bucket overhead for what is usually a
// single posting; flat arrays keep the whole index well under the helper's
// 256 MiB soft memory target even for Expanded, so the garbage collector does
// not have to keep re-scanning a heap that sits above it.

const (
	kindMovie uint8 = iota + 1
	kindSeries
)

func kindOf(kind string) uint8 {
	switch kind {
	case "movie":
		return kindMovie
	case "series":
		return kindSeries
	}
	return 0
}

// entry is what a result needs from a catalogue title. The identifier is kept
// as a number plus its digit count ("tt0816692" is 816692 and 7), which
// reproduces the original text exactly without storing a string per title.
type entry struct {
	name   string
	number uint64
	rating float64
	votes  int32
	year   int32
	digits uint8
	// words is how many distinct significant words the primary name has
	// (articles excluded, at most 255; see significantWords). It fits in the
	// struct's padding.
	words uint8
}

func newEntry(t catalog.Title) entry {
	number, _ := strconv.ParseUint(t.ID[2:], 10, 64) // Validate guarantees 1–18 digits
	return entry{name: t.Name, number: number, rating: t.Rating, votes: clamp32(t.Votes), year: clamp32(t.Year), digits: uint8(len(t.ID) - 2)}
}

// significantWords counts the distinct words of a normalized name that are
// not articles, at most 255.
func significantWords(parts []string) uint8 {
	n := 0
	for i, part := range parts {
		if !stop[part] && !slices.Contains(parts[:i], part) {
			n++
		}
	}
	return uint8(min(n, 255))
}

func clamp32(n int) int32 { return int32(max(math.MinInt32, min(math.MaxInt32, n))) }

// appendID writes the original "tt…" identifier to buf.
func (e *entry) appendID(buf []byte) []byte {
	buf = append(buf, 't', 't')
	var digits [20]byte
	n := strconv.AppendUint(digits[:0], e.number, 10)
	for i := len(n); i < int(e.digits); i++ {
		buf = append(buf, '0')
	}
	return append(buf, n...)
}

func (e *entry) id() string { return string(e.appendID(make([]byte, 0, 2+int(e.digits)))) }

// compareIDs orders two entries by their identifier text, as the original
// string comparison did ("tt10000000" sorts before "tt9999999").
func compareIDs(a, b *entry) int {
	var x, y [24]byte
	return strings.Compare(string(a.appendID(x[:0])), string(b.appendID(y[:0])))
}

// postingList maps dense key numbers 0..n-1 to runs of title numbers stored in
// one shared array.
type postingList struct {
	start []uint32 // len n+1
	ids   []int32
}

func (p postingList) get(key int32) []int32 {
	if key < 0 || int(key)+1 >= len(p.start) {
		return nil
	}
	return p.ids[p.start[key]:p.start[key+1]]
}

// buildPostings groups (key, id) pairs by key with a counting sort. Pairs must
// be appended in ascending id order; that order is kept within each key.
func buildPostings(keys, ids []int32, n int) postingList {
	p := postingList{start: make([]uint32, n+1), ids: make([]int32, len(ids))}
	for _, k := range keys {
		p.start[k+1]++
	}
	for k := 1; k <= n; k++ {
		p.start[k] += p.start[k-1]
	}
	next := slices.Clone(p.start[:n])
	for i, k := range keys {
		p.ids[next[k]] = ids[i]
		next[k]++
	}
	return p
}

// posting pairs a key with an index into Engine.titles.
type posting struct {
	key string
	id  int32
}

func sortPostings(pairs []posting) {
	slices.SortFunc(pairs, func(a, b posting) int {
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
}

// keyIndex is a sorted, allocation-light exact-key lookup: most names map to
// one title, so a map of slices would spend more on headers and buckets than
// on the data itself. joined orders the multi-word keys as if their spaces were
// removed, so "lareinedesneiges" finds "la reine des neiges" without storing a
// second copy of each key.
type keyIndex struct {
	keys   []string
	ids    []int32
	joined []int32
}

func newKeyIndex(pairs []posting) keyIndex {
	sortPostings(pairs)
	k := keyIndex{keys: make([]string, 0, len(pairs)), ids: make([]int32, 0, len(pairs))}
	multi := 0
	for i, p := range pairs {
		if i > 0 && p == pairs[i-1] {
			continue // the same name twice for one title
		}
		k.keys = append(k.keys, p.key)
		k.ids = append(k.ids, p.id)
		if strings.IndexByte(p.key, ' ') >= 0 {
			multi++
		}
	}
	k.joined = make([]int32, 0, multi)
	for i, key := range k.keys {
		if strings.IndexByte(key, ' ') >= 0 {
			k.joined = append(k.joined, int32(i))
		}
	}
	slices.SortStableFunc(k.joined, func(a, b int32) int { return compareJoined(k.keys[a], k.keys[b]) })
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
	start, _ := slices.BinarySearch(k.keys, key)
	end := start
	for end < len(k.keys) && k.keys[end] == key {
		end++
	}
	return k.ids[start:end]
}

// eachJoined visits titles whose multi-word key equals joined once spaces are
// removed.
func (k keyIndex) eachJoined(joined string, visit func(int32)) {
	start, _ := slices.BinarySearchFunc(k.joined, joined, func(i int32, target string) int { return compareJoined(k.keys[i], target) })
	for i := start; i < len(k.joined) && compareJoined(k.keys[k.joined[i]], joined) == 0; i++ {
		visit(k.ids[k.joined[i]])
	}
}

// shortIndex is an exact lookup for keys of at most eight bytes (acronyms and
// very short alternative titles), packed into integers so no strings are kept.
type shortIndex struct {
	keys []uint64
	ids  []int32
}

type shortPosting struct {
	key uint64
	id  int32
}

func packShort(key string) (uint64, bool) {
	if len(key) == 0 || len(key) > 8 {
		return 0, false
	}
	var n uint64
	for i := 0; i < 8; i++ {
		n <<= 8
		if i < len(key) {
			n |= uint64(key[i])
		}
	}
	return n, true
}

func newShortIndex(pairs []shortPosting) shortIndex {
	slices.SortFunc(pairs, func(a, b shortPosting) int {
		if c := cmp.Compare(a.key, b.key); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
	s := shortIndex{keys: make([]uint64, len(pairs)), ids: make([]int32, len(pairs))}
	for i, p := range pairs {
		s.keys[i], s.ids[i] = p.key, p.id
	}
	return s
}

func (s shortIndex) lookup(key string) []int32 {
	packed, ok := packShort(key)
	if !ok {
		return nil
	}
	start, _ := slices.BinarySearch(s.keys, packed)
	end := start
	for end < len(s.keys) && s.keys[end] == packed {
		end++
	}
	return s.ids[start:end]
}
