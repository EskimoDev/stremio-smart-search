package catalog

import (
	"bufio"
	"compress/gzip"
	"container/heap"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const RatingsURL = "https://datasets.imdbws.com/title.ratings.tsv.gz"
const BasicsURL = "https://datasets.imdbws.com/title.basics.tsv.gz"

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
	ratings, err := parseRatings(ctx, ratingsReader)
	if err != nil {
		return nil, err
	}
	return parseBasics(ctx, basicsReader, ratings)
}
func parseRatings(ctx context.Context, reader io.Reader) (map[uint32]rating, error) {
	s := scanner(reader)
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
	for s.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		row := strings.Split(s.Text(), "\t")
		if len(row) <= largest {
			continue
		}
		votes, e := strconv.Atoi(row[voteCol])
		if e != nil || votes < 250 {
			continue
		}
		id, e := numericID(row[idCol])
		if e != nil {
			continue
		}
		value, e := strconv.ParseFloat(row[ratingCol], 64)
		if e != nil {
			continue
		}
		ratings[id] = rating{votes, value}
		if len(ratings) > 1000000 {
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
func parseBasics(ctx context.Context, reader io.Reader, ratings map[uint32]rating) (*Catalog, error) {
	s := scanner(reader)
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
	for s.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		row := strings.Split(s.Text(), "\t")
		if len(row) <= largest || row[columns["isAdult"]] != "0" {
			continue
		}
		var kind string
		switch row[columns["titleType"]] {
		case "movie", "tvMovie":
			kind = "movie"
		case "tvSeries", "tvMiniSeries":
			kind = "series"
		default:
			continue
		}
		id := row[columns["tconst"]]
		numeric, err := numericID(id)
		if err != nil {
			continue
		}
		r, ok := ratings[numeric]
		if !ok {
			continue
		}
		name := row[columns["primaryTitle"]]
		if name == "" || len(name) > 1000 {
			continue
		}
		year, _ := strconv.Atoi(row[columns["startYear"]])
		original := row[columns["originalTitle"]]
		if original == `\N` || original == name {
			original = ""
		}
		t := Title{ID: id, Name: name, Original: original, Type: kind, Year: year, Votes: r.votes, Rating: r.value}
		target := movies
		if kind == "series" {
			target = series
		}
		if target.Len() < MaxTitles/2 {
			heap.Push(target, t)
		} else if t.Votes > (*target)[0].Votes || (t.Votes == (*target)[0].Votes && t.ID < (*target)[0].ID) {
			(*target)[0] = t
			heap.Fix(target, 0)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	titles := append([]Title(nil), (*movies)...)
	titles = append(titles, (*series)...)
	c := &Catalog{Version: 1, BuiltAt: time.Now().UTC(), Source: "IMDb non-commercial datasets (downloaded on this device)", Titles: titles}
	return c, c.Validate()
}

type countingReader struct {
	reader   io.Reader
	total    int64
	last     time.Time
	stage    string
	progress Progress
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.total += int64(n)
	if time.Since(c.last) > 500*time.Millisecond {
		c.last = time.Now()
		if c.progress != nil {
			c.progress(c.stage, c.total)
		}
	}
	return n, err
}

func openDataset(ctx context.Context, url, stage string, maximum int64, progress Progress) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "StremioSmartSearch/0.1 (+https://github.com/EskimoDev/stremio-smart-search)")
	client := &http.Client{Timeout: 30 * time.Minute}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("IMDb download returned HTTP %d; try again later", response.StatusCode)
	}
	if response.ContentLength > maximum {
		response.Body.Close()
		return nil, fmt.Errorf("dataset download exceeds its size limit")
	}
	counted := &countingReader{reader: io.LimitReader(response.Body, maximum+1), stage: stage, progress: progress}
	z, err := gzip.NewReader(counted)
	if err != nil {
		response.Body.Close()
		return nil, err
	}
	return &datasetReader{Reader: z, body: response.Body, gzip: z}, nil
}

type datasetReader struct {
	io.Reader
	body io.Closer
	gzip io.Closer
}

func (d *datasetReader) Close() error { _ = d.gzip.Close(); return d.body.Close() }

func DownloadIMDb(ctx context.Context, progress Progress) (*Catalog, error) {
	if progress != nil {
		progress("Downloading title ratings", 0)
	}
	r, err := openDataset(ctx, RatingsURL, "Downloading title ratings", 128<<20, progress)
	if err != nil {
		return nil, err
	}
	ratings, err := parseRatings(ctx, r)
	_ = r.Close()
	if err != nil {
		return nil, err
	}
	if progress != nil {
		progress("Downloading and selecting movie and TV titles", 0)
	}
	b, err := openDataset(ctx, BasicsURL, "Downloading and selecting movie and TV titles", 768<<20, progress)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	return parseBasics(ctx, b, ratings)
}
