package search

import (
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// Real IMDb titles with rounded, illustrative vote counts.
func soundEngine() *Engine {
	return New([]catalog.Title{
		{ID: "tt8772296", Name: "Euphoria", Type: "series", Year: 2019, Votes: 330000},
		{ID: "tt16366618", Name: "Eufòria", Type: "series", Year: 2022, Votes: 300},
		{ID: "tt0034583", Name: "Casablanca", Type: "movie", Year: 1942, Votes: 660000},
		{ID: "tt10497826", Name: "Kasablanka", Type: "movie", Year: 2019, Votes: 400},
		{ID: "tt0183649", Name: "Phone Booth", Type: "movie", Year: 2002, Votes: 300000},
		{ID: "tt0087332", Name: "Ghostbusters", Type: "movie", Year: 1984, Votes: 490000},
		{ID: "tt0000001", Name: "Green Ghost and the Masters of the Stone", Type: "movie", Year: 2021, Votes: 900},
		{ID: "tt0000003", Name: "Holy", Type: "movie", Year: 2018, Votes: 300},
		{ID: "tt0000002", Name: "Holly", Type: "movie", Year: 2023, Votes: 200000},
		{ID: "tt2467372", Name: "Brooklyn Nine-Nine", Type: "series", Year: 2013, Votes: 430000},
		{ID: "tt12343534", Name: "Jujutsu Kaisen", Type: "series", Year: 2020, Votes: 225000},
		{ID: "tt1355642", Name: "Fullmetal Alchemist: Brotherhood", Type: "series", Year: 2009, Votes: 260000},
		{ID: "tt0203259", Name: "Law & Order: Special Victims Unit", Type: "series", Year: 1999, Votes: 150000},
	})
}

func TestFamousTitleRespelledBeatsObscureExactName(t *testing.T) {
	e := soundEngine()
	for _, tc := range []struct{ query, kind, want, second string }{
		{"euforia", "series", "tt8772296", "tt16366618"},
		{"kasablanka", "movie", "tt0034583", "tt10497826"},
		{"fone booth", "movie", "tt0183649", ""},
	} {
		got := first(t, e, tc.query, tc.kind)
		if len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s: wanted %s first, got %v", tc.query, tc.want, got)
		}
		if tc.second != "" && (len(got) < 2 || got[1] != tc.second) {
			t.Errorf("%s: wanted the exact name %s second, got %v", tc.query, tc.second, got)
		}
	}
	// Respelling is narrow: a different vowel or a doubled letter is another
	// word, so an exact name stays first.
	if got := first(t, e, "holy", "movie"); len(got) == 0 || got[0] != "tt0000003" {
		t.Errorf("holy: Holly promoted over the exact name: %v", got)
	}
}

func spoken(word string) string { return string(spokenInto(nil, []byte(word))) }

func TestSpoken(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"euforia", "euphoria"}, {"kasablanka", "casablanca"}, {"fone", "phone"}, {"sherlok", "sherlock"},
	} {
		if spoken(tc.a) != spoken(tc.b) {
			t.Errorf("%s and %s should sound alike: %q, %q", tc.a, tc.b, spoken(tc.a), spoken(tc.b))
		}
	}
	for _, tc := range []struct{ a, b string }{
		{"holly", "holy"}, {"bare", "beer"}, {"rose", "roze"}, {"since", "sinke"},
	} {
		if spoken(tc.a) == spoken(tc.b) {
			t.Errorf("%s and %s should stay different", tc.a, tc.b)
		}
	}
}

func TestJoinedWordsSoundLikeOneWord(t *testing.T) {
	e := soundEngine()
	for _, q := range []string{"gost busters", "goast busters", "ghost busters"} {
		if got := first(t, e, q, "movie"); len(got) == 0 || got[0] != "tt0087332" {
			t.Errorf("%s: wanted Ghostbusters first, got %v", q, got)
		}
	}
}

func TestFanNicknames(t *testing.T) {
	e := soundEngine()
	for _, tc := range []struct{ query, want string }{
		{"b99", "tt2467372"}, {"jjk", "tt12343534"}, {"fmab", "tt1355642"}, {"fma", "tt1355642"}, {"svu", "tt0203259"},
	} {
		if got := first(t, e, tc.query, "series"); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s: wanted %s first, got %v", tc.query, tc.want, got)
		}
	}
	for name := range nicknames {
		if name != Normalize(name) {
			t.Errorf("nickname key %q is not normalized", name)
		}
	}
}
