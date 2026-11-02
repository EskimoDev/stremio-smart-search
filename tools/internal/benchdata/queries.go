package benchdata

import (
	"math/rand/v2"
	"slices"
	"sort"
	"strings"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/textnorm"
)

// Query is a mistyped or partial form of a title, with the title it came from.
type Query struct {
	Kind string // the query form, e.g. "one typo"
	Text string
	Type string // movie or series
	Want string // IMDb identifier of the source title
}

// GenerateQueries derives queries deterministically from the catalogue itself,
// so results are comparable between builds and data sets.
func GenerateQueries(c []catalog.Title, samples int, seed uint64) []Query {
	titles := append([]catalog.Title(nil), c...)
	sort.Slice(titles, func(i, j int) bool {
		if titles[i].Votes != titles[j].Votes {
			return titles[i].Votes > titles[j].Votes
		}
		return titles[i].ID < titles[j].ID
	})
	rng := rand.New(rand.NewPCG(seed, 7))
	var queries []Query
	// Sample titles across popularity: half from the top 5%, half from all.
	for i := 0; i < samples; i++ {
		var t catalog.Title
		if i%2 == 0 {
			t = titles[rng.IntN(max(1, len(titles)/20))]
		} else {
			t = titles[rng.IntN(len(titles))]
		}
		normal := textnorm.Normalize(t.Name)
		words := strings.Fields(normal)
		if len(words) == 0 || !ASCIIWords(words) {
			continue
		}
		add := func(kind, text string) { queries = append(queries, Query{kind, text, t.Type, t.ID}) }
		add("exact", normal)
		longest := 0
		for w := range words {
			if len(words[w]) > len(words[longest]) {
				longest = w
			}
		}
		if len(words[longest]) >= 4 {
			add("one typo", strings.Join(Replaced(words, longest, Typo(rng, words[longest])), " "))
		}
		if len(words[longest]) >= 8 {
			add("two typos", strings.Join(Replaced(words, longest, Typo(rng, Typo(rng, words[longest]))), " "))
		}
		if last := words[len(words)-1]; len(last) >= 6 {
			add("prefix", strings.Join(Replaced(words, len(words)-1, last[:4]), " "))
		}
		if len(words) >= 2 {
			add("joined", strings.Join(words, ""))
		}
	}
	for _, t := range titles {
		if len(t.Aliases) > 0 && len(queries) < samples*8 && rng.IntN(8) == 0 {
			normal := textnorm.Normalize(t.Aliases[0])
			queries = append(queries, Query{"alternative title", normal, t.Type, t.ID})
			if words := strings.Fields(normal); ASCIIWords(words) {
				longest := 0
				for w := range words {
					if len(words[w]) > len(words[longest]) {
						longest = w
					}
				}
				if len(words[longest]) >= 4 {
					queries = append(queries, Query{"alternative title, one typo", strings.Join(Replaced(words, longest, Typo(rng, words[longest])), " "), t.Type, t.ID})
				}
			}
		}
	}
	return queries
}

// ASCIIWords reports whether every word is lower-case ASCII letters and digits.
func ASCIIWords(words []string) bool {
	for _, w := range words {
		for i := 0; i < len(w); i++ {
			if w[i] < '0' || w[i] > 'z' || (w[i] > '9' && w[i] < 'a') {
				return false
			}
		}
	}
	return true
}

// Replaced returns a copy of words with word i replaced.
func Replaced(words []string, i int, word string) []string {
	out := slices.Clone(words)
	out[i] = word
	return out
}

// Typo applies one random deletion, insertion, substitution or adjacent
// transposition to an ASCII word, never at the first letter.
func Typo(rng *rand.Rand, word string) string {
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
