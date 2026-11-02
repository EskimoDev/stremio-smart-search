package search

import (
	"cmp"
	"slices"
)

// Queries typed without most vowels ("strngr thngs", "fther", "gdfthr") are
// several edits from the words they mean, beyond the typo limit, and do not
// sound like anything. They do keep the word's consonants in order, so the
// vocabulary is also kept sorted by consonant skeleton: the word without its
// vowels, and with a consonant that repeats the previous one (even across
// vowels) written once ("stranger" is "strngr", "succession" is "scsn",
// "mirror" and "mrrr" are both "mr"). Only word numbers are stored, four bytes
// per word; skeletons are worked out while comparing, as joined keys are.

const (
	// skeletonWeight is a dropped-vowel match's weight: below a one-letter
	// typo (.76), above a two-letter one (.6). Every letter typed is still in
	// the word, in order, so it is a stronger hint than a sound-alike.
	skeletonWeight = .7
	// skeletonLongerWeight is for a skeleton that is also cut short by one
	// consonant ("thron" for "thrones").
	skeletonLongerWeight = .64
	// skeletonWords is how many words a query word may stand for this way,
	// the most popular first.
	skeletonWords = 8
	// skeletonScan bounds the words read for one skeleton.
	skeletonScan = 2048
	// minSkeleton is the fewest consonants a skeleton needs. Two ("gme" for
	// "game") are allowed only when nothing is added after them.
	minSkeleton = 2
)

// isSkeletonVowel reports whether the letter at i of word is left out of its
// skeleton. A leading y is a consonant ("yellow"), any other a vowel
// ("mystery" is "mstr").
func isSkeletonVowel(word string, i int) bool {
	switch word[i] {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	case 'y':
		return i > 0
	}
	return false
}

// skeletonOf appends word's skeleton to buf and reports whether word is made
// of ASCII letters only.
func skeletonOf(buf []byte, word string) ([]byte, bool) {
	var last byte
	for i := 0; i < len(word); i++ {
		c := word[i]
		if c < 'a' || c > 'z' {
			return buf, false
		}
		if isSkeletonVowel(word, i) || c == last {
			continue
		}
		buf = append(buf, c)
		last = c
	}
	return buf, true
}

// compareSkeleton compares word's skeleton with target without building it.
func compareSkeleton(word string, target []byte) int {
	j := 0
	var last byte
	for i := 0; i < len(word); i++ {
		c := word[i]
		if isSkeletonVowel(word, i) || c == last {
			continue
		}
		last = c
		if j == len(target) {
			return 1
		}
		if c != target[j] {
			return cmp.Compare(c, target[j])
		}
		j++
	}
	if j < len(target) {
		return -1
	}
	return 0
}

// newSkeletons returns the vocabulary positions of ASCII-letter words with a
// skeleton of at least minSkeleton letters, in skeleton order (ties in
// vocabulary order).
func newSkeletons(vocabulary []string) []int32 {
	type pair struct {
		skeleton string
		word     int32
	}
	pairs := make([]pair, 0, len(vocabulary))
	var buf []byte
	for position, word := range vocabulary {
		var ok bool
		buf, ok = skeletonOf(buf[:0], word)
		if ok && len(buf) >= minSkeleton && len(word) > len(buf) {
			pairs = append(pairs, pair{string(buf), int32(position)})
		}
	}
	slices.SortFunc(pairs, func(a, b pair) int {
		if c := cmp.Compare(a.skeleton, b.skeleton); c != 0 {
			return c
		}
		return cmp.Compare(a.word, b.word)
	})
	words := make([]int32, len(pairs))
	for i, p := range pairs {
		words[i] = p.word
	}
	return words
}

// isSubsequence reports whether every letter of query appears in word, in
// order.
func isSubsequence(query, word string) bool {
	j := 0
	for i := 0; i < len(word) && j < len(query); i++ {
		if word[i] == query[j] {
			j++
		}
	}
	return j == len(query)
}

type skeletonMatch struct {
	word   int32
	best   int32 // the word's most popular title
	weight float64
}

// dropsVowels reports whether query looks typed without vowels: it has at
// most one, and at least two consonants.
func dropsVowels(query string) bool {
	vowels := 0
	for i := 0; i < len(query); i++ {
		if isSkeletonVowel(query, i) {
			vowels++
		}
	}
	return vowels <= 1
}

// skeletonTerms returns up to skeletonWords vocabulary words that query could
// be with vowels left out: the same skeleton, or for skeletons of four or more
// letters, one more consonant at the end. Every letter of query must appear
// in the word in order, and the word must be longer than query.
func (e *Engine) skeletonTerms(query string, matched map[int32]float64) []skeletonMatch {
	if !dropsVowels(query) {
		return nil
	}
	var stack [64]byte
	skeleton, ok := skeletonOf(stack[:0], query)
	if !ok || len(skeleton) < minSkeleton || len(skeleton) > 40 {
		return nil
	}
	var found []skeletonMatch
	collect := func(target []byte, weight float64) {
		start, _ := slices.BinarySearchFunc(e.skeletons, target, func(w int32, t []byte) int { return compareSkeleton(e.vocabulary[w], t) })
		for i := start; i < len(e.skeletons) && i < start+skeletonScan; i++ {
			w := e.skeletons[i]
			word := e.vocabulary[w]
			if compareSkeleton(word, target) != 0 {
				break
			}
			if len(word) <= len(query) || !isSubsequence(query, word) {
				continue
			}
			if _, ok := matched[w]; ok {
				continue
			}
			found = append(found, skeletonMatch{w, e.firstTitle(w), weight})
		}
	}
	collect(skeleton, skeletonWeight)
	if len(skeleton) >= 4 {
		longer := append(skeleton, 0)
		for c := byte('b'); c <= 'z'; c++ {
			if c == skeleton[len(skeleton)-1] || c == 'e' || c == 'i' || c == 'o' || c == 'u' || c == 'y' {
				continue
			}
			longer[len(longer)-1] = c
			collect(longer, skeletonLongerWeight)
		}
	}
	return smallest(found, skeletonWords, func(a, b skeletonMatch) int {
		if a.weight != b.weight {
			return cmp.Compare(b.weight, a.weight)
		}
		if a.best != b.best {
			return cmp.Compare(a.best, b.best)
		}
		return cmp.Compare(a.word, b.word)
	})
}

// firstTitle is the number of the most popular title using a word (titles
// are numbered most-voted first), or the largest number if none.
func (e *Engine) firstTitle(word int32) int32 {
	best := int32(1<<31 - 1)
	for _, list := range [2][]int32{e.words.get(word), e.alternateWords.get(word)} {
		if len(list) > 0 && list[0] < best {
			best = list[0]
		}
	}
	return best
}
