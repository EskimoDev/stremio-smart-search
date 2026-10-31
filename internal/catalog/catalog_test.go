package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIMDbSelectsTitlesNotEpisodesOrAdultContent(t *testing.T) {
	ratings := "tconst\taverageRating\tnumVotes\ntt1\t8.2\t50000\ntt2\t8.1\t40000\ntt3\t9.2\t60000\ntt4\t7.2\t40000\ntt5\t8.0\t12\n"
	basics := "tconst\ttitleType\tprimaryTitle\toriginalTitle\tisAdult\tstartYear\ntt1\tmovie\tA Movie\tA Movie\t0\t2024\ntt2\ttvSeries\tA Series\tOriginal Name\t0\t2020\ntt3\ttvEpisode\tAn Episode\tAn Episode\t0\t2024\ntt4\tmovie\tAdult\tAdult\t1\t2024\ntt5\tmovie\tToo Few Votes\tToo Few Votes\t0\t2024\n"
	c, err := ParseIMDb(context.Background(), strings.NewReader(ratings), strings.NewReader(basics))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Titles) != 2 || c.Titles[0].ID != "tt1" || c.Titles[1].Type != "series" {
		t.Fatalf("incorrect selection: %+v", c.Titles)
	}
}
func TestBadImportLeavesExistingSnapshotUsable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json.gz")
	c := &Catalog{Version: 1, BuiltAt: time.Now(), Titles: []Title{{ID: "tt123", Name: "Working", Type: "movie"}}}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	bad := &Catalog{Version: 1, Titles: []Title{{ID: "not-an-imdb-id", Name: "Bad", Type: "movie"}}}
	if err := Save(path, bad); err == nil {
		t.Fatal("bad import accepted")
	}
	loaded, err := Load(path)
	if err != nil || loaded.Titles[0].Name != "Working" {
		t.Fatalf("lost working snapshot: %+v, %v", loaded, err)
	}
	if err := os.Rename(path, path+".previous"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path + ".previous"); err != nil {
		t.Fatal("compressed recovery snapshot not readable:", err)
	}
}
func TestCancelledPreparationAndMalformedSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ParseIMDb(ctx, strings.NewReader("tconst\taverageRating\tnumVotes\ntt1\t8\t500\n"), strings.NewReader(""))
	if err == nil {
		t.Fatal("cancelled work succeeded")
	}
	_, err = ParseIMDb(context.Background(), strings.NewReader("wrong\theader\n"), strings.NewReader(""))
	if err == nil {
		t.Fatal("schema error ignored")
	}
}
