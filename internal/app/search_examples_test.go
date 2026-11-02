package app

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/search"
)

type searchExample struct {
	label, query, name string
	year               int
}

var exampleRow = regexp.MustCompile(`<dt>([^<]+)</dt><dd><code>([^<]+)</code> <span>([^<]+)</span></dd>`)
var exampleYear = regexp.MustCompile(`^(.*) \((\d{4})\)$`)

// settingsSearchExamples reads the "Ready to try it?" examples from the
// embedded settings page, so the page and this check cannot drift apart.
func settingsSearchExamples(t *testing.T) []searchExample {
	t.Helper()
	page, err := web.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	var examples []searchExample
	for _, row := range exampleRow.FindAllStringSubmatch(string(page), -1) {
		example := searchExample{label: html.UnescapeString(row[1]), query: html.UnescapeString(row[2]), name: html.UnescapeString(row[3])}
		if match := exampleYear.FindStringSubmatch(example.name); match != nil {
			example.name = match[1]
			example.year, _ = strconv.Atoi(match[2])
		}
		examples = append(examples, example)
	}
	return examples
}

func TestSettingsSearchExamplesAreListed(t *testing.T) {
	examples := settingsSearchExamples(t)
	if len(examples) < 5 {
		t.Fatalf("found %d search examples on the settings page; the markup or this test's pattern changed", len(examples))
	}
	seen := map[string]bool{}
	for _, example := range examples {
		if seen[example.label] {
			t.Errorf("capability %q is shown twice", example.label)
		}
		seen[example.label] = true
	}
}

// TestSettingsSearchExamplesOnRealData checks that every example on the
// settings page returns its title first in the movie or series row. Point
// SMART_SEARCH_EXAMPLE_CATALOGS at one or more locally built snapshots
// (separated like PATH); they are never committed.
func TestSettingsSearchExamplesOnRealData(t *testing.T) {
	paths := filepath.SplitList(os.Getenv("SMART_SEARCH_EXAMPLE_CATALOGS"))
	if len(paths) == 0 {
		t.Skip("set SMART_SEARCH_EXAMPLE_CATALOGS to locally built catalogue snapshots")
	}
	examples := settingsSearchExamples(t)
	for _, path := range paths {
		c, err := catalog.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		engine := search.New(c.Titles)
		for _, example := range examples {
			var firsts []string
			found := false
			for _, kind := range []string{"movie", "series"} {
				results := engine.Search(context.Background(), example.query, kind, 1)
				if len(results) == 0 {
					continue
				}
				first := results[0]
				firsts = append(firsts, fmt.Sprintf("%s: %s (%d)", kind, first.Name, first.Year))
				if strings.EqualFold(first.Name, example.name) && (example.year == 0 || int(first.Year) == example.year) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: %s example %q should list %q first, got %v", filepath.Base(path), example.label, example.query, example.name, firsts)
			}
		}
	}
}
