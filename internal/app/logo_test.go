package app

import (
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestManifestLogoUsesRequestOrigin(t *testing.T) {
	a := testApp(t)
	for _, origin := range []string{
		"http://127.0.0.1:23145",
		"http://localhost:32145",
		"http://[::1]:43145",
		"http://192.168.1.20:54145",
		"https://addon.example",
	} {
		t.Run(origin, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, origin+"/manifest.json", nil)
			r.Header.Set("Origin", "https://web.stremio.com")
			// An arbitrary forwarding header must not replace the request's origin.
			r.Header.Set("X-Forwarded-Host", "unrelated.example")
			r.Header.Set("X-Forwarded-Proto", "ftp")
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("manifest status %d: %s", w.Code, w.Body)
			}
			var manifest struct{ Logo string }
			if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
				t.Fatal(err)
			}
			logo, err := url.Parse(manifest.Logo)
			if err != nil || !logo.IsAbs() || logo.Host == "" || manifest.Logo != origin+"/addon-logo.png" {
				t.Fatalf("logo URL %q does not use addon origin %q", manifest.Logo, origin)
			}
		})
	}
}

func TestManifestLogoReachableBeforeSetup(t *testing.T) {
	a, err := New(context.Background(), t.TempDir(), "http://127.0.0.1:11479", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var manifest struct{ Logo string }
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Logo != server.URL+"/addon-logo.png" {
		t.Fatalf("manifest points outside the running helper: %q", manifest.Logo)
	}
	logo, err := server.Client().Get(manifest.Logo)
	if err != nil {
		t.Fatal(err)
	}
	defer logo.Body.Close()
	if logo.StatusCode != http.StatusOK || logo.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("logo response: status %d, content type %q", logo.StatusCode, logo.Header.Get("Content-Type"))
	}
	image, err := png.Decode(logo.Body)
	if err != nil {
		t.Fatalf("logo is not a valid PNG: %v", err)
	}
	if image.Bounds().Dx() != 256 || image.Bounds().Dy() != 256 {
		t.Fatalf("logo dimensions: %v", image.Bounds())
	}
	if a.Status().Ready {
		t.Fatal("logo must be available without preparing a catalogue")
	}
}

func TestAddonLogoPublicHTTPMethods(t *testing.T) {
	a := testApp(t)
	for _, tc := range []struct {
		method string
		status int
	}{
		{http.MethodGet, http.StatusOK},
		{http.MethodHead, http.StatusOK},
		{http.MethodOptions, http.StatusNoContent},
		{http.MethodPost, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.method, func(t *testing.T) {
			w := request(a, tc.method, "/addon-logo.png", "https://web.stremio.com", "")
			if w.Code != tc.status {
				t.Fatalf("logo status %d, want %d", w.Code, tc.status)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Methods") != "GET, HEAD, OPTIONS" {
				t.Fatal("public logo CORS missing")
			}
			if tc.method == http.MethodGet || tc.method == http.MethodHead {
				if w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "public, max-age=300" {
					t.Fatalf("unexpected logo headers: %v", w.Header())
				}
			}
			if (tc.method == http.MethodHead || tc.method == http.MethodOptions) && w.Body.Len() != 0 {
				t.Fatal("HEAD/OPTIONS must not return an image body")
			}
		})
	}
	// Public image access must not make settings or their assets public too.
	for _, path := range []string{"/", "/configure", "/icon.svg", "/api/status"} {
		w := request(a, http.MethodGet, path, "https://web.stremio.com", "")
		if w.Code != http.StatusForbidden || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("settings route %q became public: status %d", path, w.Code)
		}
	}
}
