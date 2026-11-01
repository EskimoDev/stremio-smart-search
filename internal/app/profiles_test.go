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
func TestNewSetupOffersStandardAndLocalAssets(t *testing.T) {
	a, err := New(context.Background(), t.TempDir(), "http://127.0.0.1:11479", "test")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status().Profile != "standard" || len(a.Status().Profiles) != 3 {
		t.Fatal("missing catalogue choices")
	}
	for _, path := range []string{"/fonts/geist.woff2", "/fonts/geist-mono.woff2", "/fonts/OFL.txt", "/apps/stremio.png", "/apps/dxbi.png", "/apps/enhanced.png"} {
		response := request(a, "GET", path, "", "")
		if response.Code != 200 || response.Body.Len() == 0 {
			t.Fatalf("local setup asset missing: %s", path)
		}
	}
}

func TestAlternativeTitlesBuildPersistAndRestore(t *testing.T) {
	a := testApp(t)
	if s := a.Status(); s.Language != "" || s.Aliases != 0 || len(s.Languages) == 0 {
		t.Fatalf("existing catalogue must start with alternative titles off: %+v", s)
	}
	var requested []catalog.AliasOptions
	a.DownloadCatalog = func(_ context.Context, profile catalog.Profile, _ catalog.Progress) (*catalog.Catalog, error) {
		c, err := catalog.Load("../../testdata/catalog.json")
		if err == nil {
			c.Profile = profile.ID
		}
		return c, err
	}
	a.DownloadAliases = func(_ context.Context, c *catalog.Catalog, options catalog.AliasOptions, _ catalog.Progress) error {
		requested = append(requested, options)
		for i := range c.Titles {
			if c.Titles[i].ID == "tt0816692" {
				c.Titles[i].Aliases = []string{"Interstellaire"}
			}
		}
		c.AliasLanguage, c.AliasRegion = options.Language, options.Region
		return nil
	}
	// An older settings page sends only a size: alternative titles stay off.
	if buildRequest(a, `{"profile":"compact"}`).Code != 202 {
		t.Fatal("build not accepted")
	}
	waitForBuild(t, a)
	if len(requested) != 0 {
		t.Fatal("alternative titles downloaded without being selected")
	}
	if response := buildRequest(a, `{"profile":"compact","language":"fr","region":"CA"}`); response.Code != 202 {
		t.Fatal(response.Body)
	}
	waitForBuild(t, a)
	if s := a.Status(); s.Language != "fr" || s.Region != "CA" || s.Aliases != 1 || s.Error != "" {
		t.Fatalf("selection not applied: %+v", s)
	}
	if response := request(a, "GET", "/catalog/movie/smart-search/search=interstellaire.json", "", ""); !strings.Contains(response.Body.String(), `"name":"Interstellar"`) {
		t.Fatalf("alias search failed or changed the display name: %s", response.Body)
	}
	// A size-only refresh keeps the active language.
	if buildRequest(a, `{"profile":"standard"}`).Code != 202 {
		t.Fatal("refresh not accepted")
	}
	waitForBuild(t, a)
	if len(requested) != 2 || requested[1] != (catalog.AliasOptions{Language: "fr", Region: "CA"}) || a.Status().Profile != "standard" {
		t.Fatalf("refresh dropped the language: %v %+v", requested, a.Status())
	}
	restored, err := New(context.Background(), a.dataDir, a.BaseURL, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Restore(); err != nil {
		t.Fatal(err)
	}
	if s := restored.Status(); s.Language != "fr" || s.Region != "CA" || s.Aliases != 1 {
		t.Fatalf("selection not restored with the catalogue: %+v", s)
	}
	// Explicitly turning it off.
	if buildRequest(a, `{"profile":"standard","language":"","region":""}`).Code != 202 {
		t.Fatal("off not accepted")
	}
	waitForBuild(t, a)
	if len(requested) != 2 || a.Status().Language != "" || a.Status().Aliases != 0 {
		t.Fatalf("turning alternative titles off failed: %+v", a.Status())
	}
}

func TestAlternativeTitleFailureKeepsPreviousCatalogue(t *testing.T) {
	a := testApp(t)
	a.DownloadCatalog = func(context.Context, catalog.Profile, catalog.Progress) (*catalog.Catalog, error) {
		return catalog.Load("../../testdata/catalog.json")
	}
	a.DownloadAliases = func(context.Context, *catalog.Catalog, catalog.AliasOptions, catalog.Progress) error {
		return errors.New("simulated alternative-title download failure")
	}
	if buildRequest(a, `{"profile":"expanded","language":"de"}`).Code != 202 {
		t.Fatal("build not accepted")
	}
	waitForBuild(t, a)
	if s := a.Status(); !s.Ready || s.Profile != "compact" || s.Language != "" || s.Error == "" || s.BuildLanguage != "" {
		t.Fatalf("previous catalogue lost: %+v", s)
	}
	if _, err := os.Stat(filepath.Join(a.dataDir, "catalog.json.gz")); !os.IsNotExist(err) {
		t.Fatal("failed build saved a snapshot")
	}
}

func TestBuildRejectsUnknownAlternativeTitleChoices(t *testing.T) {
	a := testApp(t)
	for _, body := range []string{`{"profile":"compact","language":"xx"}`, `{"profile":"compact","region":"FR"}`, `{"profile":"compact","language":"fr","region":"ES"}`, `{"profile":"compact","language":7}`} {
		if buildRequest(a, body).Code != 400 {
			t.Fatalf("invalid alternative-title choice accepted: %s", body)
		}
	}
	if a.Status().Building {
		t.Fatal("invalid input started a download")
	}
}
