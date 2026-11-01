package catalog

import (
	"bufio"
	"container/heap"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Dataset file names on IMDb's server; see Source.
const (
	RatingsFile = "title.ratings.tsv.gz"
	BasicsFile  = "title.basics.tsv.gz"
	AkasFile    = "title.akas.tsv.gz"
)

const RatingsURL = "https://datasets.imdbws.com/" + RatingsFile
const BasicsURL = "https://datasets.imdbws.com/" + BasicsFile

type Progress func(stage string, bytes int64)
type rating struct {
	votes int
	value float64
}
type titleHeap []Title

func (h titleHeap) Len() int { return len(h) }
func (h titleHeap) Less(i, j int) bool {
	if h[i].Votes != h[j].Votes {
		return h[i].Votes < h[j].Votes
	}
	return h[i].ID > h[j].ID
}
func (h titleHeap) Swap(i, j int)   { h[i], h[j] = h[j], h[i] }
func (h *titleHeap) Push(value any) { *h = append(*h, value.(Title)) }
func (h *titleHeap) Pop() any {
	old := *h
	value := old[len(old)-1]
	*h = old[:len(old)-1]
	return value
}

func scanner(reader io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(reader)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	return s
}

// aheadScanner is scanner with reading moved to its own goroutine; call stop
// when done.
func aheadScanner(reader io.Reader) (*bufio.Scanner, func()) {
	ahead, stop := readAhead(reader)
	return scanner(ahead), stop
}

func column(header []string, name string) (int, error) {
	for i, v := range header {
		if v == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("IMDb data is missing column %s", name)
}
func numericID(id string) (uint32, error) {
	if !strings.HasPrefix(id, "tt") {
		return 0, fmt.Errorf("not a title ID")
	}
	n, err := strconv.ParseUint(id[2:], 10, 32)
	return uint32(n), err
}

// ParseIMDb consumes uncompressed TSV streams; production downloads are streamed
// through gzip readers. No raw multi-gigabyte dataset is retained on disk.
func ParseIMDb(ctx context.Context, ratingsReader, basicsReader io.Reader) (*Catalog, error) {
	profile, _ := ProfileByID(DefaultProfile)
	return ParseIMDbProfile(ctx, ratingsReader, basicsReader, profile)
}

func ParseIMDbProfile(ctx context.Context, ratingsReader, basicsReader io.Reader, profile Profile) (*Catalog, error) {
	if known, ok := ProfileByID(profile.ID); !ok || known != profile {
		return nil, fmt.Errorf("unknown catalogue profile")
	}
	ratings, err := parseRatings(ctx, ratingsReader, profile.MinVotes)
	if err != nil {
		return nil, err
	}
	return parseBasics(ctx, basicsReader, ratings, profile)
}
func parseRatings(ctx context.Context, reader io.Reader, minVotes int) (map[uint32]rating, error) {
	s, stop := aheadScanner(reader)
	defer stop()
	if !s.Scan() {
		return nil, fmt.Errorf("ratings file is empty")
	}
	header := strings.Split(s.Text(), "\t")
	idCol, err := column(header, "tconst")
	if err != nil {
		return nil, err
	}
	voteCol, err := column(header, "numVotes")
	if err != nil {
		return nil, err
	}
	ratingCol, err := column(header, "averageRating")
	if err != nil {
		return nil, err
	}
	ratings := make(map[uint32]rating)
	largest := max(idCol, voteCol, ratingCol)
	var row [][]byte
	for s.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		row = splitTabs(s.Bytes(), row, largest+1)
		if len(row) <= largest {
			continue
		}
		votes, e := atoi(row[voteCol])
		if e != nil || votes < minVotes {
			continue
		}
		id, ok := parseTitleID(row[idCol])
		if !ok {
			continue
		}
		value, e := strconv.ParseFloat(string(row[ratingCol]), 64)
		if e != nil {
			continue
		}
		ratings[id] = rating{votes, value}
		if len(ratings) > 4000000 {
			return nil, fmt.Errorf("ratings exceed the configured memory limit")
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(ratings) == 0 {
		return nil, fmt.Errorf("no usable ratings in dataset")
	}
	return ratings, nil
}
func parseBasics(ctx context.Context, reader io.Reader, ratings map[uint32]rating, profile Profile) (*Catalog, error) {
	s, stop := aheadScanner(reader)
	defer stop()
	if !s.Scan() {
		return nil, fmt.Errorf("title file is empty")
	}
	header := strings.Split(s.Text(), "\t")
	columns := map[string]int{}
	largest := 0
	for _, key := range []string{"tconst", "titleType", "primaryTitle", "originalTitle", "isAdult", "startYear"} {
		index, err := column(header, key)
		if err != nil {
			return nil, err
		}
		columns[key] = index
		largest = max(largest, index)
	}
	movies, series := &titleHeap{}, &titleHeap{}
	heap.Init(movies)
	heap.Init(series)
	idCol, typeCol, nameCol, originalCol, adultCol, yearCol := columns["tconst"], columns["titleType"], columns["primaryTitle"], columns["originalTitle"], columns["isAdult"], columns["startYear"]
	var row [][]byte
	for s.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Most rows are episodes and other unsupported types: reject them from
		// the raw bytes before anything is converted or allocated.
		row = splitTabs(s.Bytes(), row, largest+1)
		if len(row) <= largest || string(row[adultCol]) != "0" {
			continue
		}
		var kind string
		switch string(row[typeCol]) {
		case "movie", "tvMovie":
			kind = "movie"
		case "tvSeries", "tvMiniSeries":
			kind = "series"
		default:
			continue
		}
		numeric, ok := parseTitleID(row[idCol])
		if !ok {
			continue
		}
		r, ok := ratings[numeric]
		if !ok {
			continue
		}
		if len(row[nameCol]) == 0 || len(row[nameCol]) > 1000 {
			continue
		}
		target := movies
		capacity := profile.Movies
		if kind == "series" {
			target = series
			capacity = profile.Series
		}
		id := string(row[idCol])
		if target.Len() >= capacity && !(r.votes > (*target)[0].Votes || (r.votes == (*target)[0].Votes && id < (*target)[0].ID)) {
			continue // would not displace the least-voted title kept so far
		}
		name := string(row[nameCol])
		year, _ := atoi(row[yearCol])
		original := ""
		if field := row[originalCol]; string(field) != `\N` && string(field) != name {
			original = string(field)
		}
		t := Title{ID: id, Name: name, Original: original, Type: kind, Year: year, Votes: r.votes, Rating: r.value}
		if target.Len() < capacity {
			heap.Push(target, t)
		} else {
			(*target)[0] = t
			heap.Fix(target, 0)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	titles := append([]Title(nil), (*movies)...)
	titles = append(titles, (*series)...)
	c := &Catalog{Version: 1, Profile: profile.ID, BuiltAt: time.Now().UTC(), Source: "IMDb non-commercial datasets (downloaded on this device)", Titles: titles}
	return c, c.Validate()
}

func DownloadIMDb(ctx context.Context, progress Progress) (*Catalog, error) {
	profile, _ := ProfileByID(DefaultProfile)
	return DownloadIMDbProfile(ctx, profile, progress)
}

// DownloadIMDbProfile builds a catalogue from IMDb's dataset server.
func DownloadIMDbProfile(ctx context.Context, profile Profile, progress Progress) (*Catalog, error) {
	return IMDb.DownloadProfile(ctx, profile, progress)
}

// DownloadIMDbAliases streams title.akas.tsv.gz and attaches the selected
// language's alternative titles to c. Nothing is downloaded when options are
// disabled. On failure c is unchanged and the caller keeps its previous snapshot.
func DownloadIMDbAliases(ctx context.Context, c *Catalog, options AliasOptions, progress Progress) error {
	return IMDb.DownloadAliases(ctx, c, options, progress)
}
