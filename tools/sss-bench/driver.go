package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EskimoDev/stremio-smart-search/tools/sss-bench/probes/probekit"
)

// defaultAppPort is the installed helper's port. The harness never uses it.
const defaultAppPort = 11479

// searchBudget is the server's per-search time limit. A search reaching it may
// return fewer results, which is why it is counted rather than just timed.
const searchBudget = 100 * time.Millisecond

var (
	readyTimeout = 2 * time.Minute
	// shedMarker identifies a request refused because both search slots were busy.
	// The server answers those with HTTP 200 and an empty list, so without this
	// check they would count as very fast successful searches. A genuine empty
	// result carries a cache age of 60.
	shedMarker = []byte(`"cacheMaxAge":0`)
)

// procSample is one reading of another process's counters. Zero means the
// platform does not provide that counter.
type procSample struct {
	Resident, Private, Peak uint64
	CPU                     time.Duration
	HasCPU                  bool
}

// tail keeps the end of a child's stderr for error messages.
type tail struct {
	mu   sync.Mutex
	data []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, p...)
	if len(t.data) > 4096 {
		t.data = t.data[len(t.data)-4096:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.data))
}

// appStatus is the part of /api/status the harness checks.
type appStatus struct {
	Ready   bool   `json:"ready"`
	Version string `json:"version"`
	Movies  int    `json:"movies"`
	Series  int    `json:"series"`
	Aliases int    `json:"aliases"`
}

// instance is one running copy of the application on a private port and data
// folder. Its key is read from that folder, so a request can only ever succeed
// against the process this harness started.
type instance struct {
	cmd           *exec.Cmd
	pid           int
	base          string
	token         string
	client        *http.Client
	transport     *http.Transport
	stderr        *tail
	exited        chan error
	state         *os.ProcessState
	readyNS       int64
	firstSearchNS int64
	last          procSample
}

// freePort asks the OS for an unused loopback port.
func freePort() (int, error) {
	for attempt := 0; attempt < 16; attempt++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		if port != defaultAppPort {
			return port, nil
		}
	}
	return 0, errors.New("no free loopback port")
}

// startApp launches the application and waits until a search is answered. The
// port can be taken between choosing it and the child binding it, so a failed
// start is retried on a new port.
func startApp(ctx context.Context, binary, dataDir, catalogPath, firstSearch string, env []string) (*instance, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		port, err := freePort()
		if err != nil {
			return nil, err
		}
		a, err := launch(ctx, binary, dataDir, catalogPath, firstSearch, port, env)
		if err == nil {
			return a, nil
		}
		last = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, last
}

func launch(ctx context.Context, binary, dataDir, catalogPath, firstSearch string, port int, env []string) (*instance, error) {
	address := "127.0.0.1:" + strconv.Itoa(port)
	base := "http://" + address
	cmd := exec.CommandContext(ctx, binary, "--no-browser", "--listen", address, "--data-dir", dataDir, "--catalog", catalogPath)
	cmd.Env = env
	cmd.Dir = filepath.Dir(dataDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &tail{}
	cmd.Stderr = stderr
	transport := &http.Transport{DisableCompression: true, MaxIdleConns: 32, MaxIdleConnsPerHost: 32, IdleConnTimeout: 90 * time.Second}
	a := &instance{cmd: cmd, base: base, transport: transport, client: &http.Client{Transport: transport, Timeout: 30 * time.Second}, stderr: stderr, exited: make(chan error, 1)}
	started := probekit.Nanos()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	a.pid = cmd.Process.Pid
	ready := make(chan int64, 1)
	go func() {
		// Printed after the listener is bound and the catalogue loaded. Waiting on
		// this line rather than polling matters on Windows, where connecting to a
		// closed loopback port takes about two seconds to fail.
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), base) {
				ready <- probekit.Nanos()
			}
		}
		io.Copy(io.Discard, stdout)
	}()
	go func() {
		err := cmd.Wait()
		a.state = cmd.ProcessState
		a.exited <- err
	}()
	select {
	case at := <-ready:
		a.readyNS = at - started
	case err := <-a.exited:
		return nil, fmt.Errorf("application exited before it was ready: %v: %s", err, stderr.String())
	case <-time.After(readyTimeout):
		a.kill()
		return nil, fmt.Errorf("application was not ready within %s: %s", readyTimeout, stderr.String())
	case <-ctx.Done():
		a.kill()
		return nil, ctx.Err()
	}
	if _, _, err := a.get(a.searchPath("movie", firstSearch)); err != nil {
		a.kill()
		return nil, fmt.Errorf("first search failed: %w", err)
	}
	a.firstSearchNS = probekit.Nanos() - started
	key, err := os.ReadFile(filepath.Join(dataDir, "control.key"))
	if err != nil {
		a.kill()
		return nil, err
	}
	a.token = string(bytes.TrimSpace(key))
	return a, nil
}

func (a *instance) kill() {
	if a.cmd.Process == nil {
		return
	}
	a.cmd.Process.Kill()
	select {
	case <-a.exited:
	case <-time.After(5 * time.Second):
	}
}

func (a *instance) searchPath(kind, query string) string {
	return "/catalog/" + kind + "/smart-search/search=" + url.QueryEscape(query) + ".json"
}

func (a *instance) get(path string) (int, []byte, error) {
	response, err := a.client.Get(a.base + path)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

// status authenticates with this instance's key and reads what it has indexed.
func (a *instance) status() (appStatus, error) {
	var status appStatus
	request, err := http.NewRequest(http.MethodGet, a.base+"/api/status", nil)
	if err != nil {
		return status, err
	}
	request.Header.Set("X-Smart-Search-Key", a.token)
	response, err := a.client.Do(request)
	if err != nil {
		return status, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return status, fmt.Errorf("status returned HTTP %d", response.StatusCode)
	}
	return status, json.NewDecoder(response.Body).Decode(&status)
}

// warmConnection opens the keep-alive connection, so connection setup never
// lands inside a timed request.
func (a *instance) warmConnection() error {
	code, _, err := a.get("/manifest.json")
	if err == nil && code != http.StatusOK {
		err = fmt.Errorf("manifest returned HTTP %d", code)
	}
	return err
}

// stop asks the application to quit and reports a process that does not exit
// cleanly. It returns the final counters read just before quitting.
func (a *instance) stop() error {
	if final, err := readProcess(a.pid); err == nil {
		a.last = final
	}
	request, err := http.NewRequest(http.MethodPost, a.base+"/api/quit", nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-Smart-Search-Key", a.token)
	response, err := a.client.Do(request)
	if err != nil {
		a.kill()
		return fmt.Errorf("quit request failed: %w", err)
	}
	response.Body.Close()
	a.transport.CloseIdleConnections()
	select {
	case err := <-a.exited:
		if err != nil {
			return fmt.Errorf("application exited with an error: %v: %s", err, a.stderr.String())
		}
		return nil
	case <-time.After(10 * time.Second):
		a.kill()
		return errors.New("application did not stop within 10 seconds and was killed")
	}
}

// peak is the peak resident size known once the process has stopped.
func (a *instance) peak() uint64 {
	if a.state != nil {
		if peak := peakAtExit(a.state); peak > 0 {
			return peak
		}
	}
	return a.last.Peak
}

// outcome of one timed request.
type outcome int

const (
	served outcome = iota
	shed
	failed
)

// searcher issues timed searches on one connection. Its buffer is reused so the
// client allocates little during a measurement.
type searcher struct {
	client *http.Client
	base   string
	buffer bytes.Buffer
	body   []byte
}

func (a *instance) searcher() *searcher { return &searcher{client: a.client, base: a.base} }

// timed fetches one path. The clock covers the request and reading the whole
// body; building the request and inspecting the body are outside it.
func (s *searcher) timed(path string) (int64, outcome, error) {
	request, err := http.NewRequest(http.MethodGet, s.base+path, nil)
	if err != nil {
		return 0, failed, err
	}
	started := probekit.Nanos()
	response, err := s.client.Do(request)
	if err != nil {
		return 0, failed, err
	}
	s.buffer.Reset()
	_, err = s.buffer.ReadFrom(response.Body)
	elapsed := probekit.Nanos() - started
	response.Body.Close()
	s.body = s.buffer.Bytes()
	switch {
	case err != nil:
		return 0, failed, err
	case response.StatusCode != http.StatusOK:
		return 0, failed, fmt.Errorf("HTTP %d", response.StatusCode)
	case bytes.Contains(s.body, shedMarker):
		return elapsed, shed, nil
	}
	return elapsed, served, nil
}

// loadResult is what a closed-loop run produced. Only served requests count
// towards throughput and latency.
type loadResult struct {
	Latencies    []float64 // milliseconds
	Served, Shed int64
	Errors       int64
	FirstError   string
	Seconds      float64
}

// closedLoop runs clients that each issue the next request as soon as the last
// finished, until the deadline, in a fixed shuffled order per client.
func (a *instance) closedLoop(ctx context.Context, paths []string, clients int, duration time.Duration, seed uint64) loadResult {
	var result loadResult
	var mu sync.Mutex
	var servedCount, shedCount, errorCount atomic.Int64
	var workers sync.WaitGroup
	started := probekit.Nanos()
	deadline := started + duration.Nanoseconds()
	for client := 0; client < clients; client++ {
		workers.Add(1)
		go func(client int) {
			defer workers.Done()
			order := probekit.Order(len(paths), seed, 2000+client)
			s := a.searcher()
			latencies := make([]float64, 0, 4096)
			for i := 0; ctx.Err() == nil && probekit.Nanos() < deadline; i++ {
				ns, done, err := s.timed(paths[order[i%len(order)]])
				switch done {
				case served:
					servedCount.Add(1)
					latencies = append(latencies, float64(ns)/1e6)
				case shed:
					shedCount.Add(1)
				default:
					errorCount.Add(1)
					mu.Lock()
					if result.FirstError == "" && err != nil {
						result.FirstError = err.Error()
					}
					mu.Unlock()
				}
			}
			mu.Lock()
			result.Latencies = append(result.Latencies, latencies...)
			mu.Unlock()
		}(client)
	}
	workers.Wait()
	result.Seconds = float64(probekit.Nanos()-started) / 1e9
	result.Served, result.Shed, result.Errors = servedCount.Load(), shedCount.Load(), errorCount.Load()
	return result
}
