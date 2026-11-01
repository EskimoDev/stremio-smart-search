package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
)

func buildRequest(a *App, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11479/api/build", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:32100"
	request.Header.Set("X-Smart-Search-Key", a.Token)
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	return response
}
func waitForBuild(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for a.Status().Building && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if a.Status().Building {
		t.Fatal("catalogue build did not finish")
	}
}
func TestProfilesKeepServingUntilSuccessfulReplacement(t *testing.T) {
	a := testApp(t)
	if a.Status().Profile != "compact" {
		t.Fatal("legacy catalogue profile not migrated")
	}
	gate := make(chan struct{})
	entered := make(chan struct{})
	a.DownloadCatalog = func(_ context.Context, profile catalog.Profile, _ catalog.Progress) (*catalog.Catalog, error) {
		close(entered)
		<-gate
		c, err := catalog.Load("../../testdata/catalog.json")
		if err == nil {
			c.Profile = profile.ID
		}
		return c, err
	}
	if response := buildRequest(a, `{"profile":"expanded"}`); response.Code != 202 {
		t.Fatal(response.Body)
	}
	<-entered
	if s := a.Status(); !s.Ready || !s.Building || s.Profile != "compact" || s.BuildProfile != "expanded" {
		t.Fatalf("working selection lost: %+v", s)
	}
	if buildRequest(a, `{"profile":"standard"}`).Code != 409 {
		t.Fatal("overlapping build accepted")
	}
	if response := request(a, "GET", "/catalog/series/smart-search/search=HOTD.json", "", ""); response.Code != 200 || !strings.Contains(response.Body.String(), "tt11198330") {
		t.Fatal("working index unavailable")
	}
	close(gate)
	waitForBuild(t, a)
	if a.Status().Profile != "expanded" || a.Status().Error != "" {
		t.Fatalf("new profile not committed: %+v", a.Status())
	}
	restored, err := New(context.Background(), a.dataDir, a.BaseURL, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Restore(); err != nil {
		t.Fatal(err)
	}
	if restored.Status().Profile != "expanded" {
		t.Fatal("profile not restored with catalogue")
	}
}
func TestFailedProfileChangePreservesSnapshotAndIndex(t *testing.T) {
	a := testApp(t)
	c, err := catalog.Load("../../testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	c.Profile = "compact"
	path := filepath.Join(a.dataDir, "catalog.json.gz")
	if err := catalog.Save(path, c); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a.DownloadCatalog = func(context.Context, catalog.Profile, catalog.Progress) (*catalog.Catalog, error) {
		return nil, errors.New("simulated source failure")
	}
	if buildRequest(a, `{"profile":"expanded"}`).Code != 202 {
		t.Fatal("build not accepted")
	}
	waitForBuild(t, a)
	if s := a.Status(); !s.Ready || s.Profile != "compact" || s.Error == "" {
		t.Fatalf("previous catalogue lost: %+v", s)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed build changed persisted snapshot")
	}
	if response := request(a, "GET", "/catalog/series/smart-search/search=HOTD.json", "", ""); response.Code != 200 || !strings.Contains(response.Body.String(), "tt11198330") {
		t.Fatal("previous index unavailable after failure")
	}
}
func TestBuildRejectsUnknownProfilesAndParameters(t *testing.T) {
	a := testApp(t)
	for _, body := range []string{`{"profile":"unlimited"}`, `{"profile":42}`, `{"profile":"expanded","movies":99999999}`, `{"profile":"expanded"}{"profile":"compact"}`} {
		if buildRequest(a, body).Code != 400 {
			t.Fatalf("invalid profile accepted: %s", body)
		}
	}
	if a.Status().Building {
		t.Fatal("invalid input started a download")
	}
}
func TestNewSetupOffersStandardAndLocalFonts(t *testing.T) {
	a, err := New(context.Background(), t.TempDir(), "http://127.0.0.1:11479", "test")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status().Profile != "standard" || len(a.Status().Profiles) != 3 {
		t.Fatal("missing catalogue choices")
	}
	for _, path := range []string{"/fonts/geist.woff2", "/fonts/geist-mono.woff2", "/fonts/OFL.txt"} {
		response := request(a, "GET", path, "", "")
		if response.Code != 200 || response.Body.Len() == 0 {
			t.Fatalf("local font asset missing: %s", path)
		}
	}
}
