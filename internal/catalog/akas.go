package catalog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/EskimoDev/stremio-smart-search/internal/textnorm"
)

const AkasURL = "https://datasets.imdbws.com/title.akas.tsv.gz"

// Candidates retained per title while streaming, before ranking and dedup.
// Popular titles have well over a hundred akas rows; one language rarely has
// more than a handful, so this only bounds pathological input.
const maxAliasCandidates = 24

type aliasCandidate struct {
	priority uint8
	ordering int
	title    string
}

// aliasPriority ranks IMDb akas "types". Lower is better; ok=false drops the row.
// Working and original rows are excluded: originals already come from
// title.basics, and working titles are pre-release names few people search for.
func aliasPriority(types string) (uint8, bool) {
	if types == `\N` || types == "" {
		return 1, true
	}
	best := uint8(3)
	for _, kind := range splitArray(types) {
		switch kind {
		case "working", "original":
			return 0, false
		case "imdbDisplay":
			best = 0
		case "alternative", "tv", "video", "dvd":
			best = min(best, 2)
		}
	}
	return best, true
}

// Attributes are free text. These describe variants that are rarely what a
// person types: literal translations, promotional, cut/format versions.
var lowValueAttributes = []string{"literal", "working", "promotional", "version", "fake", "rerun", "copyright"}

func lowValueAttribute(attributes string) bool {
	if attributes == `\N` || attributes == "" {
		return false
	}
	lower := strings.ToLower(attributes)
	for _, word := range lowValueAttributes {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// IMDb arrays are separated by \x02 in akas; commas are accepted too.
func splitArray(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == 2 || r == ',' })
}

// parseTitleID reads "tt123" from the start of a TSV line without allocating.
func parseTitleID(line []byte) (uint32, bool) {
	if len(line) < 3 || line[0] != 't' || line[1] != 't' {
		return 0, false
	}
	var n uint64
	i := 2
	for ; i < len(line) && line[i] >= '0' && line[i] <= '9'; i++ {
		if n = n*10 + uint64(line[i]-'0'); n > 1<<32-1 {
			return 0, false
		}
	}
	if i == 2 || (i < len(line) && line[i] != '\t') {
		return 0, false
	}
	return uint32(n), true
}

// ParseIMDbAliases streams title.akas (uncompressed TSV) and attaches up to
// AliasesPerTitle alternative titles in the chosen language/region to titles
// already selected in c, within the profile's alias budget (most-voted titles
// first). Rows for other titles are rejected after reading only their ID, so
// memory depends on the catalogue, not on the 50M-row source.
// On error, c is left unchanged.
func ParseIMDbAliases(ctx context.Context, reader io.Reader, c *Catalog, options AliasOptions) error {
	if err := options.Validate(); err != nil {
		return err
	}
	if !options.Enabled() {
		return nil
	}
	language, _ := LanguageByID(options.Language)
	s, stop := aheadScanner(reader)
	defer stop()
	if !s.Scan() {
		return fmt.Errorf("alternative-title file is empty")
	}
	header := strings.Split(s.Text(), "\t")
	columns := map[string]int{}
	largest := 0
	for _, key := range []string{"titleId", "title", "region", "language", "types", "attributes", "isOriginalTitle"} {
		index, err := column(header, key)
		if err != nil {
			return err
		}
		columns[key] = index
		largest = max(largest, index)
	}
	orderingCol, orderingErr := column(header, "ordering")
	if orderingErr == nil {
		largest = max(largest, orderingCol)
	}
	fastID := columns["titleId"] == 0

	selected := make(map[uint32]int32, len(c.Titles))
	for i, t := range c.Titles {
		if id, err := numericID(t.ID); err == nil {
			selected[id] = int32(i)
		}
	}
	candidates := map[int32][]aliasCandidate{}
	// Rows arrive grouped by title, so remember the last lookup: most rows
	// then skip the map entirely.
	lastID, lastIndex, lastOK := uint32(0), int32(0), false
	for rows := 0; s.Scan(); rows++ {
		if rows&0xfff == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		line := s.Bytes()
		var index int32
		if fastID {
			id, ok := parseTitleID(line)
			if !ok {
				continue
			}
			if id != lastID || rows == 0 {
				lastIndex, lastOK = selected[id]
				lastID = id
			}
			if index = lastIndex; !lastOK {
				continue
			}
		}
		if !utf8.Valid(line) || bytes.Count(line, []byte{'\t'}) < largest {
			continue
		}
		row := strings.Split(string(line), "\t")
		if !fastID {
			id, err := numericID(row[columns["titleId"]])
			var ok bool
			if index, ok = selected[id]; err != nil || !ok {
				continue
			}
		}
		title := row[columns["title"]]
		if row[columns["isOriginalTitle"]] == "1" || title == `\N` || title == "" || len(title) > maxAliasBytes {
			continue
		}
		if !language.accepts(row[columns["region"]], row[columns["language"]], options.Region) || lowValueAttribute(row[columns["attributes"]]) {
			continue
		}
		priority, ok := aliasPriority(row[columns["types"]])
		if !ok {
			continue
		}
		ordering := 1 << 30
		if orderingErr == nil {
			if n, err := strconv.Atoi(row[orderingCol]); err == nil {
				ordering = n
			}
		}
		candidate := aliasCandidate{priority, ordering, strings.Clone(title)}
		list := candidates[index]
		if len(list) < maxAliasCandidates {
			candidates[index] = append(list, candidate)
			continue
		}
		worst := 0
		for i := range list {
			if list[i].priority > list[worst].priority {
				worst = i
			}
		}
		if candidate.priority < list[worst].priority {
			list[worst] = candidate
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	chosen := make(map[int32][]string, len(candidates))
	total := 0
	for index, list := range candidates {
		sort.Slice(list, func(i, j int) bool {
			if list[i].priority != list[j].priority {
				return list[i].priority < list[j].priority
			}
			if list[i].ordering != list[j].ordering {
				return list[i].ordering < list[j].ordering
			}
			return list[i].title < list[j].title
		})
		t := c.Titles[index]
		seen := map[string]bool{textnorm.Normalize(t.Name): true, textnorm.Normalize(t.Original): true}
		var kept []string
		for _, candidate := range list {
			normal := textnorm.Normalize(candidate.title)
			if seen[normal] || !textnorm.Meaningful(normal) {
				continue
			}
			seen[normal] = true
			kept = append(kept, candidate.title)
			if len(kept) == AliasesPerTitle {
				break
			}
		}
		if len(kept) > 0 {
			chosen[index] = kept
			total += len(kept)
		}
	}
	budget := MaxAliases
	if profile, ok := ProfileByID(c.Profile); ok {
		budget = profile.Aliases
	}
	if total > budget {
		capAliases(chosen, c.Titles, budget)
	}
	for i := range c.Titles {
		c.Titles[i].Aliases = chosen[int32(i)]
	}
	c.AliasLanguage, c.AliasRegion = options.Language, options.Region
	return nil
}

// capAliases keeps at most budget aliases, preferring the most-voted titles so
// a bound never removes aliases from popular titles in favour of obscure ones.
func capAliases(chosen map[int32][]string, titles []Title, budget int) {
	order := make([]int32, 0, len(chosen))
	for index := range chosen {
		order = append(order, index)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := titles[order[i]], titles[order[j]]
		if a.Votes != b.Votes {
			return a.Votes > b.Votes
		}
		return a.ID < b.ID
	})
	for _, index := range order {
		n := min(len(chosen[index]), budget)
		if n == 0 {
			delete(chosen, index)
		} else {
			chosen[index] = chosen[index][:n]
		}
		budget -= n
	}
}
