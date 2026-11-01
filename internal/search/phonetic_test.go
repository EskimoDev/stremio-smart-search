package search

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

func TestDoubleMetaphoneReference(t *testing.T) {
	// Published reference codes (Apache Commons Codec tests and Philips'
	// examples), untruncated.
	for word, want := range map[string][2]string{
		"smith": {"SM0", "XMT"}, "schmidt": {"XMT", "SMT"}, "jose": {"HS", "HS"},
		"arnow": {"ARN", "ARNF"}, "thumb": {"0M", "TM"}, "caesar": {"SSR", "SSR"},
		"gallegos": {"KLKS", "KKS"}, "campbell": {"KMPL", "KMPL"}, "xavier": {"SF", "SFR"},
		"knight": {"NT", "NT"}, "edge": {"AJ", "AJ"}, "laugh": {"LF", "LF"},
		"island": {"ALNT", "ALNT"}, "sugar": {"XKR", "SKR"},
	} {
		if p, a := doubleMetaphone(word); p != want[0] || a != want[1] {
			t.Errorf("%s: got %s/%s, want %s/%s", word, p, a, want[0], want[1])
		}
	}
}

func TestSoundAlikeSpellingsShareCodes(t *testing.T) {
	for _, pair := range [][2]string{
		{"incepshun", "inception"}, {"nite", "night"}, {"fite", "fight"}, {"fikshun", "fiction"},
		{"fone", "phone"}, {"skool", "school"}, {"kwiet", "quiet"}, {"sitizen", "citizen"},
		{"yuforia", "euphoria"}, {"gostbusters", "ghostbusters"}, {"fraysure", "frasier"},
	} {
		a, n := soundCodes(new(metaphone), pair[0])
		b, m := soundCodes(new(metaphone), pair[1])
		if n == 0 || m == 0 || !slices.ContainsFunc(a[:n], func(c uint64) bool { return slices.Contains(b[:m], c) }) {
			p1, a1 := doubleMetaphone(pair[0])
			p2, a2 := doubleMetaphone(pair[1])
			t.Errorf("%s (%s/%s) and %s (%s/%s) share no code", pair[0], p1, a1, pair[1], p2, a2)
		}
	}
}

func TestOnlyASCIILetterWordsGetCodes(t *testing.T) {
	for _, word := range []string{"", "2049", "r2d2", "千と千尋", "léon", "ёжик", "a"} {
		if _, n := soundCodes(new(metaphone), word); n != 0 {
			t.Errorf("%q got a sound code", word)
		}
	}
	if packCode([]byte("KN")) == packCode([]byte("KNA")) || packCode([]byte("K?")) != 0 {
		t.Fatal("packed codes collide or accept unknown symbols")
	}
}

func phoneticEngine() *Engine {
	titles := []catalog.Title{
		{ID: "tt1375666", Name: "Inception", Type: "movie", Year: 2010, Votes: 2700000},
		{ID: "tt0477347", Name: "Night at the Museum", Type: "movie", Year: 2006, Votes: 400000},
		{ID: "tt0137523", Name: "Fight Club", Type: "movie", Year: 1999, Votes: 2500000},
		{ID: "tt6965662", Name: "Madam Yankelova's Fine Literature Club", Type: "movie", Year: 2017, Votes: 600},
		{ID: "tt0209144", Name: "Memento", Type: "movie", Year: 2000, Votes: 1400000},
		{ID: "tt3517344", Name: "Momento", Type: "movie", Year: 2013, Votes: 30},
		{ID: "tt0054215", Name: "Psycho", Type: "movie", Year: 1960, Votes: 750000},
		{ID: "tt8772296", Name: "Euphoria", Type: "series", Year: 2019, Votes: 300000},
		{ID: "tt0245429", Name: "Spirited Away", Original: "Sen to Chihiro no kamikakushi", Type: "movie", Year: 2001, Votes: 900000},
	}
	for i := 0; i < 60; i++ { // "bear" is in more than soundCommon titles
		titles = append(titles, catalog.Title{ID: "tt9" + strings.Repeat("0", 5) + string(rune('0'+i/10)) + string(rune('0'+i%10)), Name: "The Bear", Type: "series", Year: 2000 + i%20, Votes: 100 + i})
	}
	titles = append(titles, catalog.Title{ID: "tt7777777", Name: "Bare", Type: "series", Year: 2015, Votes: 5000000})
	return New(titles)
}

func TestSoundAlikesFindPhoneticSpellings(t *testing.T) {
	e := phoneticEngine()
	for _, tc := range []struct{ query, kind, want string }{
		{"incepshun", "movie", "tt1375666"},
		{"nite at the museum", "movie", "tt0477347"},
		{"fite club", "movie", "tt0137523"}, // the spelling match "fine" is far less popular
		{"sykho", "movie", "tt0054215"},
		{"yuforia", "series", "tt8772296"},
	} {
		if got := first(t, e, tc.query, tc.kind); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%q: got %v, want %s first", tc.query, got, tc.want)
		}
	}
}

func TestSoundAlikesStayAFallback(t *testing.T) {
	e := phoneticEngine()
	// An exact title keeps first place over a famous sound-alike.
	if got := first(t, e, "momento", "movie"); len(got) < 2 || got[0] != "tt3517344" || got[1] != "tt0209144" {
		t.Fatalf("momento: %v", got)
	}
	// A spelling match keeps priority over a sound-alike that is only a few
	// times more popular.
	near := New([]catalog.Title{
		{ID: "tt0137523", Name: "Fight Club", Type: "movie", Year: 1999, Votes: 2500000},
		{ID: "tt0000001", Name: "Fine Club", Type: "movie", Year: 2000, Votes: 600000},
	})
	if got := first(t, near, "fite club", "movie"); len(got) != 2 || got[0] != "tt0000001" {
		t.Fatalf("fite club: %v", got)
	}
	// A common indexed word is not respelled: "bear" does not reach "bare".
	// A rare one still is.
	if got := e.soundsLike("bear"); len(got) != 0 {
		t.Fatalf("common word was given sound-alikes: %v", got)
	}
	if got := e.soundsLike("bair"); len(got) == 0 {
		t.Fatal("no sound-alikes for bair")
	}
	// Keyboard noise and non-Latin queries get no sound-alikes.
	for word, want := range map[string]bool{"incepshun": true, "gostbusters": true, "strngth": false, "xqzvwkplm": false, "bcdfg": false, "zxcvbnmae": false} {
		if speakable(word) != want {
			t.Errorf("speakable(%q) = %v", word, !want)
		}
	}
	for _, q := range []string{"xqzvwkplm", "qzxqvvnonexistent", "千と千尋"} {
		if got := first(t, e, q, "movie"); len(got) != 0 {
			t.Fatalf("%q: unexpected %v", q, got)
		}
	}
	// Acronym queries take no typo or sound-alike expansion.
	if got := e.terms(context.Background(), "incepshun", false); len(got) != 0 {
		t.Fatalf("sound-alikes with typos off: %v", got)
	}
}

func TestSoundAlikesAreBounded(t *testing.T) {
	e := fixture(t)
	for _, q := range []string{"nite", "kwiet", "fite", "brekin", "toystory"} {
		sounds := e.soundsLike(q)
		if len(sounds) > soundWords {
			t.Fatalf("%q: %d sound-alikes", q, len(sounds))
		}
		if terms := e.terms(context.Background(), q, true); len(terms) > 32+soundWords {
			t.Fatalf("%q: %d terms", q, len(terms))
		}
	}
}
