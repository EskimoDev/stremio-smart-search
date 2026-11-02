package search

import (
	"cmp"
	"slices"
	"strings"
)

// Double Metaphone (Lawrence Philips, 2000) for single ASCII words, used as a
// last-resort way to find a title word spelled the way it sounds
// ("incepshun", "nite", "fikshun"). It follows the published algorithm and
// Apache Commons Codec's reference implementation, without the four-letter
// truncation: whole-word codes keep sound-alike buckets small. Rules that only
// apply across spaces ("VAN ", "SAN ") cannot fire on one word and are left
// out.
//
// The rules are English-centred with some Romance, Germanic and Slavic
// spellings. They are only applied to words made of ASCII letters; anything
// with digits or non-Latin script gets no code and so never matches
// phonetically.

// maxCode bounds a code's length. Longer words keep their first maxCode
// sounds, which is plenty to tell title words apart.
const maxCode = 12

type metaphone struct {
	word     []byte // uppercase ASCII letters
	primary  []byte
	alternat []byte
	slavo    bool // Slavic or Germanic spelling: contains W, K or CZ
	buf      [64]byte
	p, a     [maxCode + 4]byte
}

// doubleMetaphone returns the primary and alternate codes of a lowercase
// ASCII word, or empty strings if the word is not made of letters a–z only.
func doubleMetaphone(word string) (string, string) {
	var m metaphone
	if !m.run(word) {
		return "", ""
	}
	return string(m.primary), string(m.alternat)
}

// run encodes word into m.primary and m.alternat, which point into m's own
// buffers, without allocating. It reports false for a word that is not made
// of ASCII letters only.
func (m *metaphone) run(word string) bool {
	if len(word) == 0 || len(word) > len(m.buf) {
		return false
	}
	slavo := false
	for i := 0; i < len(word); i++ {
		c := word[i]
		if c < 'a' || c > 'z' {
			return false
		}
		m.buf[i] = c - 'a' + 'A'
		slavo = slavo || c == 'w' || c == 'k' || (c == 'z' && i > 0 && word[i-1] == 'c')
	}
	m.word, m.slavo = m.buf[:len(word)], slavo // "WITZ" contains W
	m.primary, m.alternat = m.p[:0], m.a[:0]
	m.encode()
	m.primary, m.alternat = m.primary[:min(len(m.primary), maxCode)], m.alternat[:min(len(m.alternat), maxCode)]
	return true
}

// has reports whether one of needles, each length bytes long, occurs at
// start. It mirrors contains(value, start, length, ...) in the reference code.
func (m *metaphone) has(start, length int, needles ...string) bool {
	if start < 0 || start+length > len(m.word) {
		return false
	}
	for _, n := range needles {
		if string(m.word[start:start+length]) == n {
			return true
		}
	}
	return false
}

func (m *metaphone) at(i int) byte {
	if i < 0 || i >= len(m.word) {
		return 0
	}
	return m.word[i]
}

func isVowelByte(c byte) bool {
	switch c {
	case 'A', 'E', 'I', 'O', 'U', 'Y':
		return true
	}
	return false
}

func (m *metaphone) add(primary, alternate string) {
	m.primary = append(m.primary, primary...)
	m.alternat = append(m.alternat, alternate...)
}

func (m *metaphone) both(code string) { m.add(code, code) }

func (m *metaphone) full() bool { return len(m.primary) >= maxCode && len(m.alternat) >= maxCode }

func (m *metaphone) encode() {
	i := 0
	last := len(m.word) - 1
	if m.has(0, 2, "GN", "KN", "PN", "WR", "PS") {
		i = 1
	}
	for i <= last && !m.full() {
		c := m.word[i]
		switch c {
		case 'A', 'E', 'I', 'O', 'U', 'Y':
			if i == 0 {
				m.both("A")
			}
			i++
		case 'B':
			m.both("P")
			i += step(m.at(i+1) == 'B')
		case 'C':
			i = m.c(i)
		case 'D':
			switch {
			case m.has(i, 2, "DG"):
				if m.has(i+2, 1, "I", "E", "Y") {
					m.both("J")
					i += 3
				} else {
					m.both("TK")
					i += 2
				}
			case m.has(i, 2, "DT", "DD"):
				m.both("T")
				i += 2
			default:
				m.both("T")
				i++
			}
		case 'F':
			m.both("F")
			i += step(m.at(i+1) == 'F')
		case 'G':
			i = m.g(i)
		case 'H':
			if (i == 0 || isVowelByte(m.at(i-1))) && isVowelByte(m.at(i+1)) {
				m.both("H")
				i += 2
			} else {
				i++
			}
		case 'J':
			i = m.j(i)
		case 'K':
			m.both("K")
			i += step(m.at(i+1) == 'K')
		case 'L':
			if m.at(i+1) == 'L' {
				if m.l0(i) {
					m.add("L", "")
				} else {
					m.both("L")
				}
				i += 2
			} else {
				m.both("L")
				i++
			}
		case 'M':
			m.both("M")
			if m.at(i+1) == 'M' || (m.has(i-1, 3, "UMB") && (i+1 == last || m.has(i+2, 2, "ER"))) {
				i += 2
			} else {
				i++
			}
		case 'N':
			m.both("N")
			i += step(m.at(i+1) == 'N')
		case 'P':
			if m.at(i+1) == 'H' {
				m.both("F")
				i += 2
			} else {
				m.both("P")
				i += step(m.has(i+1, 1, "P", "B"))
			}
		case 'Q':
			m.both("K")
			i += step(m.at(i+1) == 'Q')
		case 'R':
			if i == last && !m.slavo && m.has(i-2, 2, "IE") && !m.has(i-4, 2, "ME", "MA") {
				m.add("", "R")
			} else {
				m.both("R")
			}
			i += step(m.at(i+1) == 'R')
		case 'S':
			i = m.s(i)
		case 'T':
			i = m.t(i)
		case 'V':
			m.both("F")
			i += step(m.at(i+1) == 'V')
		case 'W':
			i = m.w(i)
		case 'X':
			if i == 0 {
				m.both("S")
				i++
				break
			}
			if !(i == last && (m.has(i-3, 3, "IAU", "EAU") || m.has(i-2, 2, "AU", "OU"))) {
				m.both("KS")
			}
			i += step(m.has(i+1, 1, "C", "X"))
		case 'Z':
			if m.at(i+1) == 'H' {
				m.both("J")
				i += 2
				break
			}
			if m.has(i+1, 2, "ZO", "ZI", "ZA") || (m.slavo && i > 0 && m.at(i-1) != 'T') {
				m.add("S", "TS")
			} else {
				m.both("S")
			}
			i += step(m.at(i+1) == 'Z')
		default:
			i++
		}
	}
}

// step is 2 when the next letter repeats the current sound, else 1.
func step(skip bool) int {
	if skip {
		return 2
	}
	return 1
}

func (m *metaphone) c(i int) int {
	switch {
	case m.c0(i):
		m.both("K")
		return i + 2
	case i == 0 && m.has(i, 6, "CAESAR"):
		m.both("S")
		return i + 2
	case m.has(i, 2, "CH"):
		return m.ch(i)
	case m.has(i, 2, "CZ") && !m.has(i-2, 4, "WICZ"):
		m.add("S", "X")
		return i + 2
	case m.has(i+1, 3, "CIA"):
		m.both("X")
		return i + 3
	case m.has(i, 2, "CC") && !(i == 1 && m.at(0) == 'M'):
		if m.has(i+2, 1, "I", "E", "H") && !m.has(i+2, 2, "HU") {
			if (i == 1 && m.at(i-1) == 'A') || m.has(i-1, 5, "UCCEE", "UCCES") {
				m.both("KS")
			} else {
				m.both("X")
			}
			return i + 3
		}
		m.both("K")
		return i + 2
	case m.has(i, 2, "CK", "CG", "CQ"):
		m.both("K")
		return i + 2
	case m.has(i, 2, "CI", "CE", "CY"):
		if m.has(i, 3, "CIO", "CIE", "CIA") {
			m.add("S", "X")
		} else {
			m.both("S")
		}
		return i + 2
	}
	m.both("K")
	if m.has(i+1, 1, "C", "K", "Q") && !m.has(i+1, 2, "CE", "CI") {
		return i + 2
	}
	return i + 1
}

// c0 is the reference "conditionC0": a Germanic "ach" sounds like K.
func (m *metaphone) c0(i int) bool {
	if m.has(i, 4, "CHIA") {
		return true
	}
	if i <= 1 || isVowelByte(m.at(i-2)) || !m.has(i-1, 3, "ACH") {
		return false
	}
	c := m.at(i + 2)
	return (c != 'I' && c != 'E') || m.has(i-2, 6, "BACHER", "MACHER")
}

func (m *metaphone) ch(i int) int {
	if i > 0 && m.has(i, 4, "CHAE") {
		m.add("K", "X")
		return i + 2
	}
	// Greek roots at the start ("chorus", "character") sound like K.
	if i == 0 && (m.has(i+1, 5, "HARAC", "HARIS") || m.has(i+1, 3, "HOR", "HYM", "HIA", "HEM")) && !m.has(0, 5, "CHORE") {
		m.both("K")
		return i + 2
	}
	if m.has(0, 3, "SCH") || m.has(i-2, 6, "ORCHES", "ARCHIT", "ORCHID") || m.has(i+2, 1, "T", "S") ||
		((m.has(i-1, 1, "A", "O", "U", "E") || i == 0) && (m.has(i+2, 1, "L", "R", "N", "M", "B", "H", "F", "V", "W") || i+1 == len(m.word)-1)) {
		m.both("K")
		return i + 2
	}
	if i > 0 {
		if m.has(0, 2, "MC") {
			m.both("K")
		} else {
			m.add("X", "K")
		}
	} else {
		m.both("X")
	}
	return i + 2
}

func (m *metaphone) g(i int) int {
	next := m.at(i + 1)
	switch {
	case next == 'H':
		return m.gh(i)
	case next == 'N':
		switch {
		case i == 1 && isVowelByte(m.at(0)) && !m.slavo:
			m.add("KN", "N")
		case !m.has(i+2, 2, "EY") && m.at(i+1) != 'Y' && !m.slavo:
			m.add("N", "KN")
		default:
			m.both("KN")
		}
		return i + 2
	case m.has(i+1, 2, "LI") && !m.slavo:
		m.add("KL", "L")
		return i + 2
	case i == 0 && (next == 'Y' || m.has(i+1, 2, "ES", "EP", "EB", "EL", "EY", "IB", "IL", "IN", "IE", "EI", "ER")):
		m.add("K", "J")
		return i + 2
	case (m.has(i+1, 2, "ER") || next == 'Y') && !m.has(0, 6, "DANGER", "RANGER", "MANGER") && !m.has(i-1, 1, "E", "I") && !m.has(i-1, 3, "RGY", "OGY"):
		m.add("K", "J")
		return i + 2
	case m.has(i+1, 1, "E", "I", "Y") || m.has(i-1, 4, "AGGI", "OGGI"):
		switch {
		case m.has(0, 3, "SCH") || m.has(i+1, 2, "ET"):
			m.both("K")
		case m.has(i+1, 3, "IER"):
			m.both("J")
		default:
			m.add("J", "K")
		}
		return i + 2
	case next == 'G':
		m.both("K")
		return i + 2
	}
	m.both("K")
	return i + 1
}

func (m *metaphone) gh(i int) int {
	if i > 0 && !isVowelByte(m.at(i-1)) {
		m.both("K")
		return i + 2
	}
	if i == 0 {
		if m.at(i+2) == 'I' {
			m.both("J")
		} else {
			m.both("K")
		}
		return i + 2
	}
	// Silent after b, h or d nearby: "bough", "hugh", "dough".
	if (i > 1 && m.has(i-2, 1, "B", "H", "D")) || (i > 2 && m.has(i-3, 1, "B", "H", "D")) || (i > 3 && m.has(i-4, 1, "B", "H")) {
		return i + 2
	}
	if i > 2 && m.at(i-1) == 'U' && m.has(i-3, 1, "C", "G", "L", "R", "T") {
		m.both("F") // "laugh", "tough"
	} else if i > 0 && m.at(i-1) != 'I' {
		m.both("K")
	}
	return i + 2
}

func (m *metaphone) j(i int) int {
	if m.has(i, 4, "JOSE") {
		if (i == 0 && m.at(i+4) == 0) || len(m.word) == 4 {
			m.both("H")
		} else {
			m.add("J", "H")
		}
		return i + 1
	}
	switch {
	case i == 0:
		m.add("J", "A")
	case isVowelByte(m.at(i-1)) && !m.slavo && (m.at(i+1) == 'A' || m.at(i+1) == 'O'):
		m.add("J", "H")
	case i == len(m.word)-1:
		m.add("J", "")
	case !m.has(i+1, 1, "L", "T", "K", "S", "N", "M", "B", "Z") && !m.has(i-1, 1, "S", "K", "L"):
		m.both("J")
	}
	return i + step(m.at(i+1) == 'J')
}

// l0 is the reference "conditionL0": Spanish "-illo", "-illa", "-alle".
func (m *metaphone) l0(i int) bool {
	n := len(m.word)
	if i == n-3 && m.has(i-1, 4, "ILLO", "ILLA", "ALLE") {
		return true
	}
	return (m.has(n-2, 2, "AS", "OS") || m.has(n-1, 1, "A", "O")) && m.has(i-1, 4, "ALLE")
}

func (m *metaphone) s(i int) int {
	last := len(m.word) - 1
	switch {
	case m.has(i-1, 3, "ISL", "YSL"):
		return i + 1 // silent: "island", "carlysle"
	case i == 0 && m.has(i, 5, "SUGAR"):
		m.add("X", "S")
		return i + 1
	case m.has(i, 2, "SH"):
		if m.has(i+1, 4, "HEIM", "HOEK", "HOLM", "HOLZ") {
			m.both("S")
		} else {
			m.both("X")
		}
		return i + 2
	case m.has(i, 3, "SIO", "SIA") || m.has(i, 4, "SIAN"):
		if m.slavo {
			m.both("S")
		} else {
			m.add("S", "X")
		}
		return i + 3
	case (i == 0 && m.has(i+1, 1, "M", "N", "L", "W")) || m.has(i+1, 1, "Z"):
		m.add("S", "X")
		return i + step(m.has(i+1, 1, "Z"))
	case m.has(i, 2, "SC"):
		if m.at(i+2) == 'H' {
			if m.has(i+3, 2, "OO", "ER", "EN", "UY", "ED", "EM") {
				if m.has(i+3, 2, "ER", "EN") {
					m.add("X", "SK")
				} else {
					m.both("SK")
				}
			} else if i == 0 && !isVowelByte(m.at(3)) && m.at(3) != 'W' {
				m.add("X", "S")
			} else {
				m.both("X")
			}
		} else if m.has(i+2, 1, "I", "E", "Y") {
			m.both("S")
		} else {
			m.both("SK")
		}
		return i + 3
	}
	if i == last && m.has(i-2, 2, "AI", "OI") {
		m.add("", "S") // French: "artois"
	} else {
		m.both("S")
	}
	return i + step(m.has(i+1, 1, "S", "Z"))
}

func (m *metaphone) t(i int) int {
	switch {
	case m.has(i, 4, "TION"):
		m.both("X")
		return i + 3
	case m.has(i, 3, "TIA", "TCH"):
		m.both("X")
		return i + 3
	case m.has(i, 2, "TH") || m.has(i, 3, "TTH"):
		if m.has(i+2, 2, "OM", "AM") || m.has(0, 3, "SCH") {
			m.both("T")
		} else {
			m.add("0", "T") // "0" is the reference code for "th"
		}
		return i + 2
	}
	m.both("T")
	return i + step(m.has(i+1, 1, "T", "D"))
}

func (m *metaphone) w(i int) int {
	if m.has(i, 2, "WR") {
		m.both("R")
		return i + 2
	}
	last := len(m.word) - 1
	switch {
	case i == 0 && (isVowelByte(m.at(i+1)) || m.has(i, 2, "WH")):
		if isVowelByte(m.at(i + 1)) {
			m.add("A", "F")
		} else {
			m.both("A")
		}
		return i + 1
	case (i == last && isVowelByte(m.at(i-1))) || m.has(i-1, 5, "EWSKI", "EWSKY", "OWSKI", "OWSKY") || m.has(0, 3, "SCH"):
		m.add("", "F")
		return i + 1
	case m.has(i, 4, "WICZ", "WITZ"):
		m.add("TS", "FX")
		return i + 4
	}
	return i + 1
}

// Sound-alike matching is a fallback for words edit distance cannot reach:
// "incepshun" is four edits from "inception", "nite" three from "night". It
// is deliberately weak and bounded.
const (
	// soundWeight ranks a sound-alike word below every spelling match (one
	// edit .76, two edits .6), so it rarely outranks a title the query spells
	// almost right: generated typo queries keep their accuracy. Popularity
	// still lets "fite club" find Fight Club.
	soundWeight = .4
	// nearWeight is for a word whose code is one likely sound away from the
	// query's (see nearSounds): below an exact sound-alike.
	nearWeight = .3
	// soundWords is how many sound-alike words one query word may expand to.
	// They are added beyond the usual term limit, so typo matches cannot crowd
	// them out.
	soundWords = 8
	// A query word that is itself in this many titles is taken as meant: "the
	// bear" is not respelled as "bare" or "beer". Rarer indexed words such as
	// "nite" still get sound-alikes.
	soundCommon = 50
	// minSoundWord is the shortest query word given sound-alikes.
	minSoundWord = 4
	// soundVotes is how many votes a sound-alike's most popular title needs
	// unless the word is also within a third of the query's letters by edit
	// distance. People respell titles they have heard of; a random string's
	// sound matching an obscure title's word was most of what the fallback
	// returned for nonsense queries.
	soundVotes = 20000
)

// soundIndex files vocabulary words under their Double Metaphone codes. Codes
// are packed four bits per symbol into a uint64 and kept once each, sorted,
// with the words of each code in a posting list.
type soundIndex struct {
	codes []uint64
	words postingList
}

// packCode packs a code of at most maxCode symbols; 0 means no code.
func packCode(code []byte) uint64 {
	var packed uint64
	for i := 0; i < len(code); i++ {
		var symbol uint64
		switch code[i] {
		case 'A':
			symbol = 1
		case 'F':
			symbol = 2
		case 'H':
			symbol = 3
		case 'J':
			symbol = 4
		case 'K':
			symbol = 5
		case 'L':
			symbol = 6
		case 'M':
			symbol = 7
		case 'N':
			symbol = 8
		case 'P':
			symbol = 9
		case 'R':
			symbol = 10
		case 'S':
			symbol = 11
		case 'T':
			symbol = 12
		case 'X':
			symbol = 13
		case '0':
			symbol = 14
		default:
			return 0
		}
		packed = packed<<4 | symbol
	}
	// Mark the length so that "KN" and "KNA" never collide.
	return packed | uint64(len(code))<<56
}

// soundCodes returns a word's distinct usable codes: at least two sounds, so
// that one consonant ("N") is never a bucket.
func soundCodes(m *metaphone, word string) (codes [2]uint64, n int) {
	if !m.run(word) {
		return codes, 0
	}
	for _, code := range [2][]byte{m.primary, m.alternat} {
		if len(code) < 2 {
			continue
		}
		if packed := packCode(code); packed != 0 && (n == 0 || codes[0] != packed) {
			codes[n] = packed
			n++
		}
	}
	return codes, n
}

func newSoundIndex(vocabulary []string) soundIndex {
	type pair struct {
		code uint64
		word int32
	}
	var pairs []pair
	m := new(metaphone)
	for position, word := range vocabulary {
		if len(word) < 3 {
			continue
		}
		codes, n := soundCodes(m, word)
		for _, code := range codes[:n] {
			pairs = append(pairs, pair{code, int32(position)})
		}
	}
	slices.SortFunc(pairs, func(a, b pair) int {
		if a.code != b.code {
			return cmp.Compare(a.code, b.code)
		}
		return cmp.Compare(a.word, b.word)
	})
	var s soundIndex
	keys := make([]int32, len(pairs))
	words := make([]int32, len(pairs))
	for i, p := range pairs {
		if len(s.codes) == 0 || s.codes[len(s.codes)-1] != p.code {
			s.codes = append(s.codes, p.code)
		}
		keys[i] = int32(len(s.codes) - 1)
		words[i] = p.word
	}
	s.codes = slices.Clip(s.codes)
	s.words = buildPostings(keys, words, len(s.codes))
	return s
}

func (s *soundIndex) lookup(code uint64) []int32 {
	i, found := slices.BinarySearch(s.codes, code)
	if !found {
		return nil
	}
	return s.words.get(int32(i))
}

// speakable reports whether a query word could be a spelling of how
// something sounds: it has a vowel and no run of more than three consonants.
// Keyboard noise ("xqzvwkplm") gets no sound-alikes.
func speakable(word string) bool {
	run, vowels := 0, 0
	for i := 0; i < len(word); i++ {
		switch word[i] {
		case 'a', 'e', 'i', 'o', 'u', 'y':
			run = 0
			vowels++
		default:
			if run++; run > 3 {
				return false
			}
		}
	}
	return vowels > 0
}

// titleCount is how many titles use a word, canonical or alternative.
func (e *Engine) titleCount(word int32) int {
	return len(e.words.get(word)) + len(e.alternateWords.get(word))
}

// bestVotes is the vote count of the most popular title using a word.
// Titles are numbered most-voted first, so that is each list's first title.
func (e *Engine) bestVotes(word int32) int32 {
	best := int32(-1)
	for _, list := range [2][]int32{e.words.get(word), e.alternateWords.get(word)} {
		if len(list) > 0 && (best < 0 || list[0] < best) {
			best = list[0]
		}
	}
	if best < 0 {
		return 0
	}
	return e.titles[best].votes
}

// soundsLike returns up to soundWords vocabulary words that sound like query,
// most used first, excluding query itself. Candidates must be within three
// letters of its length and within about half its letters of it by edit
// distance, so a shared consonant skeleton alone ("dorman", "truman") is
// not enough.
func (e *Engine) soundsLike(query string, alone bool) []int32 {
	self, ok := e.soundQuery(query)
	if !ok {
		return nil
	}
	codes, n := soundCodes(new(metaphone), query) // empty unless query is ASCII letters only
	var candidates []soundCandidate
	for _, code := range codes[:n] {
		candidates = e.soundCandidates(candidates, code, query, self, nil)
	}
	return e.closeSounds(candidates, query, len(query)/2+1, alone)
}

// nearSounds returns up to soundWords words, other than exact, whose code is
// one likely sound away from query's: a dropped final K ("brekin" PRKN,
// "breaking" PRKNK) or L ("wocking" AKNK, "walking" ALKNK), or X and T, S
// or K heard as each other ("fucher" FXR, "future" FTR; "pinokkio" PNK,
// "pinocchio" PNX). Only
// codes of three or more sounds are varied, and candidates must be closer
// in spelling than plain sound-alikes (half the query's letters), which
// keeps random pronounceable strings from gaining matches. Dropping a
// sound is not tried: shorter codes match too many common short words.
func (e *Engine) nearSounds(query string, exact []int32, alone bool) []int32 {
	self, ok := e.soundQuery(query)
	if !ok {
		return nil
	}
	m := new(metaphone)
	if !m.run(query) {
		return nil
	}
	own := [2]uint64{packCode(m.primary), packCode(m.alternat)}
	var candidates []soundCandidate
	var tried []uint64
	var buf [maxCode + 1]byte
	try := func(code []byte) {
		packed := packCode(code)
		if packed == 0 || packed == own[0] || packed == own[1] || slices.Contains(tried, packed) {
			return
		}
		tried = append(tried, packed)
		candidates = e.soundCandidates(candidates, packed, query, self, exact)
	}
	for _, code := range [2][]byte{m.primary, m.alternat} {
		if len(code) < 3 {
			continue
		}
		for i := 0; i <= len(code) && len(code) < maxCode; i++ {
			for _, sound := range [...]byte{'K', 'L'} {
				try(append(append(append(buf[:0], code[:i]...), sound), code[i:]...))
			}
		}
		for i, c := range code {
			if c == 'X' {
				for _, sound := range [...]byte{'T', 'S', 'K'} {
					v := append(buf[:0], code...)
					v[i] = sound
					try(v)
				}
			} else if c == 'T' || c == 'S' || c == 'K' {
				v := append(buf[:0], code...)
				v[i] = 'X'
				try(v)
			}
		}
	}
	return e.closeSounds(candidates, query, len(query)/2, alone)
}

// soundQuery reports whether query may be given sound-alikes, and its own
// word number (or -1): it must be long and speakable, and not a common word.
func (e *Engine) soundQuery(query string) (int32, bool) {
	if len(query) < minSoundWord || !speakable(query) {
		return -1, false
	}
	self := e.word(query)
	return self, self < 0 || e.titleCount(self) < soundCommon
}

type soundCandidate struct {
	word  int32
	count int
}

// soundCandidates appends the words filed under code within three letters of
// query's length, other than self and the words in exclude.
func (e *Engine) soundCandidates(candidates []soundCandidate, code uint64, query string, self int32, exclude []int32) []soundCandidate {
	for _, w := range e.sounds.lookup(code) {
		if w != self && abs(int(e.runeLengths[w])-len(query)) <= 3 && !slices.Contains(exclude, w) {
			candidates = append(candidates, soundCandidate{w, e.titleCount(w)})
		}
	}
	return candidates
}

// closeSounds returns up to soundWords candidates within limit edits of
// query, most used first. A word that is the whole query has no other word
// to confirm it, so from five letters on it gets one edit less. A candidate
// more than a third of the query's letters away must belong to a popular
// title (see soundVotes).
func (e *Engine) closeSounds(candidates []soundCandidate, query string, limit int, alone bool) []int32 {
	if alone && len(query) >= 5 {
		limit--
	}
	slices.SortFunc(candidates, func(a, b soundCandidate) int {
		if a.count != b.count {
			return cmp.Compare(b.count, a.count)
		}
		return cmp.Compare(a.word, b.word)
	})
	candidates = slices.Compact(candidates) // a word filed under two codes
	var result []int32
	for _, c := range candidates {
		if len(result) == soundWords {
			break
		}
		if d := soundDistance(query, e.vocabulary[c.word], limit); d <= limit && (3*d <= len(query) || e.bestVotes(c.word) >= soundVotes) {
			result = append(result, c.word)
		}
	}
	return result
}

// soundDistance is the edit distance between two words that sound alike,
// counting letters that are written but not spoken as absent: "nite" is four
// edits from "knight" but one from its spoken form "nit".
func soundDistance(query, word string, limit int) int {
	d := distance(query, word, limit)
	if d > 1 {
		if q, w := unspoken(query), unspoken(word); q != query || w != word {
			d = min(d, distance(q, w, limit))
		}
	}
	return d
}

// unspoken drops letters English usually leaves silent: the K, G and W of
// an initial "kn", "gn" and "wr", "gh" before a consonant or at the end
// ("night", "though"), and "ph" read as F.
func unspoken(word string) string {
	if len(word) > 2 && (strings.HasPrefix(word, "kn") || strings.HasPrefix(word, "gn") || strings.HasPrefix(word, "wr")) {
		word = word[1:]
	}
	if !strings.Contains(word, "gh") && !strings.Contains(word, "ph") {
		return word
	}
	var b strings.Builder
	for i := 0; i < len(word); i++ {
		if word[i] == 'g' && i+1 < len(word) && word[i+1] == 'h' && i > 0 && (i+2 == len(word) || strings.IndexByte("aeiouy", word[i+2]) < 0) {
			i++
			continue
		}
		if word[i] == 'p' && i+1 < len(word) && word[i+1] == 'h' {
			b.WriteByte('f')
			i++
			continue
		}
		b.WriteByte(word[i])
	}
	return b.String()
}
