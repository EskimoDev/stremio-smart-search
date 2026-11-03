package catalog

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

// decodeCatalog must give exactly what encoding/json gives: the same value,
// and an error exactly when encoding/json returns one.
func checkDecode(t *testing.T, doc string) {
	t.Helper()
	var want, got Catalog
	wantErr := json.NewDecoder(strings.NewReader(doc)).Decode(&want)
	gotErr := decodeCatalog([]byte(doc), &got)
	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("%.80q: error %v, encoding/json %v", doc, gotErr, wantErr)
	}
	if wantErr == nil && !reflect.DeepEqual(want, got) {
		t.Fatalf("%.80q: decoded\n%+v\nencoding/json\n%+v", doc, got, want)
	}
}

func TestDecodeCatalogMatchesEncodingJSON(t *testing.T) {
	title := `{"id":"tt1","name":"A","type":"movie","year":2001,"votes":5,"rating":7.5}`
	docs := []string{
		`{"version":1,"titles":[` + title + `]}`,
		` { "version" : 1 , "profile":"standard", "aliasLanguage":"fr","aliasRegion":"FR","builtAt":"2026-11-01T06:19:46.9294884Z","source":"x","titles" : [ ` + title + ` , {"id":"tt2","name":"B","type":"series"} ] } trailing`,
		`{"version":1,"titles":[{"id":"tt1","name":"Fast & Furious \"x\" \\ \/ \n\t\b\f\r","type":"movie"}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"Amélie 日本 é","original":"O","type":"movie","aliases":["a","b < c"]}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","aliases":[]}]}`,
		`{"version":1,"titles":[]}`,
		`{"version":1,"titles":null}`,
		`{"version":1,"titles":[{"id":"tt1","name":null,"type":"movie"}]}`,
		`{"version":1,"titles":[{"ID":"tt1","Name":"case","type":"movie"}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","extra":{"a":[1,2]}}]}`,
		`{"version":1,"unknown":true,"titles":[]}`,
		`{"version":1,"titles":[{"id":"tt1","id":"tt2","name":"dup","type":"movie"}]}`,
		`{"version":1,"titles":[],"titles":[` + title + `]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"😀 pair","type":"movie"}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"\ud83d lone","type":"movie"}]}`,
		"{\"version\":1,\"titles\":[{\"id\":\"tt1\",\"name\":\"bad \xff utf8\",\"type\":\"movie\"}]}",
		"{\"version\":1,\"titles\":[{\"id\":\"tt1\",\"name\":\"ctl \x01\",\"type\":\"movie\"}]}",
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","year":2001.0}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","year":2e3}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","year":-0,"votes":99999999999999999999}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","rating":1e400}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","rating":01}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","rating":.5}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie","rating":-1.25e-2}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie"},]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"x","type":"movie"}`,
		`{"version":1,"builtAt":"not a time","titles":[]}`,
		`{"version":"1","titles":[]}`,
		`[]`,
		``,
		`{"version":1,"titles":[{"id":"tt1","name":"\x","type":"movie"}]}`,
		`{"version":1,"titles":[{"id":"tt1","name":"\u12","type":"movie"}]}`,
	}
	for _, doc := range docs {
		checkDecode(t, doc)
	}
	// Every prefix of a valid document, which exercises each error path.
	full := docs[1]
	for i := range full {
		checkDecode(t, full[:i])
	}
}

// SMART_SEARCH_DECODE_SNAPSHOTS lists local snapshots (outside the
// repository) to compare in full.
func TestDecodeCatalogSnapshots(t *testing.T) {
	paths := os.Getenv("SMART_SEARCH_DECODE_SNAPSHOTS")
	if paths == "" {
		t.Skip("set SMART_SEARCH_DECODE_SNAPSHOTS to compare real snapshots")
	}
	for _, path := range strings.Split(paths, string(os.PathListSeparator)) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		z, err := gzip.NewReader(f)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(z)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		var want, got Catalog
		if err := json.NewDecoder(bytes.NewReader(data)).Decode(&want); err != nil {
			t.Fatal(err)
		}
		d := decoder{data: data}
		if !d.catalog(&got) {
			t.Fatalf("%s: fast path refused a snapshot", path)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("%s: decoded differently", path)
		}
		t.Logf("%s: %d titles identical", path, len(got.Titles))
	}
}
