package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestSimultaneousFirstLaunchesShareControlKey(t *testing.T) {
	dir := t.TempDir()
	keys := make(chan string, 24)
	errs := make(chan error, 24)
	var group sync.WaitGroup
	for i := 0; i < 24; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			a, err := New(context.Background(), dir, "http://127.0.0.1:11479", "0.1.0")
			if err != nil {
				errs <- err
				return
			}
			keys <- a.Token
		}()
	}
	group.Wait()
	close(keys)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	first := ""
	for key := range keys {
		if first == "" {
			first = key
		}
		if key != first {
			t.Fatal("first launches generated incompatible keys")
		}
	}
}

func testApp(t *testing.T) *App {
	t.Helper()
	a, err := New(context.Background(), t.TempDir(), "http://127.0.0.1:11479", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Load("../../testdata/catalog.json"); err != nil {
		t.Fatal(err)
	}
	return a
}
func request(a *App, method, path, origin, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:11479"+path, nil)
	r.RemoteAddr = "127.0.0.1:32100"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	r.Header.Set("X-Smart-Search-Key", token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}
func TestSearchAddonProtocolAndEncodedPunctuation(t *testing.T) {
	a := testApp(t)
	for _, tc := range []struct{ query, kind, id string }{{"HOTD", "series", "tt11198330"}, {"interstelar", "movie", "tt0816692"}, {"A/B & 100%", "movie", "tt9999901"}} {
		w := request(a, "GET", "/catalog/"+tc.kind+"/smart-search/search="+url.QueryEscape(tc.query)+".json", "https://web.stremio.com", "")
		if w.Code != 200 {
			t.Fatalf("search status %d: %s", w.Code, w.Body)
		}
		var body struct {
			Metas []struct{ ID, Name, Type, Poster string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Metas) == 0 || body.Metas[0].ID != tc.id || body.Metas[0].Type != tc.kind || !strings.HasPrefix(body.Metas[0].Poster, "https://") {
			t.Fatalf("invalid Stremio result: %s", w.Body)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatal("Stremio CORS missing")
		}
	}
}
func TestManifestHasSearchOnlyCatalogs(t *testing.T) {
	w := request(testApp(t), "GET", "/manifest.json", "https://web.stremio.com", "")
	var body struct {
		Catalogs []struct {
			Type  string
			Extra []struct {
				Name       string
				IsRequired bool
			}
		}
		Resources []string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Catalogs) != 2 || len(body.Resources) != 1 || body.Resources[0] != "catalog" {
		t.Fatal("unexpected capabilities")
	}
	for _, c := range body.Catalogs {
		if len(c.Extra) != 1 || c.Extra[0].Name != "search" || !c.Extra[0].IsRequired {
			t.Fatal("catalog should require search")
		}
	}
}
func TestBrowserCannotControlLocalCompanion(t *testing.T) {
	a := testApp(t)
	for _, tc := range []struct{ path, origin, key string }{{"/api/build", "https://unrelated.example", ""}, {"/api/build", "https://unrelated.example", a.Token}, {"/api/quit", "", "wrong"}, {"/api/status", "", ""}} {
		if got := request(a, "POST", tc.path, tc.origin, tc.key); got.Code != 403 {
			t.Fatalf("admin request accepted: %d", got.Code)
		}
	}
	w := request(a, http.MethodGet, "/api/status", "http://127.0.0.1:11479", a.Token)
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("local settings access/CORS failed")
	}
}
func TestSettingsRejectRemoteAccessAndDNSRebinding(t *testing.T) {
	a := testApp(t)
	for _, tc := range []struct{ host, remote string }{{"unrelated.example:11479", "127.0.0.1:32"}, {"127.0.0.1:11479", "192.168.1.4:32"}} {
		r := httptest.NewRequest("GET", "http://"+tc.host+"/", nil)
		r.RemoteAddr = tc.remote
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 403 || strings.Contains(w.Body.String(), a.Token) {
			t.Fatal("local control key exposed")
		}
	}
}
func TestConfigureOpensSettings(t *testing.T) {
	a := testApp(t)
	var body struct{ BehaviorHints map[string]bool }
	if err := json.Unmarshal(request(a, "GET", "/manifest.json", "", "").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.BehaviorHints["configurable"] || body.BehaviorHints["configurationRequired"] {
		t.Fatal("manifest should offer an optional Configure button")
	}
	w := request(a, "GET", "/configure", "", "")
	if w.Code != 302 || w.Header().Get("Location") != "/" {
		t.Fatalf("configure status %d location %q", w.Code, w.Header().Get("Location"))
	}
	r := httptest.NewRequest("GET", "http://192.168.1.20:11479/configure", nil)
	r.RemoteAddr = "192.168.1.30:40000"
	remote := httptest.NewRecorder()
	a.Handler().ServeHTTP(remote, r)
	if remote.Code != 403 {
		t.Fatalf("remote configure status %d", remote.Code)
	}
}
