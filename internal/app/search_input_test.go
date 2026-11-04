package app

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/EskimoDev/stremio-smart-search/internal/catalog"
	"github.com/EskimoDev/stremio-smart-search/internal/search"
)

func TestSearchInputFixesThroughAddonRoute(t *testing.T) {
	long := "Don't Be a Menace to South Central While Drinking Your Juice in the Hood"
	a := testApp(t)
	a.engine.Store(search.New([]catalog.Title{
		{ID: "tt0116126", Name: long, Type: "movie", Year: 1996, Votes: 68000},
		{ID: "tt0816692", Name: "Interstellar", Type: "movie", Year: 2014, Votes: 2000000},
		{ID: "tt0944947", Name: "Game of Thrones", Type: "series", Year: 2011, Votes: 2500000},
		{ID: "tt0000001", Name: strings.Repeat("x", 513), Type: "movie"},
	}))
	for _, tc := range []struct{ query, kind, id string }{
		{long, "movie", "tt0116126"},
		{"Ｉｎｔｅｒｓｔｅｌｌａｒ", "movie", "tt0816692"},
		{"ＧＯＴ ٢٠١١", "series", "tt0944947"},
		{"got 2011", "movie", ""},
		{"got 2012", "series", ""},
		{strings.Repeat("x", 513), "movie", ""},
	} {
		t.Run(tc.query+"/"+tc.kind, func(t *testing.T) {
			w := request(a, "GET", "/catalog/"+tc.kind+"/smart-search/search="+url.QueryEscape(tc.query)+".json", "https://web.stremio.com", "")
			var body struct{ Metas []struct{ ID, Type string } }
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil {
				t.Fatalf("invalid addon response: %d %s", w.Code, w.Body)
			}
			if tc.id == "" {
				if len(body.Metas) != 0 {
					t.Fatalf("unexpected results: %s", w.Body)
				}
			} else if len(body.Metas) == 0 || body.Metas[0].ID != tc.id || body.Metas[0].Type != tc.kind {
				t.Fatalf("wanted %s first: %s", tc.id, w.Body)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Fatal("addon CORS header missing")
			}
		})
	}
}
