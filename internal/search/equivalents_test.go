package search

import (
	"slices"
	"strings"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

// Real IMDb titles from the 2026-11-02 comparison with Stremio's default
// search; vote counts are rounded, illustrative values.
func gapEngine() *Engine {
	return New([]catalog.Title{
		{ID: "tt0298203", Name: "8 Mile", Type: "movie", Year: 2002, Votes: 357000},
		{ID: "tt0817230", Name: "Eight Miles High", Type: "movie", Year: 2007, Votes: 1000},
		{ID: "tt0075148", Name: "Rocky", Type: "movie", Year: 1976, Votes: 705000},
		{ID: "tt0079817", Name: "Rocky II", Type: "movie", Year: 1979, Votes: 230000},
		{ID: "tt0089927", Name: "Rocky IV", Type: "movie", Year: 1985, Votes: 250000},
		{ID: "tt0068646", Name: "The Godfather", Type: "movie", Year: 1972, Votes: 2100000},
		{ID: "tt0071562", Name: "The Godfather Part II", Type: "movie", Year: 1974, Votes: 1400000},
		{ID: "tt0088247", Name: "The Terminator", Type: "movie", Year: 1984, Votes: 950000},
		{ID: "tt0103064", Name: "Terminator 2: Judgment Day", Type: "movie", Year: 1991, Votes: 1300000},
		{ID: "tt1232829", Name: "21 Jump Street", Type: "movie", Year: 2012, Votes: 600000},
		{ID: "tt0322259", Name: "2 Fast 2 Furious", Type: "movie", Year: 2003, Votes: 300000},
		{ID: "tt0232500", Name: "The Fast and the Furious", Type: "movie", Year: 2001, Votes: 450000},
		{ID: "tt0031304", Name: "Fast and Furious", Type: "movie", Year: 1939, Votes: 1000},
		{ID: "tt4158110", Name: "Mr. Robot", Type: "series", Year: 2015, Votes: 477000},
		{ID: "tt0055928", Name: "Dr. No", Type: "movie", Year: 1962, Votes: 199000},
		{ID: "tt1211837", Name: "Doctor Strange", Type: "movie", Year: 2016, Votes: 903000},
		{ID: "tt0077473", Name: "Dr. Strange", Type: "movie", Year: 1978, Votes: 2000},
		{ID: "tt0436992", Name: "Doctor Who", Type: "series", Year: 2005, Votes: 264000},
		{ID: "tt1400620", Name: "Dr Hoo", Type: "series", Year: 2009, Votes: 15},
		{ID: "tt0848228", Name: "The Avengers", Type: "movie", Year: 2012, Votes: 1590000},
		{ID: "tt0458339", Name: "Captain America: The First Avenger", Type: "movie", Year: 2011, Votes: 993000},
		{ID: "tt0889671", Name: "Avenger", Type: "movie", Year: 2006, Votes: 1000},
		{ID: "tt1843866", Name: "Captain America: The Winter Soldier", Type: "movie", Year: 2014, Votes: 995000},
		{ID: "tt0069512", Name: "Winter Soldier", Type: "movie", Year: 1972, Votes: 1000},
		{ID: "tt0118715", Name: "The Big Lebowski", Type: "movie", Year: 1998, Votes: 900000},
		{ID: "tt0878804", Name: "The Blind Side", Type: "movie", Year: 2009, Votes: 394000},
		{ID: "tt2195548", Name: "The Blind", Type: "movie", Year: 2023, Votes: 3000},
		{ID: "tt0111161", Name: "The Shawshank Redemption", Type: "movie", Year: 1994, Votes: 3260000},
		{ID: "tt11300932", Name: "Land Shark", Type: "movie", Year: 2020, Votes: 300},
		{ID: "tt0103644", Name: "Alien³", Type: "movie", Year: 1992, Votes: 357000},
		{ID: "tt0078748", Name: "Alien", Type: "movie", Year: 1979, Votes: 1080000},
	})
}

func TestSearchGapsAgainstStremio(t *testing.T) {
	e := gapEngine()
	for _, tc := range []struct{ query, kind, want string }{
		{"eight mile", "movie", "tt0298203"},
		{"8 mile", "movie", "tt0298203"},
		{"rocky 4", "movie", "tt0089927"},
		{"rocky four", "movie", "tt0089927"},
		{"rocky iv", "movie", "tt0089927"},
		{"godfather 2", "movie", "tt0071562"},
		{"terminator 2", "movie", "tt0103064"},
		{"twenty one jump street", "movie", "tt1232829"},
		{"too fast too furious", "movie", "tt0322259"},
		{"fast and furious", "movie", "tt0232500"},
		{"mister robot", "series", "tt4158110"},
		{"doctor no", "movie", "tt0055928"},
		{"dr strange", "movie", "tt1211837"},
		{"doctor strange", "movie", "tt1211837"},
		{"dr who", "series", "tt0436992"},
		{"doctor hoo", "series", "tt0436992"},
		{"avenger", "movie", "tt0848228"},
		{"winter soldier", "movie", "tt1843866"},
		{"shaw shank", "movie", "tt0111161"},
		{"alien 3", "movie", "tt0103644"},
		{"alien", "movie", "tt0078748"},
	} {
		if got := first(t, e, tc.query, tc.kind); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s: wanted %s first, got %v", tc.query, tc.want, got)
		}
	}
}

func TestExactTitleKeepsFirstPlace(t *testing.T) {
	e := gapEngine()
	// The Blind Side has far more votes but is not what "the blind" names.
	if got := first(t, e, "the blind", "movie"); len(got) == 0 || got[0] != "tt2195548" {
		t.Errorf("the blind: %v", got)
	}
	// The obscure exact titles stay in the results.
	for _, tc := range []struct{ query, want string }{{"avenger", "tt0889671"}, {"winter soldier", "tt0069512"}, {"dr strange", "tt0077473"}} {
		if got := first(t, e, tc.query, "movie"); !slices.Contains(got, tc.want) {
			t.Errorf("%s: %s missing from %v", tc.query, tc.want, got)
		}
	}
}

func TestEquivalentNames(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"eight mile", []string{"8 mile", "viii mile"}},
		{"twenty one jump street", []string{"21 jump street"}},
		{"rocky 4", []string{"rocky four", "rocky iv"}},
		{"doctor no", []string{"dr no"}},
		{"star wars", nil},
	} {
		if got := equivalentNames(strings.Fields(tc.query)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %q, want %q", tc.query, got, tc.want)
		}
	}
}
