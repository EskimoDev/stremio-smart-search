package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func serverAnswering(t *testing.T, handler http.HandlerFunc) *instance {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &instance{base: server.URL, client: server.Client()}
}

func TestShedRequestIsNotASuccess(t *testing.T) {
	a := serverAnswering(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "busy"):
			w.Write([]byte(`{"cacheMaxAge":0,"metas":[]}`))
		case strings.Contains(r.URL.Path, "none"):
			w.Write([]byte(`{"cacheMaxAge":60,"metas":[]}`))
		case strings.Contains(r.URL.Path, "error"):
			http.Error(w, "no", http.StatusBadGateway)
		default:
			w.Write([]byte(`{"cacheMaxAge":60,"metas":[{"id":"tt1"}]}`))
		}
	})
	s := a.searcher()
	for path, expected := range map[string]outcome{"/busy": shed, "/none": served, "/fine": served, "/error": failed} {
		if _, got, _ := s.timed(path); got != expected {
			t.Errorf("%s: outcome %d, want %d", path, got, expected)
		}
	}
}

func TestTimerCoversTheWholeBody(t *testing.T) {
	a := serverAnswering(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"cacheMaxAge":60,`))
		w.(http.Flusher).Flush()
		time.Sleep(30 * time.Millisecond)
		w.Write([]byte(`"metas":[]}`))
	})
	ns, result, err := a.searcher().timed("/slow")
	if err != nil || result != served {
		t.Fatal(result, err)
	}
	if time.Duration(ns) < 25*time.Millisecond {
		t.Fatalf("timed %s: stopped before the body finished", time.Duration(ns))
	}
}

func TestClosedLoopExcludesShedRequests(t *testing.T) {
	var count atomic.Int64
	a := serverAnswering(t, func(w http.ResponseWriter, _ *http.Request) {
		if count.Add(1)%2 == 0 {
			w.Write([]byte(`{"cacheMaxAge":0,"metas":[]}`))
			return
		}
		w.Write([]byte(`{"cacheMaxAge":60,"metas":[{"id":"tt1"}]}`))
	})
	r := a.closedLoop(context.Background(), []string{"/a", "/b"}, 3, 80*time.Millisecond, 1)
	if r.Served == 0 || r.Shed == 0 || r.Errors != 0 {
		t.Fatalf("%+v", r)
	}
	if int64(len(r.Latencies)) != r.Served {
		t.Fatalf("%d latencies for %d served requests: shed requests must not be timed", len(r.Latencies), r.Served)
	}
}

func TestClosedLoopRecordsFailures(t *testing.T) {
	a := serverAnswering(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusInternalServerError) })
	r := a.closedLoop(context.Background(), []string{"/a"}, 1, 20*time.Millisecond, 1)
	if r.Errors == 0 || r.Served != 0 || !strings.Contains(r.FirstError, "500") {
		t.Fatalf("%+v", r)
	}
}

func TestClosedLoopStopsWhenCancelled(t *testing.T) {
	a := serverAnswering(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{}`)) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan loadResult, 1)
	go func() { done <- a.closedLoop(ctx, []string{"/a"}, 2, time.Hour, 1) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("did not stop")
	}
}

func TestFreePortIsNeverTheInstalledHelpersPort(t *testing.T) {
	for i := 0; i < 50; i++ {
		port, err := freePort()
		if err != nil {
			t.Fatal(err)
		}
		if port == defaultAppPort {
			t.Fatal("chose the default port")
		}
	}
}

func TestTailKeepsTheEnd(t *testing.T) {
	var buffer tail
	buffer.Write([]byte(strings.Repeat("x", 5000)))
	buffer.Write([]byte("end"))
	if got := buffer.String(); len(got) > 4096 || !strings.HasSuffix(got, "end") {
		t.Fatalf("%d bytes", len(got))
	}
}
