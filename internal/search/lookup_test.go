package search

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// The bucketed, filtered word lookup must agree with a plain binary search
// over the whole vocabulary, for words present and absent.
func TestWordLookupMatchesBinarySearch(t *testing.T) {
	vocabulary := []string{"a", "aa", "ab", "abc", "b", "ba", "bé", "zz", "zzz", "é", "éa", "日本"}
	rng := rand.New(rand.NewPCG(3, 4))
	letters := []string{"a", "b", "c", "z", "é", "日"}
	seen := map[string]bool{}
	for _, w := range vocabulary {
		seen[w] = true
	}
	for len(vocabulary) < 3000 {
		w := ""
		for n := 1 + rng.IntN(6); n > 0; n-- {
			w += letters[rng.IntN(len(letters))]
		}
		if !seen[w] {
			seen[w] = true
			vocabulary = append(vocabulary, w)
		}
	}
	slices.Sort(vocabulary)
	e := &Engine{vocabulary: vocabulary, starts: newStarts(vocabulary), filter: newWordFilter(vocabulary)}
	probes := append(slices.Clone(vocabulary), "", "0", "aaaaaaaaaa", "zzzzzzz", "\xff", "\xff\xff")
	for i := 0; i < 5000; i++ {
		w := ""
		for n := 1 + rng.IntN(7); n > 0; n-- {
			w += letters[rng.IntN(len(letters))]
		}
		probes = append(probes, w)
	}
	for _, p := range probes {
		wantIndex, wantFound := slices.BinarySearch(vocabulary, p)
		gotIndex, gotFound := e.find(p)
		if gotIndex != wantIndex || gotFound != wantFound {
			t.Fatalf("find(%q) = %d, %v; want %d, %v", p, gotIndex, gotFound, wantIndex, wantFound)
		}
		want := int32(-1)
		if wantFound {
			want = int32(wantIndex)
		}
		if got := e.word(p); got != want {
			t.Fatalf("word(%q) = %d; want %d", p, got, want)
		}
	}
}

// distance must agree with the plain rune-by-rune definition it replaced.
func TestDistanceMatchesReference(t *testing.T) {
	reference := func(a, b string, limit int) int {
		x, y := []rune(a), []rune(b)
		if abs(len(x)-len(y)) > limit {
			return limit + 1
		}
		pp, p := make([]int, len(y)+1), make([]int, len(y)+1)
		for j := range p {
			p[j] = j
		}
		for i := 1; i <= len(x); i++ {
			c := make([]int, len(y)+1)
			c[0] = i
			rowMin := i
			for j := 1; j <= len(y); j++ {
				cost := 0
				if x[i-1] != y[j-1] {
					cost = 1
				}
				c[j] = min(c[j-1]+1, p[j]+1, p[j-1]+cost)
				if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
					c[j] = min(c[j], pp[j-2]+1)
				}
				rowMin = min(rowMin, c[j])
			}
			if rowMin > limit {
				return limit + 1
			}
			pp, p = p, c
		}
		return p[len(y)]
	}
	rng := rand.New(rand.NewPCG(5, 6))
	letters := []string{"a", "b", "c", "d", "é", "ö"}
	word := func(max int) string {
		w := ""
		for n := rng.IntN(max + 1); n > 0; n-- {
			w += letters[rng.IntN(len(letters))]
		}
		return w
	}
	long := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	pairs := [][2]string{{"", ""}, {"", "abc"}, {long, long[1:]}, {long, long + "é"}, {"thornes", "thrones"}, {"nite", "knight"}}
	for i := 0; i < 20000; i++ {
		pairs = append(pairs, [2]string{word(9), word(9)})
	}
	for _, p := range pairs {
		for limit := 0; limit <= 3; limit++ {
			if got, want := distance(p[0], p[1], limit), reference(p[0], p[1], limit); got != want {
				t.Fatalf("distance(%q, %q, %d) = %d; want %d", p[0], p[1], limit, got, want)
			}
		}
	}
}
