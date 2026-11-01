// Package benchdata holds the deterministic inputs shared by the developer
// tools: invented title text and search queries derived from a catalogue. It is
// never linked into the shipped helper.
package benchdata

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

var syllables = strings.Fields("ba be bi bo bu ca ce ci co cu da de di do du fa fe fi fo ga ge gi go ha he hi ho ja jo ka ke ki ko la le li lo lu ma me mi mo mu na ne ni no nu pa pe pi po ra re ri ro ru sa se si so su ta te ti to tu va ve vi vo wa we wi ya yo za zo an en in on ar er ir or al el il ol as es is os am em im om ash ell ing ter ton ver str tra pri bla cro dra fla gra")

var common = strings.Fields("the of and a in to love man night last life day dead war world girl house time story black blood dark lost king return city death secret first american big little home wild one two my game new high star great road heart fire red dream island summer christmas devil queen ghost lady boy family murder money power line street woman dragon land hunter end beyond monster sea all no is it for on with you me under out")

var accented = strings.Fields("amélie café señor über mädchen garçon élan déjà naïve façade crème piñata")

// NonLatin invents a title fragment in kana, Cyrillic, Hangul or Han.
func NonLatin(r *rand.Rand) string {
	var b strings.Builder
	ranges := [][2]rune{{0x3041, 0x3093}, {0x0430, 0x044f}, {0xac00, 0xd7a3}, {0x4e00, 0x6fff}}
	rg := ranges[r.IntN(len(ranges))]
	n := 2 + r.IntN(6)
	for i := 0; i < n; i++ {
		b.WriteRune(rg[0] + rune(r.IntN(int(rg[1]-rg[0]))))
	}
	return b.String()
}

// Vocab is a word list drawn from with a Zipf distribution, so a few words are
// shared by many titles, as in real catalogues.
type Vocab struct {
	words []string
	zipf  *rand.Zipf
}

// NewVocab invents size distinct words.
func NewVocab(r *rand.Rand, size int) *Vocab {
	seen := map[string]bool{}
	v := &Vocab{}
	for _, w := range common {
		if !seen[w] {
			seen[w] = true
			v.words = append(v.words, w)
		}
	}
	for len(v.words) < size {
		n := 1 + r.IntN(3)
		if r.IntN(4) == 0 {
			n++
		}
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(syllables[r.IntN(len(syllables))])
		}
		if r.IntN(5) == 0 {
			b.WriteByte("nstrlkdm"[r.IntN(8)])
		}
		w := b.String()
		if !seen[w] {
			seen[w] = true
			v.words = append(v.words, w)
		}
	}
	v.zipf = rand.NewZipf(r, 1.07, 3, uint64(len(v.words)-1))
	return v
}

// Title invents a title of one to seven words.
func (v *Vocab) Title(r *rand.Rand) string {
	n := []int{1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 4, 4, 5, 6, 7}[r.IntN(15)]
	parts := make([]string, 0, n+1)
	for i := 0; i < n; i++ {
		w := v.words[v.zipf.Uint64()]
		if r.IntN(200) == 0 {
			w = accented[r.IntN(len(accented))]
		}
		if r.IntN(3) == 0 || i == 0 {
			w = strings.ToUpper(w[:1]) + w[1:]
		}
		parts = append(parts, w)
	}
	if r.IntN(40) == 0 {
		parts = append(parts, fmt.Sprint(2+r.IntN(4)))
	}
	t := strings.Join(parts, " ")
	if n >= 3 && r.IntN(8) == 0 {
		i := strings.IndexByte(t, ' ')
		t = t[:i] + ":" + t[i:]
	}
	return t
}
