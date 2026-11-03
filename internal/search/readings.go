package search

import (
	"context"
	"math/bits"
	"slices"
	"unicode/utf8"
)

// Several typos in one query often break the rule that a title must match
// every query word: a three-letter word with a typo ("bda", "teh", "rik") has
// no typo matching, and a letter pushed across a space ("gma e of thr" for
// "game of thr") leaves a fragment too short to look up. So when no title
// matches the query as typed about as well as a one-letter typo, a few other
// readings are scored too and each title keeps its best score:
//
//   - one word left out, for a word that matched nothing useful, at a
//     discount (a missing word adds nothing to the score);
//   - a short fragment joined to its neighbour ("gma e" as "gmae", which is
//     one swap from "game"), or two words that form an indexed word when
//     joined ("shaw shank");
//   - two words split again at another point when both halves are indexed
//     words ("gam eof" as "game of"), and a word that is two indexed words
//     run together ("maxfury");
//   - rare three-letter words with a swapped pair or one missing letter
//     (see shortTypos).
//
// Queries that read fine as typed do none of this. Readings reuse the word groups the query already looked up, so the extra
// cost is a few intersections, plus a lookup for each joined or split form.

const (
	// dropPenalty is taken off a title that matches all but one query word,
	// on top of the missing word adding nothing.
	dropPenalty = 10
	// dropFloor is the least average weight the matched words need for a
	// word to be left out: a two-letter typo, not a sound-alike.
	dropFloor = .6
	// splitWeight is the weight of a word found by moving or adding a space:
	// every letter is right, but the spacing was not.
	splitWeight = .8
	// strongWeight is the average word weight at which the query as typed
	// is trusted and no other reading is tried: a one-letter typo.
	strongWeight = .76
)

type hit struct {
	id    int32
	score float64
}

type tally struct {
	id     int32
	count  int32
	seen   uint32 // bit i: matched use[i]
	weight float64
}

// queryWord is one query word in typed order.
type queryWord struct {
	text   string
	lookup bool // a word with its own group (three or more letters, not a stop word or year)
	short  bool // a fragment of one or two letters that is not a stop word
	year   bool
}

// reading is an alternative set of word groups for the query. mult is how
// many query words a group stands for; denominator is how many query words
// the reading accounts for.
type reading struct {
	groups      []int
	mult        []float64
	denominator float64
}

// matchWords returns the word matches' scores, with room for extra more
// hits.
func (e *Engine) matchWords(ctx context.Context, parts []string, typos bool, want uint8, extra int) []hit {
	unique := map[string]int{}
	var work *scratch
	var groups [][]member
	var years []int32
	words := 0
	for _, part := range parts {
		if !stop[part] && utf8.RuneCountInString(part) >= 3 && yearOf(part) == 0 {
			words++
		}
	}
	typed := make([]queryWord, 0, len(parts))
	for _, part := range parts {
		w := queryWord{text: part}
		switch n := utf8.RuneCountInString(part); {
		case stop[part]:
		case n < 3:
			// A fragment left by a misplaced space ("gma e", "squi d"). A
			// two-letter word many titles use ("de", "la", "me") is a word of
			// its own, and joining it to every neighbour costs lookups.
			w.short = n == 1 || e.rareWord(part)
		case yearOf(part) != 0 && words > 0:
			w.year = true
		default:
			w.lookup = true
		}
		typed = append(typed, w)
	}
	scratchArea := func() *scratch {
		if work == nil {
			work = e.scratch()
		}
		return work
	}
	newGroup := func(terms []termMatch, scale float64) int {
		scratchArea()
		gen := work.next()
		var group []member
		if len(groups) < len(work.groups) {
			group = work.groups[len(groups)][:0]
		}
		add := func(id int32, weight float64) {
			if e.kinds[id] != want {
				return
			}
			if work.marks[id].stamp != gen {
				work.marks[id].stamp = gen
				work.marks[id].slot = int32(len(group))
				group = append(group, member{id, weight})
			} else if m := &group[work.marks[id].slot]; weight > m.weight {
				m.weight = weight
			}
		}
		for _, match := range terms {
			for n, id := range e.words.get(match.word) {
				if n >= 3000 || ctx.Err() != nil {
					break
				}
				add(id, match.weight*scale)
			}
			for n, id := range e.alternateWords.get(match.word) {
				if n >= 3000 || ctx.Err() != nil {
					break
				}
				add(id, match.weight*alternateWordWeight*scale)
			}
		}
		groups = append(groups, group)
		return len(groups) - 1
	}
	exactGroup := func(word string) int {
		w := e.word(word)
		if w < 0 {
			return -1
		}
		return newGroup([]termMatch{{w, 1}}, splitWeight)
	}
	defer func() {
		if work != nil {
			for i, group := range groups {
				if i < len(work.groups) {
					work.groups[i] = group
				} else {
					work.groups = append(work.groups, group)
				}
			}
			e.release(work)
		}
	}()

	var base []int // one group per distinct looked-up word, in typed order
	var baseWords []string
	for _, w := range typed {
		if w.year {
			years = append(years, yearOf(w.text))
			continue
		}
		if !w.lookup {
			continue
		}
		if _, ok := unique[w.text]; ok {
			continue
		}
		unique[w.text] = newGroup(e.terms(ctx, scratchArea(), w.text, typos, words == 1), 1)
		base = append(base, unique[w.text])
		baseWords = append(baseWords, w.text)
	}
	var out []hit
	if ctx.Err() != nil {
		return nil
	}
	// index finds a title's place in out once readings other than the query
	// as typed add to it; titles from the query as typed are unique.
	var index map[int32]int
	keep := func(id int32, score float64) {
		if index == nil {
			index = make(map[int32]int, len(out))
			for i, h := range out {
				index[h.id] = i
			}
		}
		if i, ok := index[id]; !ok {
			index[id] = len(out)
			out = append(out, hit{id, score})
		} else if score > out[i].score {
			out[i].score = score
		}
	}
	// score adds the titles a reading matched, with summed weight over
	// matched query words.
	score := func(list []member, matched, denominator, penalty float64, first bool) {
		if first {
			out = make([]hit, 0, len(list)+extra)
		}
		for _, c := range list {
			s := 100*c.weight/denominator - penalty
			// Among equally good word matches, prefer titles the query
			// covers: "the terminater" means The Terminator before
			// Terminator 2: Judgment Day.
			if w := e.titles[c.id].words; w > 0 {
				s += coverageBonus * min(1, matched/float64(w))
			}
			if first {
				out = append(out, hit{c.id, s})
			} else {
				keep(c.id, s)
			}
		}
	}
	denominator := float64(len(base) + len(years))
	// strict scores the query as typed and reports whether some title
	// matches every word about as well as a one-letter typo.
	strict := func(first bool) bool {
		list := e.intersect(work, groups, base, nil)
		strong := false
		for _, c := range list {
			if c.weight >= strongWeight*float64(len(base)) {
				strong = true
				break
			}
		}
		if len(years) > 0 && len(base) == 1 {
			list = slices.Clone(list) // intersect returned the group itself
		}
		for _, y := range years {
			list = e.filterYear(work, list, y)
		}
		score(list, denominator, denominator, 0, first)
		return strong
	}
	// When the query as typed reads fine nothing else is tried, which keeps
	// ordinary searches as fast as before.
	if len(base) > 0 && strict(true) || !typos || ctx.Err() != nil {
		return out
	}
	// Three-letter words get their typo variants only now: most are typed
	// as meant, and their variants are mostly unrelated words.
	retry := false
	for i, w := range baseWords {
		if extra := e.shortTypos(w); len(extra) > 0 {
			base[i] = newGroup(append(e.terms(ctx, scratchArea(), w, typos, words == 1), extra...), 1)
			unique[w] = base[i]
			retry = true
		}
	}
	if retry && strict(false) || ctx.Err() != nil {
		return out
	}
	if len(base) >= 2 {
		// A short word is the likeliest to be a typo beyond reach ("bda",
		// "teh"); a longer one that matches nothing may be what the query is
		// about, so in a two-word query only a short word may be left out.
		droppable := make([]bool, len(base))
		any := false
		for i, w := range baseWords {
			droppable[i] = len(base) >= 3 || utf8.RuneCountInString(w) <= 4
			any = any || droppable[i]
		}
		var dropped []member
		if any {
			dropped = e.dropOne(work, groups, base, droppable)
		}
		for _, y := range years {
			dropped = e.filterYear(work, dropped, y)
		}
		score(dropped, denominator-1, denominator, dropPenalty, false)
	}

	var readings []reading
	without := func(skip ...string) ([]int, []float64) {
		var g []int
		var m []float64
		for _, w := range typed {
			if !w.lookup || contains(skip, w.text) {
				continue
			}
			if i, ok := unique[w.text]; ok && !containsInt(g, i) {
				g = append(g, i)
				m = append(m, 1)
			}
		}
		return g, m
	}
	for i := 0; i+1 < len(typed); i++ {
		a, b := typed[i], typed[i+1]
		if a.year || b.year || !isASCII(a.text) || !isASCII(b.text) {
			continue
		}
		joined := a.text + b.text
		if len(joined) > 40 {
			continue
		}
		// Words joined: a fragment with its neighbour, or two words that
		// make an indexed word.
		group := -1
		fragments := 0.0
		for _, w := range [2]queryWord{a, b} {
			if w.short {
				fragments++
			}
		}
		switch {
		case (a.short && (b.lookup || b.short)) || (b.short && a.lookup):
			group = newGroup(e.quickTerms(ctx, scratchArea(), joined), 1)
		case a.lookup && b.lookup:
			if w := e.word(joined); w >= 0 {
				group = newGroup([]termMatch{{w, 1}}, 1)
			}
		}
		if group >= 0 {
			g, m := without(a.text, b.text)
			readings = append(readings, reading{append(g, group), append(m, 2), denominator + fragments})
		}
		// The space moved: both halves of another split are indexed words.
		if !(a.lookup || b.lookup) || (e.word(a.text) >= 0 && e.word(b.text) >= 0) {
			continue
		}
		for p := 1; p < len(joined); p++ {
			if p == len(a.text) {
				continue
			}
			x, y := joined[:p], joined[p:]
			halves := [2]string{x, y}
			ok, named := true, 0
			for _, h := range halves {
				if stop[h] {
					continue
				}
				if len(h) < 3 || e.word(h) < 0 {
					ok = false
					break
				}
				named++
			}
			if !ok || named == 0 {
				continue
			}
			g, m := without(a.text, b.text)
			for _, h := range halves {
				if !stop[h] {
					if n := exactGroup(h); n >= 0 {
						g = append(g, n)
						m = append(m, 2/float64(named))
					}
				}
			}
			readings = append(readings, reading{g, m, denominator + fragments})
		}
	}
	// A word that is two indexed words run together.
	for _, w := range typed {
		if !w.lookup || !isASCII(w.text) || len(w.text) < 6 || e.word(w.text) >= 0 {
			continue
		}
		for p := 3; p+3 <= len(w.text); p++ {
			x, y := w.text[:p], w.text[p:]
			if stop[x] || stop[y] || e.word(x) < 0 || e.word(y) < 0 {
				continue
			}
			g, m := without(w.text)
			g = append(g, exactGroup(x), exactGroup(y))
			m = append(m, 1, 1)
			readings = append(readings, reading{g, m, denominator + 1})
		}
	}
	for _, r := range readings {
		if ctx.Err() != nil {
			break
		}
		list := e.intersect(work, groups, r.groups, r.mult)
		for _, y := range years {
			list = e.filterYear(work, list, y)
		}
		matched := 0.0
		for _, m := range r.mult {
			matched += m
		}
		score(list, matched+float64(len(years)), r.denominator, 0, false)
	}
	return out
}

// dropOne returns the titles in all but one of the given groups (two or
// more), each with its summed weight, when droppable allows the missing group
// and the matched words are spelled close enough.
func (e *Engine) dropOne(work *scratch, groups [][]member, use []int, droppable []bool) []member {
	gen := work.next()
	tallies := work.tallies[:0]
	for i, g := range use {
		for _, m := range groups[g] {
			if work.marks[m.id].stamp != gen {
				work.marks[m.id].stamp = gen
				work.marks[m.id].slot = int32(len(tallies))
				tallies = append(tallies, tally{m.id, 1, 1 << i, m.weight})
			} else {
				t := &tallies[work.marks[m.id].slot]
				t.count++
				t.seen |= 1 << i
				t.weight += m.weight
			}
		}
	}
	work.tallies = tallies
	var dropped []member
	for _, t := range tallies {
		// Leaving a word out is only worth it when the words that do match
		// are spelled close to the title's: not for sound-alikes of a
		// gibberish query ("zarnet bluve").
		if int(t.count) == len(use)-1 && t.weight >= dropFloor*float64(t.count) && droppable[bits.TrailingZeros32(^t.seen)] {
			dropped = append(dropped, member{t.id, t.weight})
		}
	}
	return dropped
}

// intersect returns the titles in all the given groups, weighted by mult
// (all 1 if nil). The groups are not changed.
func (e *Engine) intersect(work *scratch, groups [][]member, use []int, mult []float64) []member {
	if len(use) == 0 {
		return nil
	}
	if len(use) == 1 && mult == nil {
		return groups[use[0]] // the group itself
	}
	smallest := 0
	for i, g := range use {
		if len(groups[g]) < len(groups[use[smallest]]) {
			smallest = i
		}
	}
	list := make([]member, 0, len(groups[use[smallest]]))
	for _, m := range groups[use[smallest]] {
		list = append(list, member{m.id, m.weight * factor(mult, smallest)})
	}
	for i, g := range use {
		if i == smallest || len(list) == 0 {
			continue
		}
		gen := work.next()
		for j, m := range groups[g] {
			work.marks[m.id].stamp = gen
			work.marks[m.id].slot = int32(j)
		}
		kept := list[:0]
		for _, c := range list {
			if work.marks[c.id].stamp == gen {
				c.weight += groups[g][work.marks[c.id].slot].weight * factor(mult, i)
				kept = append(kept, c)
			}
		}
		list = kept
	}
	return list
}

// rareWord reports whether word is used by fewer than soundCommon titles.
func (e *Engine) rareWord(word string) bool {
	w := e.word(word)
	return w < 0 || e.titleCount(w) < soundCommon
}

func factor(mult []float64, i int) float64 {
	if mult == nil {
		return 1
	}
	return mult[i]
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func containsInt(list []int, n int) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}
