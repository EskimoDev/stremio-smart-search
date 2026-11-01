// Command catalogops is the sss-bench catalogue-operations probe. It parses
// local IMDb-shaped files, and runs a live refresh through the real settings API
// while searches continue.
//
// It is separate from the engine probe because it needs more of the application:
// if a source tree's App differs, only these measurements are lost. It replaces
// the download hooks, never starts the update schedule, and so never touches the
// network; download time is excluded from everything it reports.
package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/app"
	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/tools/sss-bench/probes/probekit"
)

func main() { probekit.Serve(handle) }

func handle(request probekit.OpsRequest) (probekit.OpsResponse, error) {
	request.Limits.Apply()
	switch request.Mode {
	case "prepare":
		return prepare(request)
	case "refresh":
		return refresh(request)
	}
	return probekit.OpsResponse{}, fmt.Errorf("unknown mode %q", request.Mode)
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

func parse(ctx context.Context, dir string, profile catalog.Profile) (*catalog.Catalog, error) {
	ratings, closeRatings, err := openGzip(filepath.Join(dir, "title.ratings.tsv.gz"))
	if err != nil {
		return nil, err
	}
	defer closeRatings()
	basics, closeBasics, err := openGzip(filepath.Join(dir, "title.basics.tsv.gz"))
	if err != nil {
		return nil, err
	}
	defer closeBasics()
	return catalog.ParseIMDbProfile(ctx, ratings, basics, profile)
}

func prepare(request probekit.OpsRequest) (probekit.OpsResponse, error) {
	var r probekit.OpsResponse
	profile, ok := catalog.ProfileByID(request.Profile)
	if !ok {
		return r, fmt.Errorf("unknown catalogue profile %q", request.Profile)
	}
	cpu := probekit.ProcessCPU()
	started := probekit.Nanos()
	c, err := parse(context.Background(), request.IMDbDir, profile)
	if err != nil {
		return r, err
	}
	r.ParseNS = probekit.Nanos() - started
	r.ParseCPUNS = int64(probekit.ProcessCPU() - cpu)
	r.Titles = len(c.Titles)
	started = probekit.Nanos()
	if err := catalog.Save(request.Output, c); err != nil {
		return r, err
	}
	r.SaveNS = probekit.Nanos() - started
	info, err := os.Stat(request.Output)
	if err != nil {
		return r, err
	}
	r.FileBytes = info.Size()
	r.PeakResident = probekit.PeakResident()
	return r, nil
}

func refresh(request probekit.OpsRequest) (probekit.OpsResponse, error) {
	var r probekit.OpsResponse
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return r, err
	}
	base := "http://" + listener.Addr().String()
	ctx := context.Background()
	a, err := app.New(ctx, request.DataDir, base, "sss-bench")
	if err != nil {
		return r, err
	}
	if err := a.Load(request.Catalog); err != nil {
		return r, err
	}
	a.DownloadCatalog = func(ctx context.Context, profile catalog.Profile, _ catalog.Progress) (*catalog.Catalog, error) {
		return parse(ctx, request.IMDbDir, profile)
	}
	a.DownloadAliases = func(context.Context, *catalog.Catalog, catalog.AliasOptions, catalog.Progress) error {
		return errors.New("sss-bench refreshes never download")
	}
	server := &http.Server{Handler: a.Handler()}
	go server.Serve(listener)
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	// Alternative titles are switched off: their source would need title.akas.
	body := fmt.Sprintf(`{"profile":%q,"language":""}`, request.Profile)
	build, err := http.NewRequest(http.MethodPost, base+"/api/build", strings.NewReader(body))
	if err != nil {
		return r, err
	}
	build.Header.Set("X-Smart-Search-Key", a.Token)
	cpu := probekit.ProcessCPU()
	started := probekit.Nanos()
	response, err := client.Do(build)
	if err != nil {
		return r, err
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return r, fmt.Errorf("refresh was not accepted: HTTP %d", response.StatusCode)
	}
	address := base + "/catalog/" + request.Kind + "/smart-search/search=" + url.QueryEscape(request.Query) + ".json"
	timeout := time.Duration(request.TimeoutSeconds) * time.Second
	for a.Status().Building {
		metas, err := searchCount(client, address)
		if err != nil {
			return r, err
		}
		if metas > 0 {
			r.SearchesServed++
		} else {
			r.SearchesEmpty++
		}
		if time.Duration(probekit.Nanos()-started) > timeout {
			return r, fmt.Errorf("refresh exceeded %s", timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
	r.RefreshNS = probekit.Nanos() - started
	r.RefreshCPUNS = int64(probekit.ProcessCPU() - cpu)
	status := a.Status()
	if status.Error != "" {
		return r, errors.New(status.Error)
	}
	r.Titles = status.Movies + status.Series
	r.PeakResident = probekit.PeakResident()
	return r, nil
}

func searchCount(client *http.Client, address string) (int, error) {
	response, err := client.Get(address)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("search returned HTTP %d during a refresh", response.StatusCode)
	}
	var body struct {
		Metas []json.RawMessage `json:"metas"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return 0, err
	}
	return len(body.Metas), nil
}
