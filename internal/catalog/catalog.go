package catalog

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const MaxTitles = 50000

type Title struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Original string  `json:"original,omitempty"`
	Type     string  `json:"type"`
	Year     int     `json:"year,omitempty"`
	Votes    int     `json:"votes,omitempty"`
	Rating   float64 `json:"rating,omitempty"`
}

type Catalog struct {
	Version int       `json:"version"`
	BuiltAt time.Time `json:"builtAt"`
	Source  string    `json:"source"`
	Titles  []Title   `json:"titles"`
}

func (c *Catalog) Validate() error {
	if c.Version != 1 || len(c.Titles) == 0 || len(c.Titles) > MaxTitles {
		return fmt.Errorf("unsupported or empty catalogue (maximum %d titles)", MaxTitles)
	}
	seen := make(map[string]bool, len(c.Titles))
	for _, t := range c.Titles {
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

func Load(path string) (*Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buffer := bufio.NewReader(f)
	var reader io.Reader = buffer
	magic, _ := buffer.Peek(2)
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		z, err := gzip.NewReader(buffer)
		if err != nil {
			return nil, err
		}
		defer z.Close()
		reader = z
	}
	var c Catalog
	if err := json.NewDecoder(io.LimitReader(reader, 32<<20)).Decode(&c); err != nil {
		return nil, fmt.Errorf("could not read catalogue: %w", err)
	}
	return &c, c.Validate()
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
