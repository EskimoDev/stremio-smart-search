package catalog

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const MaxTitles = 500000

// Alternative-title bounds. The builder keeps at most AliasesPerTitle per title;
// the loader tolerates a little more for hand-made catalogues. MaxAliases bounds
// the snapshot, the decompressed JSON size and the index regardless of profile.
const (
	AliasesPerTitle    = 4
	MaxAliasesPerTitle = 8
	MaxAliases         = 600000
	maxAliasBytes      = 300
)

type Title struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Original string  `json:"original,omitempty"`
	Type     string  `json:"type"`
	Year     int     `json:"year,omitempty"`
	Votes    int     `json:"votes,omitempty"`
	Rating   float64 `json:"rating,omitempty"`
	// Aliases are optional regional/translated titles (IMDb title.akas). They are
	// search keys only; results keep the canonical ID and name. Older builds
	// ignore this field, and older snapshots simply have none.
	Aliases []string `json:"aliases,omitempty"`
}

type Catalog struct {
	Version int    `json:"version"`
	Profile string `json:"profile,omitempty"`
	// AliasLanguage/AliasRegion record which alternative titles this snapshot
	// contains. Empty means none (every snapshot made before this option).
	AliasLanguage string    `json:"aliasLanguage,omitempty"`
	AliasRegion   string    `json:"aliasRegion,omitempty"`
	BuiltAt       time.Time `json:"builtAt"`
	Source        string    `json:"source"`
	Titles        []Title   `json:"titles"`
	// Sources records the HTTP validators of the dataset files this catalogue
	// was built from. It is filled by a download, kept by the caller in its
	// update state, and never written to the snapshot.
	Sources []SourceFile `json:"-"`
}

func (c *Catalog) Validate() error {
	if c.Version != 1 || len(c.Titles) == 0 || len(c.Titles) > MaxTitles {
		return fmt.Errorf("unsupported or empty catalogue (maximum %d titles)", MaxTitles)
	}
	if _, ok := ProfileByID(c.Profile); c.Profile != "" && !ok {
		return fmt.Errorf("unsupported catalogue profile")
	}
	// Unknown alias languages are tolerated so a snapshot from a newer build
	// still loads; they only label the snapshot.
	if len(c.AliasLanguage) > 16 || len(c.AliasRegion) > 16 {
		return fmt.Errorf("invalid alternative-title selection")
	}
	seen := make(map[string]bool, len(c.Titles))
	aliases := 0
	for _, t := range c.Titles {
		if len(t.Aliases) > MaxAliasesPerTitle {
			return fmt.Errorf("too many alternative titles for one entry")
		}
		for _, alias := range t.Aliases {
			if len(alias) == 0 || len(alias) > maxAliasBytes {
				return fmt.Errorf("invalid alternative title")
			}
		}
		if aliases += len(t.Aliases); aliases > MaxAliases {
			return fmt.Errorf("catalogue exceeds %d alternative titles", MaxAliases)
		}
		if len(t.ID) < 3 || len(t.ID) > 20 || t.ID[:2] != "tt" || len(t.Name) == 0 || len(t.Name) > 1000 || (t.Type != "movie" && t.Type != "series") {
			return fmt.Errorf("invalid catalogue entry")
		}
		for _, ch := range t.ID[2:] {
			if ch < '0' || ch > '9' {
				return fmt.Errorf("invalid title identifier")
			}
		}
		key := t.Type + ":" + t.ID
		if seen[key] {
			return fmt.Errorf("duplicate catalogue entry")
		}
		seen[key] = true
	}
	return nil
}

// Options reports the alternative-title selection stored with the snapshot.
func (c *Catalog) Options() AliasOptions {
	return AliasOptions{Language: c.AliasLanguage, Region: c.AliasRegion}
}

// AliasCount is the number of stored alternative titles.
func (c *Catalog) AliasCount() int {
	n := 0
	for _, t := range c.Titles {
		n += len(t.Aliases)
	}
	return n
}

func Load(path string) (*Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buffer := bufio.NewReader(f)
	var reader io.Reader = buffer
	size := int64(0) // expected decompressed size, a capacity hint only
	if info, err := f.Stat(); err == nil {
		size = info.Size()
	}
	magic, _ := buffer.Peek(2)
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		size = gzipSize(f, size)
		z, err := gzip.NewReader(buffer)
		if err != nil {
			return nil, err
		}
		defer z.Close()
		reader = z
	}
	// The whole document is read into one buffer of about the right size
	// (json.Decoder buffered it too, growing by doubling) and then decoded.
	const limit = 128 << 20
	data := bytes.NewBuffer(make([]byte, 0, min(max(size, 0), limit)+bytes.MinRead))
	if _, err := data.ReadFrom(io.LimitReader(reader, limit)); err != nil {
		return nil, fmt.Errorf("could not read catalogue: %w", err)
	}
	var c Catalog
	if err := decodeCatalog(data.Bytes(), &c); err != nil {
		return nil, fmt.Errorf("could not read catalogue: %w", err)
	}
	return &c, c.Validate()
}

// gzipSize reads a gzip file's recorded uncompressed size (modulo 4 GiB) from
// its last four bytes, or 0 when it cannot be read.
func gzipSize(f *os.File, length int64) int64 {
	var tail [4]byte
	if length < 18 {
		return 0
	}
	if _, err := f.ReadAt(tail[:], length-4); err != nil {
		return 0
	}
	return int64(binary.LittleEndian.Uint32(tail[:]))
}

func Save(path string, c *Catalog) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".catalog-*.tmp")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	_ = f.Chmod(0600)
	z := gzip.NewWriter(f)
	err = json.NewEncoder(z).Encode(c)
	if closeErr := z.Close(); err == nil {
		err = closeErr
	}
	if syncErr := f.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	// The serving index stays live while the new file is prepared. Windows cannot
	// replace an existing path with Rename; preserve the previous snapshot on failure.
	backup := path + ".previous"
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(backup)
		if err := os.Rename(path, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

// CarryAliases copies the alternative titles stored in the snapshot at path
// onto c's titles with the same type and ID, so a routine update can skip
// re-downloading IMDb's 500 MB title.akas file. It decodes only IDs and
// aliases, never the rest of the old snapshot. The snapshot must hold the
// same alternative-title selection; titles new since it was made get none
// until title.akas is next downloaded.
func CarryAliases(path string, c *Catalog, options AliasOptions) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buffer := bufio.NewReader(f)
	var reader io.Reader = buffer
	if magic, _ := buffer.Peek(2); len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		z, err := gzip.NewReader(buffer)
		if err != nil {
			return err
		}
		defer z.Close()
		reader = z
	}
	var old struct {
		Version       int    `json:"version"`
		AliasLanguage string `json:"aliasLanguage"`
		AliasRegion   string `json:"aliasRegion"`
		Titles        []struct {
			ID      string   `json:"id"`
			Type    string   `json:"type"`
			Aliases []string `json:"aliases"`
		} `json:"titles"`
	}
	if err := json.NewDecoder(io.LimitReader(reader, 128<<20)).Decode(&old); err != nil {
		return fmt.Errorf("could not read the previous alternative titles: %w", err)
	}
	if old.Version != 1 || old.AliasLanguage != options.Language || old.AliasRegion != options.Region {
		return fmt.Errorf("the previous catalogue has a different alternative-title selection")
	}
	byKey := make(map[string][]string, len(old.Titles))
	for _, t := range old.Titles {
		if len(t.Aliases) > 0 {
			byKey[t.Type+":"+t.ID] = t.Aliases
		}
	}
	for i := range c.Titles {
		c.Titles[i].Aliases = byKey[c.Titles[i].Type+":"+c.Titles[i].ID]
	}
	c.AliasLanguage, c.AliasRegion = options.Language, options.Region
	return c.Validate()
}
