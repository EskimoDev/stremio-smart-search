package search

import (
	"strconv"
	"strings"
)

// Some query words have other spellings that titles use just as often: a
// number written as digits, as a word or in Roman numerals ("eight mile" for
// 8 Mile, "godfather 2" for The Godfather Part II), and a few abbreviations
// ("mister robot" for Mr. Robot). A number also matches its other spellings,
// a little below its own, and the query with numbers and abbreviations
// swapped in is tried as a name.

// equivalentWeight is the weight of another spelling of a query word: below
// the word as typed, so a title spelled the same way wins a tie, and above a
// one-letter completion (.88).
const equivalentWeight = .95

var (
	numberNames = [...]string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
		"eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen", "twenty"}
	romanNumerals = [...]string{"", "i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x",
		"xi", "xii", "xiii", "xiv", "xv", "xvi", "xvii", "xviii", "xix", "xx"}
	// abbreviations are respelled in whole names only ("mister robot" is
	// Mr. Robot), not word by word: as a word, "doctor hoo" would then
	// match an obscure Dr Hoo well enough that Doctor Who is never tried.
	abbreviations = map[string][]string{
		"mister": {"mr"}, "mr": {"mister"},
		"doctor": {"dr"}, "dr": {"doctor"},
		"missus": {"mrs"}, "mrs": {"missus"},
	}
	tensNames = map[string]int{"twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90}
	// equivalentWords maps each number word, numeral and digit string to its
	// other spellings, the likeliest first.
	equivalentWords = func() map[string][]string {
		m := map[string][]string{
			// "too fast too furious" for 2 Fast 2 Furious.
			"too": {"2"},
		}
		for n := 0; n <= 20; n++ {
			digits := strconv.Itoa(n)
			if n == 0 {
				m[digits] = []string{numberNames[n]}
				m[numberNames[n]] = []string{digits}
				continue
			}
			m[digits] = []string{numberNames[n], romanNumerals[n]}
			m[numberNames[n]] = []string{digits, romanNumerals[n]}
			// One-letter Roman numerals (i, v, x) are ordinary words and
			// initials far more often than numbers.
			if len(romanNumerals[n]) > 1 {
				m[romanNumerals[n]] = []string{digits, numberNames[n]}
			}
		}
		for name, n := range tensNames {
			if n > 20 {
				m[name] = []string{strconv.Itoa(n)}
			}
		}
		return m
	}()
)

// numberValue returns the value of a number word from one to nine, or 0.
func unitValue(word string) int {
	for n := 1; n <= 9; n++ {
		if numberNames[n] == word {
			return n
		}
	}
	return 0
}

// equivalentNames returns the query's other exact-name forms, at most three:
// number words such as "twenty one" written as one number, and every word
// with other spellings swapped for its first, then its second, alternative.
func equivalentNames(parts []string) []string {
	var merged []string
	compound := false
	for i := 0; i < len(parts); i++ {
		if tens, ok := tensNames[parts[i]]; ok && i+1 < len(parts) {
			if unit := unitValue(parts[i+1]); unit > 0 {
				merged = append(merged, strconv.Itoa(tens+unit))
				compound = true
				i++
				continue
			}
		}
		merged = append(merged, parts[i])
	}
	var names []string
	add := func(words []string) {
		name := strings.Join(words, " ")
		for _, n := range names {
			if n == name {
				return
			}
		}
		names = append(names, name)
	}
	if compound {
		add(merged)
	}
	for k := 0; k < 2; k++ {
		swapped := make([]string, len(merged))
		changed := false
		for i, word := range merged {
			swapped[i] = word
			alternatives := equivalentWords[word]
			if alternatives == nil {
				alternatives = abbreviations[word]
			}
			if len(alternatives) > 0 {
				swapped[i] = alternatives[min(k, len(alternatives)-1)]
				changed = true
			}
		}
		if changed {
			add(swapped)
		}
	}
	return names
}

// hasEquivalents reports whether any query word has other spellings.
func hasEquivalents(parts []string) bool {
	for _, part := range parts {
		if equivalentWords[part] != nil || abbreviations[part] != nil {
			return true
		}
	}
	return false
}

// withEquivalents adds the other spellings of word to its terms.
func (e *Engine) withEquivalents(terms []termMatch, word string) []termMatch {
	for _, alternative := range equivalentWords[word] {
		w := e.word(alternative)
		if w < 0 {
			continue
		}
		found := false
		for i := range terms {
			if terms[i].word == w {
				terms[i].weight = max(terms[i].weight, equivalentWeight)
				found = true
				break
			}
		}
		if !found {
			terms = append(terms, termMatch{w, equivalentWeight})
		}
	}
	return terms
}

// A title whose name is the query respelled ("8 mile" for "eight mile") or
// with its last word made plural ("the avengers" for "avenger") is a guess at
// what the query names. It scores above any word match, so it comes first
// among them, but far below the exact band; promoteFamous can still lift it
// above an obscure exact title ("dr strange" for Doctor Strange, not the 1978
// Dr. Strange). A guess never lifts an obscure title above a far more popular
// direct match, though: "doctor hoo" means Doctor Who, not a Dr Hoo with 15
// votes.
const (
	scoreRespelled = 120
	// scoreObscureGuess is a guess's score next to a far more popular word
	// match: nothing but its own popularity, so it follows the word matches.
	scoreObscureGuess = 0
)

// respelled returns the titles named by the query respelled or made plural.
func (e *Engine) respelled(q string, parts []string) []int32 {
	var ids []int32
	lookup := func(name string) {
		ids = append(ids, e.names.lookup(name)...)
		ids = append(ids, e.alternate.lookup(name)...)
	}
	if hasEquivalents(parts) {
		for _, name := range equivalentNames(parts) {
			lookup(name)
		}
	}
	// A plural name is often typed singular: "transformer", "the
	// incredible", and "avenger" for The Avengers.
	if last := parts[len(parts)-1]; len(last) >= 4 && isASCII(last) && last[len(last)-1] != 's' {
		lookup(q + "s")
		if parts[0] != "the" {
			lookup("the " + q + "s")
		}
	}
	return ids
}

// scoreGuesses seeds the respelled titles, scored against the word matches.
func (e *Engine) scoreGuesses(hits []hit, guesses []int32, seed func(int32, float64)) {
	best := int32(0)
	for _, h := range hits {
		best = max(best, e.titles[h.id].votes)
	}
	for _, id := range guesses {
		if famousRatio*int64(e.titles[id].votes) < int64(best) {
			seed(id, scoreObscureGuess)
		} else {
			seed(id, scoreRespelled)
		}
	}
}

// A title whose exact name is the query always ranks first, but an obscure
// one should not hide the famous title the query names just as well:
// "avenger" is the name of a 2006 film with a thousand votes, yet means The
// Avengers, and "winter soldier" means Captain America: The Winter Soldier.
// So a match joins the exact band when it is as good as exact (every query
// word matched as typed or as another spelling, or the name respelled), it
// names the whole title or its subtitle word for word, and it has far more
// votes than every exact, joined, alternative-title or acronym match. A title
// that merely contains the query ("the blind" in The Blind Side) is not
// promoted.
const (
	// famousScore is the least match score that counts as nearly exact:
	// every word at least another spelling's weight.
	famousScore = 100 * equivalentWeight
	// famousRatio is how many times the votes of the best special match a
	// title needs, and famousVotes the least it needs at all.
	famousRatio = 20
	famousVotes = 10000
	// famousOffset puts a promoted title just under an exact match with
	// the same votes: its score before promotion is at most about 120.
	famousOffset = scoreExact - 120
)

// promoteFamous moves famous titles the query names into the exact band.
// Scores do not yet include popularity.
func (e *Engine) promoteFamous(hits []hit, parts []string) {
	best := int32(-1)
	for _, h := range hits {
		if h.score >= scoreAcronym {
			best = max(best, e.titles[h.id].votes)
		}
	}
	if best < 0 {
		return
	}
	need := max(int64(famousVotes), famousRatio*int64(best))
	for i := range hits {
		t := &e.titles[hits[i].id]
		if s := hits[i].score; s >= famousScore && s < scoreAcronym && int64(t.votes) >= need && namesTitle(parts, e.nameOf(hits[i].id)) {
			hits[i].score += famousOffset
		}
	}
}

// namesTitle reports whether the query words name the title, or its subtitle
// after the last colon or dash, word for word: each significant word on
// either side is a word on the other, allowing plurals and other spellings.
func namesTitle(parts []string, name string) bool {
	if sameWords(parts, strings.Fields(Normalize(name))) {
		return true
	}
	cut := max(strings.LastIndex(name, ":"), strings.LastIndex(name, " - "))
	return cut >= 0 && sameWords(parts, strings.Fields(Normalize(name[cut+1:])))
}

func sameWords(a, b []string) bool {
	return coveredBy(a, b) && coveredBy(b, a)
}

// coveredBy reports whether every significant word of a matches a word of b.
func coveredBy(a, b []string) bool {
	significant := false
	for _, x := range a {
		if stop[x] {
			continue
		}
		significant = true
		found := false
		for _, y := range b {
			if sameWord(x, y) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return significant
}

func sameWord(x, y string) bool {
	if x == y || x+"s" == y || y+"s" == x {
		return true
	}
	for _, alternative := range equivalentWords[x] {
		if alternative == y {
			return true
		}
	}
	for _, alternative := range abbreviations[x] {
		if alternative == y {
			return true
		}
	}
	return false
}
