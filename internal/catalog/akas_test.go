package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixtureFile(t testing.TB, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "testdata", "aliases", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// aliasFixture builds the Standard catalogue from the handwritten fixture and
// applies alternative titles for options.
func aliasFixture(t testing.TB, options AliasOptions) *Catalog {
	t.Helper()
	profile, _ := ProfileByID("standard")
	c, err := ParseIMDbProfile(context.Background(), fixtureFile(t, "title.ratings.tsv"), fixtureFile(t, "title.basics.tsv"), profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := ParseIMDbAliases(context.Background(), fixtureFile(t, "title.akas.tsv"), c, options); err != nil {
		t.Fatal(err)
	}
	return c
}

func aliasesByID(c *Catalog) map[string][]string {
	out := map[string][]string{}
	for _, t := range c.Titles {
		if len(t.Aliases) > 0 {
			out[t.ID] = t.Aliases
		}
	}
	return out
}

func TestFrenchAliasesAreFilteredDeduplicatedAndRanked(t *testing.T) {
	c := aliasFixture(t, AliasOptions{Language: "fr"})
	want := map[string][]string{
		"tt2294629": {"La Reine des neiges"},           // CA/BE duplicates, literal, 3-D, working, English-language and shared-region rows dropped
		"tt2948356": {"Zootopie", "Zootopie, le film"}, // region-less row accepted through its explicit language; display title first
		"tt9999902": {"La Reine des neiges"},           // identical alias on a different title is kept
		"tt1049413": {"Là-haut"},                       // accents preserved in storage
		"tt0245429": {"Le Voyage de Chihiro"},
		"tt0944947": {"Le Trône de fer"}, // series aliases too; tv duplicate dropped
	}
	if got := aliasesByID(c); !reflect.DeepEqual(got, want) {
		t.Fatalf("French aliases:\n got %v\nwant %v", got, want)
	}
	if c.AliasLanguage != "fr" || c.AliasRegion != "" || c.AliasCount() != 7 {
		t.Fatalf("selection not recorded: %q %q %d", c.AliasLanguage, c.AliasRegion, c.AliasCount())
	}
	for _, title := range c.Titles {
		if title.ID == "tt0959621" || title.ID == "tt0000001" {
			t.Fatal("episode or unrated title entered the catalogue")
		}
	}
}

func TestRegionalPreferenceNarrowsSpanishAliases(t *testing.T) {
	spain := aliasesByID(aliasFixture(t, AliasOptions{Language: "es", Region: "ES"}))
	mexico := aliasesByID(aliasFixture(t, AliasOptions{Language: "es", Region: "MX"}))
	all := aliasesByID(aliasFixture(t, AliasOptions{Language: "es"}))
	if !reflect.DeepEqual(spain["tt2948356"], []string{"Zootrópolis"}) || !reflect.DeepEqual(spain["tt2294629"], []string{"Frozen: El reino del hielo"}) {
		t.Fatalf("Spain: %v", spain)
	}
	if mexico["tt2948356"] != nil || !reflect.DeepEqual(mexico["tt2294629"], []string{"Frozen: Una aventura congelada"}) {
		t.Fatalf("Mexico (Zootopia's Mexican title equals the canonical one): %v", mexico)
	}
	if len(all["tt2294629"]) != 2 || spain["tt1049413"] != nil || all["tt1049413"] == nil {
		t.Fatalf("all Spanish regions: %v (short alias 'Lo' must be dropped)", all)
	}
}

func TestNonLatinAliases(t *testing.T) {
	japanese := aliasesByID(aliasFixture(t, AliasOptions{Language: "ja"}))
	if !reflect.DeepEqual(japanese["tt0245429"], []string{"千と千尋の神隠し"}) || !reflect.DeepEqual(japanese["tt2294629"], []string{"アナと雪の女王"}) {
		t.Fatalf("Japanese: %v", japanese)
	}
	korean := aliasesByID(aliasFixture(t, AliasOptions{Language: "ko"}))
	if !reflect.DeepEqual(korean["tt6751668"], []string{"기생충"}) {
		t.Fatalf("Korean: %v", korean)
	}
	russian := aliasesByID(aliasFixture(t, AliasOptions{Language: "ru"}))
	if !reflect.DeepEqual(russian["tt2294629"], []string{"Холодное сердце"}) {
		t.Fatalf("Russian: %v", russian)
	}
}

func TestEnglishAliasesAndExplicitLanguageWins(t *testing.T) {
	english := aliasesByID(aliasFixture(t, AliasOptions{Language: "en"}))
	// A row in region FR explicitly tagged en counts as English, not French.
	if !reflect.DeepEqual(english["tt2948356"], []string{"Zootropolis"}) || !reflect.DeepEqual(english["tt2294629"], []string{"Frozen (English version)"}) {
		t.Fatalf("English: %v", english)
	}
	// Shared multilingual regions without a language are never guessed.
	german := aliasesByID(aliasFixture(t, AliasOptions{Language: "de"}))
	for _, aliases := range german {
		for _, alias := range aliases {
			if strings.Contains(alias, "glaces") {
				t.Fatal("Swiss row without a language was attributed to German")
			}
		}
	}
}

func TestAliasOptionsValidation(t *testing.T) {
	for _, bad := range []AliasOptions{{Language: "xx"}, {Region: "FR"}, {Language: "fr", Region: "ES"}, {Language: "it", Region: "IT"}} {
		if bad.Validate() == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	c := aliasFixture(t, AliasOptions{})
	if c.AliasCount() != 0 || c.AliasLanguage != "" {
		t.Fatal("disabled options changed the catalogue")
	}
	for _, language := range Languages() {
		if (AliasOptions{Language: language.ID}).Validate() != nil {
			t.Fatalf("listed language %s rejected", language.ID)
		}
		for _, region := range language.Choices {
			if region.Name == "" || (AliasOptions{Language: language.ID, Region: region.ID}).Validate() != nil {
				t.Fatalf("listed region %s/%s unusable", language.ID, region.ID)
			}
		}
	}
}

func TestAliasParsingFailuresLeaveCatalogueUnchanged(t *testing.T) {
	c := aliasFixture(t, AliasOptions{})
	before := fmt.Sprint(c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ParseIMDbAliases(ctx, fixtureFile(t, "title.akas.tsv"), c, AliasOptions{Language: "fr"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	for _, source := range []string{"", "titleId\ttitle\n", "wrong\theader\n"} {
		if ParseIMDbAliases(context.Background(), strings.NewReader(source), c, AliasOptions{Language: "fr"}) == nil {
			t.Fatalf("bad source accepted: %q", source)
		}
	}
	if fmt.Sprint(c) != before {
		t.Fatal("failed alias parse modified the catalogue")
	}
}

func TestAliasCapsPreferPopularTitles(t *testing.T) {
	c := &Catalog{Version: 1, Titles: []Title{{ID: "tt1", Name: "Popular", Type: "movie", Votes: 900}, {ID: "tt2", Name: "Obscure", Type: "movie", Votes: 10}}}
	var akas strings.Builder
	akas.WriteString("titleId\tordering\ttitle\tregion\tlanguage\ttypes\tattributes\tisOriginalTitle\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&akas, "tt1\t%d\tTitre populaire %d\tFR\t\\N\talternative\t\\N\t0\n", i, i)
	}
	fmt.Fprintf(&akas, "tt1\t99\tTitre officiel\tFR\t\\N\timdbDisplay\t\\N\t0\n")
	if err := ParseIMDbAliases(context.Background(), strings.NewReader(akas.String()), c, AliasOptions{Language: "fr"}); err != nil {
		t.Fatal(err)
	}
	if got := c.Titles[0].Aliases; len(got) != AliasesPerTitle || got[0] != "Titre officiel" {
		t.Fatalf("per-title cap/priority: %v", got)
	}
	// The profile budget applies to real builds.
	compact, _ := ProfileByID("compact")
	many := &Catalog{Version: 1, Profile: "compact"}
	akas.Reset()
	akas.WriteString("titleId\tordering\ttitle\tregion\tlanguage\ttypes\tattributes\tisOriginalTitle\n")
	for i := 0; i < compact.Aliases/2+10; i++ {
		many.Titles = append(many.Titles, Title{ID: fmt.Sprintf("tt%d", i+1), Name: "Name", Type: "movie", Votes: i})
		fmt.Fprintf(&akas, "tt%d\t1\tPremier titre %d\tFR\t\\N\t\\N\t\\N\t0\n", i+1, i)
		fmt.Fprintf(&akas, "tt%d\t2\tSecond titre %d\tFR\t\\N\t\\N\t\\N\t0\n", i+1, i)
	}
	if err := ParseIMDbAliases(context.Background(), strings.NewReader(akas.String()), many, AliasOptions{Language: "fr"}); err != nil {
		t.Fatal(err)
	}
	if many.AliasCount() != compact.Aliases || len(many.Titles[0].Aliases) != 0 || len(many.Titles[len(many.Titles)-1].Aliases) != 2 {
		t.Fatalf("profile budget not applied by popularity: %d", many.AliasCount())
	}
	chosen := map[int32][]string{0: {"a", "b", "c"}, 1: {"d", "e"}}
	capAliases(chosen, []Title{{ID: "tt1", Votes: 5}, {ID: "tt2", Votes: 50}}, 3)
	if !reflect.DeepEqual(chosen, map[int32][]string{1: {"d", "e"}, 0: {"a"}}) {
		t.Fatalf("global cap: %v", chosen)
	}
}

func TestSnapshotsWithAndWithoutAliases(t *testing.T) {
	legacy, err := Load("../../testdata/catalog.json")
	if err != nil || legacy.AliasCount() != 0 || legacy.Options().Enabled() {
		t.Fatalf("legacy snapshot: %v", err)
	}
	c := aliasFixture(t, AliasOptions{Language: "fr", Region: "FR"})
	path := filepath.Join(t.TempDir(), "catalog.json.gz")
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.AliasLanguage != "fr" || loaded.AliasRegion != "FR" || !reflect.DeepEqual(aliasesByID(loaded), aliasesByID(c)) {
		t.Fatalf("round trip lost aliases: %v", err)
	}
	// A snapshot from a newer build with an unknown language still loads.
	loaded.AliasLanguage = "tlh"
	if loaded.Validate() != nil {
		t.Fatal("unknown future language rejected")
	}
	loaded.Titles[0].Aliases = make([]string, MaxAliasesPerTitle+1)
	for i := range loaded.Titles[0].Aliases {
		loaded.Titles[0].Aliases[i] = "x"
	}
	if loaded.Validate() == nil {
		t.Fatal("unbounded aliases accepted")
	}
	loaded.Titles[0].Aliases = []string{""}
	if loaded.Validate() == nil {
		t.Fatal("empty alias accepted")
	}
}
