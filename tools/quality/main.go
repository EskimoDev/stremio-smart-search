// Command quality scores search accuracy on curated cases with known IMDb
// identifiers against explicitly selected local catalogue snapshots. Snapshots
// are built from IMDb data and must stay outside this repository.
//
//	go run ./tools/quality -cases testdata/quality/cases.json \
//	    -out results/after -baseline results/before  standard.json.gz standard-fr.json.gz
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/quality"
	"github.com/EskimoDev/stremio-smart-search/internal/search"
)

func main() {
	casesPath := flag.String("cases", "testdata/quality/cases.json", "curated case file")
	out := flag.String("out", "", "optional directory for one JSON report per catalogue")
	baseline := flag.String("baseline", "", "optional directory of earlier reports to compare against")
	verbose := flag.Bool("v", false, "list every failing case, not only changes")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: quality [flags] catalogue.json.gz ...")
		os.Exit(2)
	}
	cases, err := quality.LoadCases(*casesPath)
	must(err)
	failed := false
	for _, path := range flag.Args() {
		c, err := catalog.Load(path)
		must(err)
		prepared := quality.Verify(c, cases)
		engine := search.New(c.Titles)
		results := quality.Run(context.Background(), engine, prepared)
		name := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".gz"), ".json")
		report := quality.Report{Catalogue: name, Profile: c.Profile, Language: c.AliasLanguage, Titles: len(c.Titles), Summary: quality.Summarize(results), Results: results}
		fmt.Printf("\n## %s (%d titles%s)\n\n", name, len(c.Titles), langNote(c.AliasLanguage))
		printSummary(report.Summary)
		counts := map[string]int{}
		for _, r := range results {
			counts[r.Status]++
			if r.Status == quality.TruthError {
				failed = true
				fmt.Printf("GROUND TRUTH ERROR %s: %s\n", r.Name, r.Detail)
			}
		}
		fmt.Printf("\nScored %d; out of coverage %d; other language %d; regional title not stored %d.\n",
			counts[quality.Scored], counts[quality.Uncovered], counts[quality.OtherLang], counts[quality.NoAlias])
		if *verbose {
			for _, r := range results {
				if r.Status == quality.Scored && (r.Rank != 1 && len(r.Want) > 0 || len(r.FalsePositives) > 0) {
					fmt.Printf("  %-28s %-8s %-30q rank %-2d top %v", r.Name, r.Category, r.Query, r.Rank, first(r.Top, 3))
					if len(r.FalsePositives) > 0 {
						fmt.Printf(" FP %v", r.FalsePositives)
					}
					fmt.Println()
				}
			}
		}
		if *baseline != "" {
			var before quality.Report
			data, err := os.ReadFile(filepath.Join(*baseline, name+".json"))
			if err == nil {
				err = json.Unmarshal(data, &before)
			}
			if err != nil {
				fmt.Println("no baseline:", err)
			} else {
				fmt.Println("\nAgainst baseline:")
				printSummaryDiff(before.Summary, report.Summary)
				regressions, improvements := quality.Compare(before.Results, results)
				for _, label := range []struct {
					name string
					list []quality.Change
				}{{"Regressions", regressions}, {"Improvements", improvements}} {
					fmt.Printf("\n%s: %d\n", label.name, len(label.list))
					for _, ch := range label.list {
						fmt.Printf("  %-28s %-30q rank %d -> %d, false positives %+d\n", ch.Name, ch.Query, ch.Before, ch.After, ch.FPDiff)
					}
				}
			}
		}
		if *out != "" {
			must(os.MkdirAll(*out, 0o755))
			data, err := json.MarshalIndent(report, "", "  ")
			must(err)
			must(os.WriteFile(filepath.Join(*out, name+".json"), append(data, '\n'), 0o644))
		}
	}
	if failed {
		os.Exit(1)
	}
}

func langNote(language string) string {
	if language == "" {
		return ""
	}
	return ", alternative titles: " + language
}

func pct(n, d int) string {
	if d == 0 {
		return "–"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(d))
}

func printSummary(rows []quality.Summary) {
	fmt.Println("| Category | Cases | Top-1 | Recall@5 | MRR | Missing | False-positive cases |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, s := range rows {
		fmt.Printf("| %s | %d | %s | %s | %.3f | %d | %d |\n", s.Category, s.Cases, pct(s.Top1, s.Ranked), pct(s.RecallK, s.Ranked), s.MRR, s.Missing, s.FPCases)
	}
}

func printSummaryDiff(before, after []quality.Summary) {
	old := map[string]quality.Summary{}
	for _, s := range before {
		old[s.Category] = s
	}
	fmt.Println("| Category | Top-1 before → after | Recall@5 before → after | FP cases before → after |")
	fmt.Println("| --- | ---: | ---: | ---: |")
	for _, s := range after {
		o := old[s.Category]
		fmt.Printf("| %s | %s → %s | %s → %s | %d → %d |\n", s.Category, pct(o.Top1, o.Ranked), pct(s.Top1, s.Ranked), pct(o.RecallK, o.Ranked), pct(s.RecallK, s.Ranked), o.FPCases, s.FPCases)
	}
}

func first(list []string, n int) []string { return list[:min(n, len(list))] }

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
