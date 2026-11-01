package catalog

import (
	"context"
	"strings"
	"testing"
)

func TestProfilesSelectByVoteThreshold(t *testing.T) {
	ratings := "tconst\taverageRating\tnumVotes\ntt1\t8\t250\ntt2\t7\t25\ntt3\t6\t5\ntt4\t5\t4\n"
	basics := "tconst\ttitleType\tprimaryTitle\toriginalTitle\tisAdult\tstartYear\ntt1\tmovie\tPopular\tPopular\t0\t2024\ntt2\ttvSeries\tLess Popular\tLess Popular\t0\t2024\ntt3\tmovie\tNew Title\tNew Title\t0\t2026\ntt4\tmovie\tBelow Threshold\tBelow Threshold\t0\t2026\n"
	for i, profile := range Profiles() {
		t.Run(profile.ID, func(t *testing.T) {
			c, err := ParseIMDbProfile(context.Background(), strings.NewReader(ratings), strings.NewReader(basics), profile)
			if err != nil {
				t.Fatal(err)
			}
			if c.Profile != profile.ID || len(c.Titles) != i+1 {
				t.Fatalf("wrong selection for %s: %+v", profile.ID, c)
			}
			if profile.Movies+profile.Series > MaxTitles {
				t.Fatal("profile exceeds loader bound")
			}
		})
	}
	if _, err := ParseIMDbProfile(context.Background(), strings.NewReader(ratings), strings.NewReader(basics), Profile{ID: "arbitrary"}); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
func TestProfileCapacityKeepsPopularTitlesPerType(t *testing.T) {
	basics := "tconst\ttitleType\tprimaryTitle\toriginalTitle\tisAdult\tstartYear\ntt3\tmovie\tThird\tThird\t0\t2024\ntt2\tmovie\tSecond\tSecond\t0\t2024\ntt1\tmovie\tFirst\tFirst\t0\t2024\ntt5\ttvSeries\tLower\tLower\t0\t2024\ntt4\ttvSeries\tHigher\tHigher\t0\t2024\n"
	ratings := map[uint32]rating{1: {votes: 500}, 2: {votes: 500}, 3: {votes: 250}, 4: {votes: 500}, 5: {votes: 250}}
	// Small internal capacities exercise the same selection used by real profiles.
	c, err := parseBasics(context.Background(), strings.NewReader(basics), ratings, Profile{ID: "compact", Movies: 1, Series: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Titles) != 2 || c.Titles[0].ID != "tt1" || c.Titles[1].ID != "tt4" {
		t.Fatalf("incorrect bounded selection: %+v", c.Titles)
	}
}
