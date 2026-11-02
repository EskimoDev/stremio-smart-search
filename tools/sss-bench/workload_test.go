package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/quality"
)

const caseDir = "../../testdata/quality"

func allSuites(t *testing.T) []suiteFile {
	t.Helper()
	var suites []suiteFile
	for _, name := range []string{"cases", "phonetic-holdout", "phonetic-validation"} {
		suite, err := loadSuite(caseDir, name)
		if err != nil {
			t.Fatal(err)
		}
		suites = append(suites, suite)
	}
	return suites
}

func TestFixtureIsDeterministicAndValid(t *testing.T) {
	suites := allSuites(t)
	a, b := buildFixture(3000, suites, fixtureLanguage), buildFixture(3000, suites, fixtureLanguage)
	if contentFingerprint(a) != contentFingerprint(b) {
		t.Fatal("the fixture differs between two builds")
	}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(a.Titles) != 3000 {
		t.Fatalf("%d titles", len(a.Titles))
	}
	if other := buildFixture(3001, suites, fixtureLanguage); contentFingerprint(other) == contentFingerprint(a) {
		t.Fatal("a different size must have a different fingerprint")
	}
}

func TestFixtureAnswersEveryCuratedCase(t *testing.T) {
	suites := allSuites(t)
	c := buildFixture(3000, suites, fixtureLanguage)
	for _, suite := range suites {
		for _, p := range quality.Verify(c, suite.cases) {
			switch p.Status {
			case quality.TruthError, quality.Uncovered:
				t.Errorf("%s: %s %s", p.Name, p.Status, p.Detail)
			case quality.NoAlias:
				t.Errorf("%s: its alias should be in the fixture", p.Name)
			}
		}
	}
}

func TestFixtureGivesFalsePositiveChecksSomethingToCatch(t *testing.T) {
	suites := allSuites(t)
	c := buildFixture(3000, suites, fixtureLanguage)
	for _, suite := range suites {
		for _, tc := range suite.cases {
			for _, fragment := range tc.Forbid {
				found := false
				for _, title := range c.Titles {
					if title.Type == tc.Kind && strings.EqualFold(title.Name, fragment) {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: no decoy for %q", tc.Name, fragment)
				}
			}
		}
	}
}

func TestFingerprintIgnoresFileAndOrderOnly(t *testing.T) {
	c := &catalog.Catalog{Version: 1, Titles: []catalog.Title{{ID: "tt1", Name: "A", Type: "movie"}, {ID: "tt2", Name: "B", Type: "movie"}}}
	reordered := &catalog.Catalog{Version: 1, Source: "elsewhere", Titles: []catalog.Title{c.Titles[1], c.Titles[0]}}
	if contentFingerprint(c) != contentFingerprint(reordered) {
		t.Fatal("order and provenance should not change the fingerprint")
	}
	changed := &catalog.Catalog{Version: 1, Titles: []catalog.Title{c.Titles[0], {ID: "tt2", Name: "B", Type: "movie", Votes: 1}}}
	if contentFingerprint(c) == contentFingerprint(changed) {
		t.Fatal("a changed title must change the fingerprint")
	}
}

func TestEveryCategoryHasALatencyClass(t *testing.T) {
	for _, category := range quality.Categories {
		class := classOf(category)
		found := false
		for _, known := range queryClasses {
			found = found || known == class
		}
		if !found {
			t.Errorf("%s maps to unknown class %q", category, class)
		}
	}
	for query, expected := range map[string]string{"hotd": "short", "千と千尋": "short", "interstellar": "medium", "the lord of the rings": "long"} {
		if got := lengthOf(query); got != expected {
			t.Errorf("%s: %s", query, got)
		}
	}
}

func TestWorkloadIsStableAndCoversCommonWords(t *testing.T) {
	cases := allSuites(t)[0].cases
	a, b := buildQueries(cases), buildQueries(cases)
	if queriesSHA(a) != queriesSHA(b) {
		t.Fatal("query set is not deterministic")
	}
	counts := map[string]int{}
	for _, q := range a {
		counts[q.Class]++
	}
	for _, class := range queryClasses {
		if counts[class] == 0 {
			t.Errorf("no %s queries", class)
		}
	}
}

func TestWorkloadIDChangesWithTheExperimentButNotItsLength(t *testing.T) {
	base := WorkloadInfo{Version: 1, Profile: "quick", Repetitions: 4, Passes: 3}
	id := workloadID(base)
	longer := base
	longer.Repetitions = 8
	if workloadID(longer) != id {
		t.Fatal("more repetitions of the same experiment must stay comparable")
	}
	changed := base
	changed.Passes = 4
	if workloadID(changed) == id {
		t.Fatal("a different number of passes is a different experiment")
	}
}

func TestIMDbShapedFilesParseBackToTheFixture(t *testing.T) {
	c := buildFixture(2000, allSuites(t), fixtureLanguage)
	dir := t.TempDir()
	if err := writeIMDbShaped(dir, c); err != nil {
		t.Fatal(err)
	}
	open := func(name string) *gzip.Reader {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		return reader
	}
	profile, _ := catalog.ProfileByID("compact")
	parsed, err := catalog.ParseIMDbProfile(context.Background(), open("title.ratings.tsv.gz"), open("title.basics.tsv.gz"), profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Titles) != len(c.Titles) {
		t.Fatalf("parsed %d titles from %d", len(parsed.Titles), len(c.Titles))
	}
}

func TestLoadSuiteReportsMissingAndCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadSuite(dir, "cases"); err == nil || !strings.Contains(err.Error(), "cases.json") {
		t.Fatalf("missing file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cases.json"), []byte(`{"cases": [`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSuite(dir, "cases"); err == nil {
		t.Fatal("a corrupt file must be an error")
	}
	if err := os.WriteFile(filepath.Join(dir, "cases.json"), []byte(`{"cases":[{"name":"x","query":"q","kind":"film","category":"exact"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSuite(dir, "cases"); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("invalid case: %v", err)
	}
}
