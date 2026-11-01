// Command benchmark compares explicitly selected local IMDb snapshots.
// Raw datasets and generated catalogues must stay outside the public repository.
package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/app"
	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

type processStats struct {
	CPUSeconds   float64
	RSSBytes     uint64
	PeakRSSBytes uint64
	PrivateBytes uint64
}

type sample struct {
	Query    string `json:"query"`
	Kind     string `json:"kind"`
	Expected string `json:"expected,omitempty"`
	FirstID  string `json:"firstID,omitempty"`
	Passed   bool   `json:"passed"`
}
type report struct {
	SteadyIdleSeconds    float64  `json:"steadyIdleSeconds,omitempty"`
	SteadyIdleCPUSeconds float64  `json:"steadyIdleCPUSeconds"`
	RefreshSearches      int      `json:"refreshSearches,omitempty"`
	Phase                string   `json:"phase"`
	Profile              string   `json:"profile"`
	Movies               int      `json:"movies"`
	Series               int      `json:"series"`
	MinVotes             int      `json:"minVotes"`
	CatalogBytes         int64    `json:"catalogBytes"`
	WallMS               float64  `json:"wallMS"`
	CPUSeconds           float64  `json:"cpuSeconds"`
	RSSBytes             uint64   `json:"rssBytes"`
	PrivateBytes         uint64   `json:"privateBytes"`
	HeapBytes            uint64   `json:"heapBytes"`
	PeakRSSBytes         uint64   `json:"peakRSSBytes"`
	IdleSeconds          float64  `json:"idleSeconds,omitempty"`
	IdleCPUSeconds       float64  `json:"idleCPUSeconds,omitempty"`
	Requests             int      `json:"requests,omitempty"`
	RequestCPUMS         float64  `json:"requestCPUMS,omitempty"`
	P50MS                float64  `json:"p50MS"`
	P95MS                float64  `json:"p95MS,omitempty"`
	P99MS                float64  `json:"p99MS,omitempty"`
	MaxMS                float64  `json:"maxMS,omitempty"`
	Cases                []sample `json:"cases,omitempty"`
}

func main() {
	phase := flag.String("phase", "measure", "prepare cached IMDb data, or measure a catalogue")
	dir := flag.String("data-dir", "", "Explicit directory for local raw data and snapshots")
	id := flag.String("profile", "compact", "compact, standard or expanded")
	iterations := flag.Int("iterations", 50, "Repetitions of the fixed loopback HTTP workload")
	memoryMiB := flag.Int64("memory-mib", 256, "Go soft memory target; match the installed helper")
	steadyIdle := flag.Duration("steady-idle", 0, "Optional quiet CPU sample after searches and a 3-second settling period")
	flag.Parse()
	runtime.GOMAXPROCS(2)
	debug.SetMemoryLimit(*memoryMiB << 20)
	profile, ok := catalog.ProfileByID(*id)
	if !ok || *dir == "" || *iterations < 1 {
		panic("choose a data directory, known profile and positive iteration count")
	}
	ctx := context.Background()
	path := filepath.Join(*dir, *id+".json.gz")
	r := report{Phase: *phase, Profile: *id, MinVotes: profile.MinVotes}
	before := readProcess()
	start := time.Now()
	switch *phase {
	case "prepare":
		ratings, closeRatings, err := openGzip(filepath.Join(*dir, "title.ratings.tsv.gz"))
		must(err)
		defer closeRatings()
		basics, closeBasics, err := openGzip(filepath.Join(*dir, "title.basics.tsv.gz"))
		must(err)
		defer closeBasics()
		c, err := catalog.ParseIMDbProfile(ctx, ratings, basics, profile)
		must(err)
		must(catalog.Save(path, c))
		for _, title := range c.Titles {
			if title.Type == "movie" {
				r.Movies++
			} else {
				r.Series++
			}
		}
		r.WallMS = float64(time.Since(start).Microseconds()) / 1000
		r.CPUSeconds = readProcess().CPUSeconds - before.CPUSeconds
		r.PeakRSSBytes = readProcess().PeakRSSBytes
		runtime.KeepAlive(c)
	case "measure", "refresh":
		a, err := app.New(ctx, filepath.Join(*dir, "controls-"+*id), "http://127.0.0.1:11479", "benchmark")
		must(err)
		must(a.Load(path))
		r.WallMS = float64(time.Since(start).Microseconds()) / 1000
		r.CPUSeconds = readProcess().CPUSeconds - before.CPUSeconds
		r.Movies = a.Status().Movies
		r.Series = a.Status().Series
		if *phase == "refresh" {
			a.DownloadCatalog = func(ctx context.Context, profile catalog.Profile, _ catalog.Progress) (*catalog.Catalog, error) {
				ratings, closeRatings, err := openGzip(filepath.Join(*dir, "title.ratings.tsv.gz"))
				if err != nil {
					return nil, err
				}
				defer closeRatings()
				basics, closeBasics, err := openGzip(filepath.Join(*dir, "title.basics.tsv.gz"))
				if err != nil {
					return nil, err
				}
				defer closeBasics()
				return catalog.ParseIMDbProfile(ctx, ratings, basics, profile)
			}
			server := httptest.NewServer(a.Handler())
			defer server.Close()
			request, err := http.NewRequest(http.MethodPost, server.URL+"/api/build", strings.NewReader(fmt.Sprintf("{\"profile\":%q}", *id)))
			must(err)
			request.Host = "127.0.0.1:11479"
			request.Header.Set("X-Smart-Search-Key", a.Token)
			before = readProcess()
			start = time.Now()
			response, err := http.DefaultClient.Do(request)
			must(err)
			response.Body.Close()
			if response.StatusCode != http.StatusAccepted {
				panic("refresh was not accepted")
			}
			for a.Status().Building {
				response, err := http.Get(server.URL + "/catalog/series/smart-search/search=HOTD.json")
				must(err)
				var body struct{ Metas []struct{ ID string } }
				must(json.NewDecoder(response.Body).Decode(&body))
				response.Body.Close()
				if len(body.Metas) == 0 || body.Metas[0].ID != "tt11198330" {
					panic("working index unavailable during refresh")
				}
				r.RefreshSearches++
				if time.Since(start) > 2*time.Minute {
					panic("refresh exceeded two-minute benchmark budget")
				}
				time.Sleep(100 * time.Millisecond)
			}
			if a.Status().Error != "" {
				panic(a.Status().Error)
			}
			r.WallMS = float64(time.Since(start).Microseconds()) / 1000
			time.Sleep(200 * time.Millisecond)
			r.CPUSeconds = readProcess().CPUSeconds - before.CPUSeconds
			r.PeakRSSBytes = readProcess().PeakRSSBytes
			runtime.KeepAlive(a)
			break
		}
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		process := readProcess()
		r.HeapBytes = memory.HeapAlloc
		r.RSSBytes = process.RSSBytes
		r.PrivateBytes = process.PrivateBytes
		idleStart := time.Now()
		idleBefore := readProcess()
		time.Sleep(3 * time.Second)
		r.IdleSeconds = time.Since(idleStart).Seconds()
		r.IdleCPUSeconds = readProcess().CPUSeconds - idleBefore.CPUSeconds
		// Match ordinary startup: do not force a GC or trim the working set.
		runtime.ReadMemStats(&memory)
		process = readProcess()
		r.HeapBytes = memory.HeapAlloc
		r.RSSBytes = process.RSSBytes
		r.PrivateBytes = process.PrivateBytes
		server := httptest.NewServer(a.Handler())
		defer server.Close()
		client := &http.Client{Timeout: 5 * time.Second}
		cases := []sample{
			{Query: "HOTD", Kind: "series", Expected: "tt11198330"},
			{Query: "house of dragom", Kind: "series", Expected: "tt11198330"},
			{Query: "interstelar", Kind: "movie", Expected: "tt0816692"},
			{Query: "LOTR", Kind: "movie"},
			{Query: "spiderman", Kind: "movie"},
			{Query: "man", Kind: "movie"},
			{Query: "the", Kind: "movie"},
			{Query: "love", Kind: "movie"},
			{Query: "harry poter", Kind: "movie"},
			{Query: "breaing bad", Kind: "series", Expected: "tt0903747"},
			{Query: "game thrones", Kind: "series", Expected: "tt0944947"},
			{Query: "dune part two", Kind: "movie"},
			{Query: "Amélie", Kind: "movie"},
			{Query: "千と千尋", Kind: "movie"},
			{Query: "qzxqvvnonexistent", Kind: "movie"},
		}
		request := func(test sample) []string {
			response, err := client.Get(server.URL + "/catalog/" + test.Kind + "/smart-search/search=" + url.QueryEscape(test.Query) + ".json")
			must(err)
			defer response.Body.Close()
			if response.StatusCode != 200 {
				panic(fmt.Sprintf("search returned HTTP %d", response.StatusCode))
			}
			var body struct{ Metas []struct{ ID string } }
			must(json.NewDecoder(response.Body).Decode(&body))
			ids := make([]string, 0, len(body.Metas))
			for _, meta := range body.Metas {
				ids = append(ids, meta.ID)
			}
			return ids
		}
		for i := range cases {
			ids := request(cases[i])
			if len(ids) > 0 {
				cases[i].FirstID = ids[0]
			}
			cases[i].Passed = cases[i].Expected == "" || slices.Contains(ids, cases[i].Expected)
			if !cases[i].Passed {
				fmt.Fprintln(os.Stderr, "quality check failed:", cases[i].Query, cases[i].FirstID)
			}
		}
		durations := make([]float64, 0, len(cases)**iterations)
		activeBefore := readProcess()
		for iteration := 0; iteration < *iterations; iteration++ {
			for _, test := range cases {
				started := clockMS()
				request(test)
				durations = append(durations, clockMS()-started)
			}
		}
		r.Requests = len(durations)
		r.RequestCPUMS = (readProcess().CPUSeconds - activeBefore.CPUSeconds) * 1000 / float64(r.Requests)
		sort.Float64s(durations)
		r.P50MS = durations[len(durations)/2]
		r.P95MS = durations[len(durations)*95/100]
		r.P99MS = durations[len(durations)*99/100]
		r.MaxMS = durations[len(durations)-1]
		r.PeakRSSBytes = readProcess().PeakRSSBytes
		r.Cases = cases
		if *steadyIdle > 0 {
			time.Sleep(3 * time.Second)
			quietBefore := readProcess()
			quietStart := time.Now()
			time.Sleep(*steadyIdle)
			r.SteadyIdleSeconds = time.Since(quietStart).Seconds()
			r.SteadyIdleCPUSeconds = readProcess().CPUSeconds - quietBefore.CPUSeconds
		}
		runtime.KeepAlive(a)
	default:
		panic("unknown phase")
	}
	info, err := os.Stat(path)
	must(err)
	r.CatalogBytes = info.Size()
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	must(encoder.Encode(r))
	for _, test := range r.Cases {
		if !test.Passed {
			os.Exit(1)
		}
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func openGzip(path string) (io.Reader, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	stream, err := gzip.NewReader(file)
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return stream, func() { stream.Close(); file.Close() }, nil
}
