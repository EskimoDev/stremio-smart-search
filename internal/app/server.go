package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/search"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed web/*
var web embed.FS

const Product = "Stremio Smart Search"
const DefaultPort = "11479"

type Status struct {
	Product      string            `json:"product"`
	Version      string            `json:"version"`
	Profile      string            `json:"profile"`
	BuildProfile string            `json:"buildProfile,omitempty"`
	Profiles     []catalog.Profile `json:"profiles"`
	// Alternative-title selection of the active catalogue, the one being built,
	// and the choices offered. Empty language means alternative titles are off.
	Language        string             `json:"language"`
	Region          string             `json:"region"`
	BuildLanguage   string             `json:"buildLanguage,omitempty"`
	BuildRegion     string             `json:"buildRegion,omitempty"`
	Languages       []catalog.Language `json:"languages"`
	Aliases         int                `json:"aliases"`
	Ready           bool               `json:"ready"`
	Building        bool               `json:"building"`
	Stage           string             `json:"stage"`
	Bytes           int64              `json:"bytes"`
	Movies          int                `json:"movies"`
	Series          int                `json:"series"`
	BuiltAt         time.Time          `json:"builtAt"`
	Error           string             `json:"error,omitempty"`
	Manifest        string             `json:"manifest"`
	WebInstall      string             `json:"webInstall"`
	PreferredClient string             `json:"preferredClient"`
}
type App struct {
	ctx             context.Context
	dataDir         string
	Token           string
	BaseURL         string
	mu              sync.Mutex
	status          Status
	engine          atomic.Pointer[search.Engine]
	searchSlots     chan struct{}
	Quit            func()
	LaunchClient    func(string) error
	DownloadCatalog func(context.Context, catalog.Profile, catalog.Progress) (*catalog.Catalog, error)
	// DownloadAliases adds alternative titles to a freshly built catalogue. It
	// is only called when the build selected a language.
	DownloadAliases func(context.Context, *catalog.Catalog, catalog.AliasOptions, catalog.Progress) error
}

func New(ctx context.Context, dataDir, baseURL, version string) (*App, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	tokenPath := filepath.Join(dataDir, "control.key")
	token, err := controlToken(tokenPath)
	if err != nil {
		return nil, err
	}
	a := &App{ctx: ctx, dataDir: dataDir, Token: string(token), BaseURL: baseURL, searchSlots: make(chan struct{}, 2)}
	a.LaunchClient = OpenClient
	a.DownloadCatalog = catalog.DownloadIMDbProfile
	a.DownloadAliases = catalog.DownloadIMDbAliases
	a.status = Status{Product: Product, Version: version, Profile: catalog.DefaultProfile, Profiles: catalog.Profiles(), Languages: catalog.Languages(), Stage: "Set up your local catalogue", Manifest: baseURL + "/manifest.json", WebInstall: "https://web.stremio.com/#/addons?addon=" + url.QueryEscape(baseURL+"/manifest.json"), PreferredClient: loadClientPreference(dataDir)}
	return a, nil
}

func controlToken(path string) ([]byte, error) {
	for attempt := 0; attempt < 20; attempt++ {
		token, err := os.ReadFile(path)
		if err == nil {
			decoded, decodeErr := hex.DecodeString(string(token))
			if decodeErr == nil && len(decoded) == 32 {
				return token, nil
			}
			// A simultaneous first launch may have created but not yet written it.
			if len(token) != 0 {
				return nil, fmt.Errorf("invalid local control key")
			}
		} else if os.IsNotExist(err) {
			bytes := make([]byte, 32)
			if _, err = rand.Read(bytes); err != nil {
				return nil, err
			}
			f, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if createErr == nil {
				token = []byte(hex.EncodeToString(bytes))
				_, err = f.Write(token)
				closeErr := f.Close()
				if err != nil {
					return nil, err
				}
				return token, closeErr
			}
			if !os.IsExist(createErr) {
				return nil, createErr
			}
		} else {
			return nil, err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, fmt.Errorf("local control key was not ready; reopen Smart Search")
}
func (a *App) Status() Status { a.mu.Lock(); defer a.mu.Unlock(); return a.status }
func (a *App) setCatalog(c *catalog.Catalog) {
	engine := search.New(c.Titles)
	a.engine.Store(engine)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.Ready = true
	a.status.Building = false
	a.status.BuildProfile = ""
	a.status.BuildLanguage, a.status.BuildRegion = "", ""
	a.status.Language, a.status.Region = c.AliasLanguage, c.AliasRegion
	a.status.Aliases = engine.AlternateTitles()
	a.status.Profile = c.Profile
	if a.status.Profile == "" {
		a.status.Profile = "compact" // Catalogues made before selectable profiles.
	}
	a.status.Stage = "Ready to search"
	a.status.Bytes = 0
	a.status.BuiltAt = c.BuiltAt
	a.status.Error = ""
	a.status.Movies = 0
	a.status.Series = 0
	for _, t := range c.Titles {
		if t.Type == "movie" {
			a.status.Movies++
		} else {
			a.status.Series++
		}
	}
}
func (a *App) Load(path string) error {
	c, err := catalog.Load(path)
	if err != nil {
		return err
	}
	a.setCatalog(c)
	// Decoding the snapshot leaves a parsed copy of every title behind once
	// the index is built. Collect it and return it to the OS once, so startup
	// memory reflects the index rather than the loader. This runs once per
	// load, never per search.
	c = nil
	debug.FreeOSMemory()
	return nil
}
func (a *App) Restore() error {
	path := filepath.Join(a.dataDir, "catalog.json.gz")
	err := a.Load(path)
	if err != nil {
		if backupErr := a.Load(path + ".previous"); backupErr == nil {
			return nil
		}
	}
	return err
}
func (a *App) startBuild(profile catalog.Profile, aliases catalog.AliasOptions) bool {
	a.mu.Lock()
	if a.status.Building {
		a.mu.Unlock()
		return false
	}
	a.status.Building = true
	a.status.BuildProfile = profile.ID
	a.status.BuildLanguage, a.status.BuildRegion = aliases.Language, aliases.Region
	a.status.Error = ""
	a.status.Stage = "Connecting to IMDb"
	a.status.Bytes = 0
	a.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 45*time.Minute)
		defer cancel()
		progress := func(stage string, bytes int64) {
			a.mu.Lock()
			a.status.Stage = stage
			a.status.Bytes = bytes
			a.mu.Unlock()
		}
		c, err := a.DownloadCatalog(ctx, profile, progress)
		if err == nil && aliases.Enabled() {
			err = a.DownloadAliases(ctx, c, aliases, progress)
		}
		if err == nil {
			err = catalog.Save(filepath.Join(a.dataDir, "catalog.json.gz"), c)
		}
		if err == nil {
			a.mu.Lock()
			a.status.Stage = "Preparing fast local search"
			a.mu.Unlock()
			a.setCatalog(c)
			// Return transient parsing memory after an explicit large download.
			// This is not a timer or a per-search garbage collection request.
			debug.FreeOSMemory()
			return
		}
		a.mu.Lock()
		a.status.Building = false
		a.status.BuildProfile = ""
		a.status.BuildLanguage, a.status.BuildRegion = "", ""
		a.status.Stage = "Catalogue update did not finish"
		a.status.Error = err.Error()
		a.mu.Unlock()
	}()
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (a *App) validLocalHost(host string) bool {
	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	base, _ := url.Parse(a.BaseURL)
	_, wantedPort, _ := net.SplitHostPort(base.Host)
	return port == wantedPort && (hostname == "127.0.0.1" || hostname == "localhost" || hostname == "::1")
}
func (a *App) localRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() || !a.validLocalHost(r.Host) {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "http" || u.Host != r.Host {
			return false
		}
	}
	return true
}
func (a *App) admin(w http.ResponseWriter, r *http.Request) bool {
	if !a.localRequest(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Smart-Search-Key")), []byte(a.Token)) != 1 {
		writeJSON(w, 403, map[string]string{"error": "Open the local settings page to manage the addon."})
		return false
	}
	return true
}
func (a *App) Handler() http.Handler {
	assets, _ := fs.Sub(web, "web")
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		public := r.URL.Path == "/manifest.json" || strings.HasPrefix(r.URL.Path, "/catalog/")
		if public {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(204)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				writeJSON(w, 405, map[string]string{"error": "Use GET"})
				return
			}
			if r.URL.Path == "/manifest.json" {
				a.manifest(w)
				return
			}
			a.catalog(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/health" && r.Method == http.MethodGet {
			writeJSON(w, 200, map[string]string{"product": Product, "version": a.Status().Version})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if !a.admin(w, r) {
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			switch {
			case r.URL.Path == "/api/status" && r.Method == http.MethodGet:
				writeJSON(w, 200, a.Status())
			case r.URL.Path == "/api/clients" && r.Method == http.MethodGet:
				writeJSON(w, 200, map[string]any{"clients": InstalledClients()})
			case r.URL.Path == "/api/install" && r.Method == http.MethodPost:
				var selection struct {
					Client string `json:"client"`
				}
				decoder := json.NewDecoder(r.Body)
				decoder.DisallowUnknownFields()
				if decoder.Decode(&selection) != nil || !validClient(selection.Client) {
					writeJSON(w, 400, map[string]string{"error": "Choose Stremio, dx.bi or Stremio Enhanced."})
					return
				}
				if !a.Status().Ready {
					writeJSON(w, 409, map[string]string{"error": "Prepare the local catalogue first."})
					return
				}
				if err := a.LaunchClient(selection.Client); err != nil {
					code := 500
					message := "Could not open the selected app. Copy the addon link and open it manually."
					if errors.Is(err, ErrClientNotFound) {
						code = 404
						message = clientNames[selection.Client] + " was not found. Copy the addon link and open the app manually."
					}
					writeJSON(w, code, map[string]string{"error": message})
					return
				}
				if err := a.rememberClient(selection.Client); err != nil {
					writeJSON(w, 500, map[string]string{"error": "The app opened, but its selection could not be saved."})
					return
				}
				writeJSON(w, 200, map[string]string{"client": selection.Client, "name": clientNames[selection.Client], "manifest": a.Status().Manifest})
			case r.URL.Path == "/api/build" && r.Method == http.MethodPost:
				// Omitted fields keep the active selection, so an older settings
				// page that only sends a size never turns alternative titles off.
				current := a.Status()
				selection := struct {
					Profile  string  `json:"profile"`
					Language *string `json:"language"`
					Region   *string `json:"region"`
				}{Profile: current.Profile}
				decoder := json.NewDecoder(r.Body)
				decoder.DisallowUnknownFields()
				err := decoder.Decode(&selection)
				if err != nil && err != io.EOF {
					writeJSON(w, 400, map[string]string{"error": "Choose a catalogue size: Compact, Standard or Expanded."})
					return
				}
				if err == nil {
					var extra any
					if decoder.Decode(&extra) != io.EOF {
						writeJSON(w, 400, map[string]string{"error": "Send one catalogue size selection."})
						return
					}
				}
				profile, ok := catalog.ProfileByID(selection.Profile)
				if !ok {
					writeJSON(w, 400, map[string]string{"error": "Choose a catalogue size: Compact, Standard or Expanded."})
					return
				}
				aliases := catalog.AliasOptions{Language: current.Language, Region: current.Region}
				if aliases.Validate() != nil {
					aliases = catalog.AliasOptions{} // e.g. a snapshot from a newer build
				}
				if selection.Language != nil {
					aliases = catalog.AliasOptions{Language: *selection.Language}
				}
				if selection.Region != nil {
					aliases.Region = *selection.Region
				}
				if aliases.Validate() != nil {
					writeJSON(w, 400, map[string]string{"error": "Choose a listed language and region for alternative titles."})
					return
				}
				if !a.startBuild(profile, aliases) {
					writeJSON(w, 409, map[string]string{"error": "A catalogue update is already running."})
					return
				}
				writeJSON(w, 202, a.Status())
			case r.URL.Path == "/api/launch" && r.Method == http.MethodPost:
				if err := a.LaunchClient(a.Status().PreferredClient); err != nil {
					writeJSON(w, 500, map[string]string{"error": "Could not open Stremio. Open it normally, then add the copied addon link."})
					return
				}
				writeJSON(w, 200, map[string]bool{"ok": true})
			case r.URL.Path == "/api/quit" && r.Method == http.MethodPost:
				writeJSON(w, 200, map[string]bool{"ok": true})
				if a.Quit != nil {
					go a.Quit()
				}
			default:
				http.NotFound(w, r)
			}
			return
		}
		if !a.localRequest(r) {
			http.Error(w, "Settings are available on the host computer only.", 403)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Use GET", 405)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'nonce-"+a.Token+"'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.URL.Path == "/" {
			html, _ := web.ReadFile("web/index.html")
			bootstrap, _ := json.Marshal(map[string]string{"token": a.Token, "baseURL": a.BaseURL})
			page := strings.Replace(string(html), "<!--BOOTSTRAP-->", "<script nonce=\""+a.Token+"\">window.smartSearch="+string(bootstrap)+";</script>", 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(page))
			return
		}
		switch r.URL.Path {
		case "/app.js", "/style.css", "/icon.svg", "/fonts/geist.woff2", "/fonts/geist-mono.woff2", "/fonts/OFL.txt",
			"/apps/stremio.png", "/apps/dxbi.png", "/apps/enhanced.png":
			files.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
func (a *App) manifest(w http.ResponseWriter) {
	catalogs := []any{}
	for _, kind := range []string{"movie", "series"} {
		catalogs = append(catalogs, map[string]any{"id": "smart-search", "type": kind, "name": "Smart Search", "extra": []any{map[string]any{"name": "search", "isRequired": true}}})
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, 200, map[string]any{"id": "community.eskimodev.smartsearch", "version": strings.TrimPrefix(a.Status().Version, "v"), "name": Product, "description": "Fast local title matching with typo and abbreviation support. Recommended: move Smart Search to the top of your addon order. Requires the Smart Search companion on this computer.", "resources": []string{"catalog"}, "types": []string{"movie", "series"}, "catalogs": catalogs, "behaviorHints": map[string]bool{"adult": false, "p2p": false}})
}
func (a *App) catalog(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), "/catalog/"), "/")
	if len(parts) != 3 || (parts[0] != "movie" && parts[0] != "series") || parts[1] != "smart-search" || !strings.HasSuffix(parts[2], ".json") {
		http.NotFound(w, r)
		return
	}
	// Use the escaped path so percent signs, ampersands and slashes inside a
	// search value are decoded exactly once by ParseQuery, never as path separators.
	extra, err := url.ParseQuery(strings.TrimSuffix(parts[2], ".json"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid search query"})
		return
	}
	query := extra.Get("search")
	if len(query) > 512 || extra.Get("skip") != "" && extra.Get("skip") != "0" {
		writeJSON(w, 200, map[string]any{"metas": []any{}})
		return
	}
	engine := a.engine.Load()
	if engine == nil {
		writeJSON(w, 200, map[string]any{"metas": []any{}, "cacheMaxAge": 0})
		return
	}
	select {
	case a.searchSlots <- struct{}{}:
		defer func() { <-a.searchSlots }()
	default:
		writeJSON(w, 200, map[string]any{"metas": []any{}, "cacheMaxAge": 0})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
	defer cancel()
	titles := engine.Search(ctx, query, parts[0], 20)
	metas := make([]map[string]any, 0, len(titles))
	for _, t := range titles {
		m := map[string]any{"id": t.ID, "type": t.Type, "name": t.Name, "poster": "https://images.metahub.space/poster/medium/" + t.ID + "/img", "posterShape": "poster"}
		if t.Year > 0 {
			m["releaseInfo"] = fmt.Sprint(t.Year)
		}
		if t.Rating > 0 {
			m["imdbRating"] = fmt.Sprintf("%.1f", t.Rating)
		}
		metas = append(metas, m)
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, 200, map[string]any{"metas": metas, "cacheMaxAge": 60})
}
