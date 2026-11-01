package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/quality"
	"github.com/EskimoDev/stremio-smart-search/tools/internal/benchdata"
	"github.com/EskimoDev/stremio-smart-search/tools/sss-bench/probes/probekit"
)

const (
	workloadSeed = 20261101
	// fixtureLanguage is the alias language of the synthetic catalogue, chosen
	// because it has the most curated regional cases.
	fixtureLanguage = "fr"
	searchLimit     = 20 // what the server asks of the engine
)

// fixtureBuiltAt is fixed so that a fixture's content never depends on the clock.
var fixtureBuiltAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

// suiteFile is one curated case file.
type suiteFile struct {
	name   string
	sha256 string
	cases  []quality.Case
}

// loadSuite reads a case file with the same checks tools/quality applies.
func loadSuite(dir, name string) (suiteFile, error) {
	path := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return suiteFile{}, fmt.Errorf("case file %s.json: %w", name, err)
	}
	cases, err := quality.LoadCases(path)
	if err != nil {
		return suiteFile{}, fmt.Errorf("case file %s.json: %w", name, err)
	}
	sum := sha256.Sum256(data)
	return suiteFile{name: name, sha256: hex.EncodeToString(sum[:]), cases: cases}, nil
}

// benchQuery is one workload query with the labels latency is grouped by.
type benchQuery struct {
	probekit.Query
	Class, Length, Path string
}

// classOf maps a curated category to the coarser class latency is reported by.
func classOf(category string) string {
	switch category {
	case "exact", "original":
		return "exact"
	case "fuzzy", "transposition":
		return "typo"
	case "punctuation":
		return "joined"
	case "accent", "non-latin":
		return "international"
	case "alias", "alias-fuzzy":
		return "alias"
	case "similar", "year":
		return "ambiguous"
	case "phonetic", "acronym", "joined", "short", "negative":
		return category
	}
	return "other"
}

func lengthOf(query string) string {
	switch n := utf8.RuneCountInString(query); {
	case n <= 5:
		return "short"
	case n <= 15:
		return "medium"
	}
	return "long"
}

// commonWordQueries are the slowest honest searches: words shared by a great
// many titles. Curated cases rarely contain them because they have no one answer.
var commonWordQueries = []probekit.Query{{Text: "the", Kind: "movie"}, {Text: "man", Kind: "movie"}, {Text: "love", Kind: "movie"}, {Text: "night", Kind: "series"}}

// buildQueries makes the timed workload from cases.json, the curated mix of real
// search forms, so the timed queries and the scored ones stay the same set.
func buildQueries(cases []quality.Case) []benchQuery {
	seen := map[probekit.Query]bool{}
	var queries []benchQuery
	add := func(q probekit.Query, class string) {
		if !seen[q] {
			seen[q] = true
			path := "/catalog/" + q.Kind + "/smart-search/search=" + url.QueryEscape(q.Text) + ".json"
			queries = append(queries, benchQuery{Query: q, Class: class, Length: lengthOf(q.Text), Path: path})
		}
	}
	for _, c := range cases {
		add(probekit.Query{Text: c.Query, Kind: c.Kind}, classOf(c.Category))
	}
	for _, q := range commonWordQueries {
		add(q, "common")
	}
	return queries
}

func queriesSHA(queries []benchQuery) string {
	data, _ := json.Marshal(queries)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// workloadID covers everything that changes what a timing means. Repetitions are
// excluded so more of the same experiment remains comparable.
func workloadID(w WorkloadInfo) string {
	w.ID, w.Repetitions, w.Interleaved = "", 0, false
	data, _ := json.Marshal(w)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

// title hashes an identifier into a stable pseudo-random number, so curated
// titles keep the same popularity however the cases are ordered.
func stableHash(text string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(text))
	return h.Sum64()
}

// curatedTitle builds a real title from what the case file records about it.
func curatedTitle(w quality.Want, kind string) catalog.Title {
	votes := 200_000 + int(stableHash(w.ID)%1_800_000)
	return catalog.Title{ID: w.ID, Name: w.Title, Type: kind, Year: w.Year, Votes: votes, Rating: 8}
}

// buildFixture makes the deterministic offline catalogue: every title the curated
// cases expect, one decoy per forbidden result so false-positive checks have
// something to catch, and invented titles up to the requested size.
//
// It holds no IMDb data beyond the identifiers and titles already in the case
// files, and the popularity figures are invented. Its quality scores detect
// ranking changes; they say nothing about accuracy on a real catalogue.
func buildFixture(titles int, suites []suiteFile, language string) *catalog.Catalog {
	byID := map[string]*catalog.Title{}
	var order []string
	aliases := map[string][]string{}
	decoys := []catalog.Title{}
	seenDecoy := map[string]bool{}
	for _, suite := range suites {
		for _, c := range suite.cases {
			for _, w := range c.Expect {
				t := byID[w.ID]
				if t == nil {
					title := curatedTitle(w, c.Kind)
					byID[w.ID] = &title
					order = append(order, w.ID)
					continue
				}
				// Cases may name a title by its primary or its original name.
				if w.Title != t.Name && t.Original == "" {
					t.Original = w.Title
				}
			}
			if c.Language == language && c.Alias != "" && len(c.Expect) > 0 && !slices.Contains(aliases[c.Expect[0].ID], c.Alias) {
				aliases[c.Expect[0].ID] = append(aliases[c.Expect[0].ID], c.Alias)
			}
			for _, fragment := range c.Forbid {
				if key := c.Kind + ":" + fragment; !seenDecoy[key] {
					seenDecoy[key] = true
					decoys = append(decoys, catalog.Title{Name: strings.ToUpper(fragment[:1]) + fragment[1:], Type: c.Kind, Year: 2001, Votes: 30_000, Rating: 6})
				}
			}
		}
	}
	out := make([]catalog.Title, 0, max(titles, len(order)+len(decoys)))
	for _, id := range order {
		t := *byID[id]
		t.Aliases = aliases[id][:min(len(aliases[id]), catalog.AliasesPerTitle)]
		if len(t.Aliases) == 0 {
			t.Aliases = nil
		}
		out = append(out, t)
	}
	number := 900_000_000
	nextID := func() string {
		number++
		return fmt.Sprintf("tt%d", number)
	}
	for _, d := range decoys {
		d.ID = nextID()
		out = append(out, d)
	}
	r := rand.New(rand.NewPCG(workloadSeed, 2))
	vocabulary := benchdata.NewVocab(r, max(2_000, titles))
	for i := len(out); i < titles; i++ {
		name := vocabulary.Title(r)
		if r.IntN(40) == 0 {
			name = benchdata.NonLatin(r)
		}
		kind := "movie"
		if i%2 == 1 {
			kind = "series"
		}
		// Heavy-tailed popularity, above every catalogue size's minimum vote count
		// so a prepared catalogue keeps all of them.
		votes := 300 + int(math.Min(1_500_000, 1/math.Pow(1-r.Float64(), 3.2)))
		out = append(out, catalog.Title{ID: nextID(), Name: name, Type: kind, Year: 1930 + r.IntN(96), Votes: votes, Rating: 1 + float64(r.IntN(90))/10})
	}
	return &catalog.Catalog{Version: 1, AliasLanguage: language, BuiltAt: fixtureBuiltAt, Source: "sss-bench synthetic fixture: invented titles plus the titles named in the quality cases; not IMDb data", Titles: out}
}

// contentFingerprint identifies a catalogue by what it holds, not by its file, so
// recompressing a snapshot or rebuilding it at another time does not change it.
func contentFingerprint(c *catalog.Catalog) string {
	titles := slices.Clone(c.Titles)
	slices.SortFunc(titles, func(a, b catalog.Title) int {
		return strings.Compare(a.Type+a.ID, b.Type+b.ID)
	})
	h := sha256.New()
	encoder := json.NewEncoder(h)
	encoder.Encode([]string{c.Profile, c.AliasLanguage, c.AliasRegion})
	for _, t := range titles {
		encoder.Encode(t)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// describeCatalogue counts a catalogue's contents.
func describeCatalogue(c *catalog.Catalog, kind, name, path string) (CatalogueInfo, error) {
	info := CatalogueInfo{Kind: kind, Name: name, ContentSHA256: contentFingerprint(c), Titles: len(c.Titles), Aliases: c.AliasCount(), AliasLanguage: c.AliasLanguage, Profile: c.Profile, BuiltAt: c.BuiltAt, Source: c.Source}
	for _, t := range c.Titles {
		if t.Type == "movie" {
			info.Movies++
		} else {
			info.Series++
		}
	}
	sum, size, err := fileFingerprint(path)
	info.FileSHA256, info.FileBytes = sum, size
	return info, err
}

func fileFingerprint(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), size, err
}

// writeIMDbShaped writes a catalogue as IMDb-shaped files, with the unrated
// episode rows that make up most of the real basics file and that the parser must
// reject, so a prepared fixture exercises the same code as a real preparation.
func writeIMDbShaped(dir string, c *catalog.Catalog) error {
	create := func(name string) (*gzip.Writer, func() error, error) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		z := gzip.NewWriter(f)
		return z, func() error {
			if err := z.Close(); err != nil {
				f.Close()
				return err
			}
			return f.Close()
		}, nil
	}
	basics, closeBasics, err := create("title.basics.tsv.gz")
	if err != nil {
		return err
	}
	ratings, closeRatings, err := create("title.ratings.tsv.gz")
	if err != nil {
		closeBasics()
		return err
	}
	fmt.Fprintln(basics, "tconst\ttitleType\tprimaryTitle\toriginalTitle\tisAdult\tstartYear\tendYear\truntimeMinutes\tgenres")
	fmt.Fprintln(ratings, "tconst\taverageRating\tnumVotes")
	for i, t := range c.Titles {
		kind := "movie"
		if t.Type == "series" {
			kind = "tvSeries"
		}
		original := t.Original
		if original == "" {
			original = t.Name
		}
		fmt.Fprintf(basics, "%s\t%s\t%s\t%s\t0\t%d\t\\N\t100\tDrama\n", t.ID, kind, t.Name, original, t.Year)
		fmt.Fprintf(ratings, "%s\t%.1f\t%d\n", t.ID, t.Rating, t.Votes)
		for episode := 0; episode < 3; episode++ {
			fmt.Fprintf(basics, "tt%d\ttvEpisode\tEpisode %d\tEpisode %d\t0\t2001\t\\N\t45\tDrama\n", 800_000_000+i*3+episode, episode+1, episode+1)
		}
	}
	if err := closeBasics(); err != nil {
		closeRatings()
		return err
	}
	return closeRatings()
}
