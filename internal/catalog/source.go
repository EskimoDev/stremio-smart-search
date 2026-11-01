package catalog

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Source is the IMDb dataset server. Tests point BaseURL at a local server.
type Source struct {
	BaseURL string
	// Client is used for every request; nil means a client with a 30 minute
	// overall timeout, long enough for a slow full download.
	Client *http.Client
}

// IMDb is the real dataset server.
var IMDb = Source{BaseURL: "https://datasets.imdbws.com"}

const userAgent = "StremioSmartSearch/0.1 (+https://github.com/EskimoDev/stremio-smart-search)"

// SourceFile is what the server said about one dataset file when it was last
// downloaded: its validators and size. Either validator may be empty when the
// server does not send it.
type SourceFile struct {
	File         string `json:"file"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Size         int64  `json:"size,omitempty"`
}

// Freshness is the outcome of a metadata check.
type Freshness string

const (
	// Unchanged: every file matched its stored validator (HTTP 304, or the
	// same ETag or Last-Modified).
	Unchanged Freshness = "unchanged"
	// Changed: at least one file has a different validator.
	Changed Freshness = "changed"
	// Unknown: the server gave nothing to compare (no stored validator, no
	// validator in the reply, or HEAD is not supported). Callers fall back
	// to their time-based schedule.
	Unknown Freshness = "unknown"
)

// DatasetFiles lists the files a build with these options downloads.
func DatasetFiles(aliases AliasOptions) []string {
	files := []string{RatingsFile, BasicsFile}
	if aliases.Enabled() {
		files = append(files, AkasFile)
	}
	return files
}

func (s Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 30 * time.Minute}
}

func (s Source) url(file string) string { return strings.TrimRight(s.BaseURL, "/") + "/" + file }

func fileInfo(file string, h http.Header, size int64) SourceFile {
	return SourceFile{File: file, ETag: h.Get("ETag"), LastModified: h.Get("Last-Modified"), Size: max(size, 0)}
}

// Check asks the server, with one HEAD request per file and no body, whether
// the files changed since known was recorded. Stored validators are sent as
// If-None-Match and If-Modified-Since, and a 200 reply is compared too, so a
// server that ignores conditional headers still gives a correct answer.
func (s Source) Check(ctx context.Context, files []string, known []SourceFile) (Freshness, error) {
	byFile := map[string]SourceFile{}
	for _, f := range known {
		byFile[f.File] = f
	}
	result := Unchanged
	client := s.client()
	for _, file := range files {
		previous, ok := byFile[file]
		if !ok || (previous.ETag == "" && previous.LastModified == "") {
			result = Unknown // nothing to compare against; still check the rest for errors
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.url(file), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", userAgent)
		if previous.ETag != "" {
			req.Header.Set("If-None-Match", previous.ETag)
		}
		if previous.LastModified != "" {
			req.Header.Set("If-Modified-Since", previous.LastModified)
		}
		response, err := client.Do(req)
		if err != nil {
			return "", err
		}
		response.Body.Close()
		switch {
		case response.StatusCode == http.StatusNotModified:
			continue
		case response.StatusCode == http.StatusMethodNotAllowed || response.StatusCode == http.StatusNotImplemented:
			if result != Changed {
				result = Unknown
			}
			continue
		case response.StatusCode != http.StatusOK:
			return "", fmt.Errorf("IMDb returned HTTP %d while checking for changes", response.StatusCode)
		}
		current := fileInfo(file, response.Header, response.ContentLength)
		switch {
		case olderCopy(previous, current):
			// A CDN edge can still hold the previous day's file for a few
			// hours after IMDb publishes. A different ETag on an older file
			// is not news; never "update" backwards.
		case previous.ETag != "" && current.ETag != "":
			if current.ETag != previous.ETag {
				result = Changed
			}
		case previous.LastModified != "" && current.LastModified != "":
			if current.LastModified != previous.LastModified {
				result = Changed
			}
		default:
			if result != Changed {
				result = Unknown
			}
		}
	}
	return result, nil
}

// olderCopy reports a reply whose Last-Modified is not after the stored one.
func olderCopy(previous, current SourceFile) bool {
	if previous.LastModified == "" || current.LastModified == "" {
		return false
	}
	before, err1 := http.ParseTime(previous.LastModified)
	now, err2 := http.ParseTime(current.LastModified)
	return err1 == nil && err2 == nil && now.Before(before)
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

// open starts a full download of one file and records its validators.
func (s Source) open(ctx context.Context, file, stage string, maximum int64, progress Progress) (io.ReadCloser, SourceFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(file), nil)
	if err != nil {
		return nil, SourceFile{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	response, err := s.client().Do(req)
	if err != nil {
		return nil, SourceFile{}, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, SourceFile{}, fmt.Errorf("IMDb download returned HTTP %d; try again later", response.StatusCode)
	}
	if response.ContentLength > maximum {
		response.Body.Close()
		return nil, SourceFile{}, fmt.Errorf("dataset download exceeds its size limit")
	}
	info := fileInfo(file, response.Header, response.ContentLength)
	counted := &countingReader{reader: io.LimitReader(response.Body, maximum+1), stage: stage, progress: progress}
	z, err := gzip.NewReader(counted)
	if err != nil {
		response.Body.Close()
		return nil, SourceFile{}, err
	}
	return &datasetReader{Reader: z, body: response.Body, gzip: z, counted: counted, expected: response.ContentLength}, info, nil
}

// DownloadProfile streams ratings and titles and selects a catalogue.
func (s Source) DownloadProfile(ctx context.Context, profile Profile, progress Progress) (*Catalog, error) {
	if known, ok := ProfileByID(profile.ID); !ok || known != profile {
		return nil, fmt.Errorf("unknown catalogue profile")
	}
	if progress != nil {
		progress("Downloading title ratings", 0)
	}
	r, ratingsInfo, err := s.open(ctx, RatingsFile, "Downloading title ratings", 128<<20, progress)
	if err != nil {
		return nil, err
	}
	ratings, err := parseRatings(ctx, r, profile.MinVotes)
	if err == nil {
		err = r.(*datasetReader).complete()
	}
	_ = r.Close()
	if err != nil {
		return nil, err
	}
	if progress != nil {
		progress("Downloading and selecting movie and TV titles", 0)
	}
	b, basicsInfo, err := s.open(ctx, BasicsFile, "Downloading and selecting movie and TV titles", 768<<20, progress)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	c, err := parseBasics(ctx, b, ratings, profile)
	if err == nil {
		err = b.(*datasetReader).complete()
	}
	if err != nil {
		return nil, err
	}
	c.Sources = []SourceFile{ratingsInfo, basicsInfo}
	return c, nil
}

// DownloadAliases streams title.akas.tsv.gz into c; see DownloadIMDbAliases.
func (s Source) DownloadAliases(ctx context.Context, c *Catalog, options AliasOptions, progress Progress) error {
	if err := options.Validate(); err != nil || !options.Enabled() {
		return err
	}
	const stage = "Downloading alternative titles"
	if progress != nil {
		progress(stage, 0)
	}
	a, info, err := s.open(ctx, AkasFile, stage, 1536<<20, progress)
	if err != nil {
		return err
	}
	defer a.Close()
	if err := ParseIMDbAliases(ctx, a, c, options); err != nil {
		return err
	}
	if err := a.(*datasetReader).complete(); err != nil {
		return err
	}
	c.Sources = append(c.Sources, info)
	return nil
}

type datasetReader struct {
	io.Reader
	body     io.Closer
	gzip     io.Closer
	counted  *countingReader
	expected int64
}

// complete reports a download that ended early. The gzip trailer normally
// catches truncation, but a server can end a response cleanly at a gzip
// member boundary; a short body against a declared length is never accepted.
func (d *datasetReader) complete() error {
	// Drain what is left of the current member so the trailer is verified.
	if _, err := io.Copy(io.Discard, d.Reader); err != nil {
		return err
	}
	if d.expected > 0 && d.counted.total != d.expected {
		return fmt.Errorf("IMDb download ended early (%s of %s bytes); try again later", strconv.FormatInt(d.counted.total, 10), strconv.FormatInt(d.expected, 10))
	}
	return nil
}

func (d *datasetReader) Close() error { _ = d.gzip.Close(); return d.body.Close() }

// Head returns what the server currently says about one file.
func (s Source) Head(ctx context.Context, file string) (SourceFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.url(file), nil)
	if err != nil {
		return SourceFile{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	response, err := s.client().Do(req)
	if err != nil {
		return SourceFile{}, err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return SourceFile{}, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return fileInfo(file, response.Header, response.ContentLength), nil
}
