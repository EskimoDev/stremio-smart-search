package catalog

import (
	"bytes"
	"encoding/json"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// decodeCatalog reads a snapshot. Snapshots are written by Save in one known
// shape, which a direct scanner reads several times faster than
// encoding/json's reflection. Anything outside that shape (an unknown or
// differently cased key, null, an unusual escape, invalid UTF-8, a syntax
// error) is handed to encoding/json instead, so the result, and any error,
// is always exactly what encoding/json would give.
func decodeCatalog(data []byte, c *Catalog) error {
	var fast Catalog
	d := decoder{data: data}
	if d.catalog(&fast) {
		*c = fast
		return nil
	}
	return json.NewDecoder(bytes.NewReader(data)).Decode(c)
}

// decoder scans one JSON value. Every method reports false when the input
// leaves the shape it understands; the caller then falls back.
type decoder struct {
	data []byte
	i    int
	buf  []byte // for strings with escapes
	// kinds interns the title types, which are nearly always one of two.
	movie, series string
}

func (d *decoder) space() {
	for d.i < len(d.data) {
		switch d.data[d.i] {
		case ' ', '\t', '\n', '\r':
			d.i++
		default:
			return
		}
	}
}

func (d *decoder) next(c byte) bool {
	d.space()
	if d.i < len(d.data) && d.data[d.i] == c {
		d.i++
		return true
	}
	return false
}

// members visits each key of an object; visit reads the value.
func (d *decoder) members(visit func(key []byte) bool) bool {
	if !d.next('{') {
		return false
	}
	if d.next('}') {
		return true
	}
	for {
		key, ok := d.rawString()
		if !ok || !d.next(':') || !visit(key) {
			return false
		}
		if d.next(',') {
			continue
		}
		return d.next('}')
	}
}

// rawString returns a string's bytes when it has no escapes, ASCII control
// characters or invalid UTF-8. The slice aliases the input.
func (d *decoder) rawString() ([]byte, bool) {
	if !d.next('"') {
		return nil, false
	}
	start := d.i
	ascii := true
	for d.i < len(d.data) {
		c := d.data[d.i]
		switch {
		case c == '"':
			s := d.data[start:d.i]
			d.i++
			return s, ascii || utf8.Valid(s)
		case c == '\\' || c < 0x20:
			return nil, false
		case c >= utf8.RuneSelf:
			ascii = false
		}
		d.i++
	}
	return nil, false
}

// str reads a string value, decoding the escapes Save's encoder writes.
func (d *decoder) str() (string, bool) {
	if !d.next('"') {
		return "", false
	}
	start := d.i
	for d.i < len(d.data) {
		c := d.data[d.i]
		if c == '"' {
			s := d.data[start:d.i]
			d.i++
			if !utf8.Valid(s) {
				return "", false
			}
			return string(s), true
		}
		if c == '\\' {
			return d.escaped(start)
		}
		if c < 0x20 {
			return "", false
		}
		d.i++
	}
	return "", false
}

func (d *decoder) escaped(start int) (string, bool) {
	out := append(d.buf[:0], d.data[start:d.i]...)
	for d.i < len(d.data) {
		c := d.data[d.i]
		switch {
		case c == '"':
			d.i++
			d.buf = out
			if !utf8.Valid(out) {
				return "", false
			}
			return string(out), true
		case c < 0x20:
			return "", false
		case c != '\\':
			out = append(out, c)
			d.i++
			continue
		}
		if d.i+1 >= len(d.data) {
			return "", false
		}
		e := d.data[d.i+1]
		d.i += 2
		switch e {
		case '"', '\\', '/':
			out = append(out, e)
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r, ok := d.hex4()
			if !ok || utf16.IsSurrogate(r) {
				return "", false // surrogate pairs are left to encoding/json
			}
			out = utf8.AppendRune(out, r)
		default:
			return "", false
		}
	}
	return "", false
}

func (d *decoder) hex4() (rune, bool) {
	if d.i+4 > len(d.data) {
		return 0, false
	}
	var r rune
	for _, c := range d.data[d.i : d.i+4] {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			r |= rune(c - 'A' + 10)
		default:
			return 0, false
		}
	}
	d.i += 4
	return r, true
}

// number returns a JSON number's text.
func (d *decoder) number() ([]byte, bool) {
	d.space()
	start := d.i
	for d.i < len(d.data) {
		c := d.data[d.i]
		if (c < '0' || c > '9') && c != '-' && c != '+' && c != '.' && c != 'e' && c != 'E' {
			break
		}
		d.i++
	}
	return d.data[start:d.i], d.i > start && validNumber(d.data[start:d.i])
}

// validNumber checks JSON number grammar, which is stricter than strconv's.
func validNumber(s []byte) bool {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	switch {
	case i < len(s) && s[i] == '0':
		i++
	case i < len(s) && s[i] >= '1' && s[i] <= '9':
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	default:
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	return i == len(s)
}

func (d *decoder) integer(n *int) bool {
	text, ok := d.number()
	if !ok {
		return false
	}
	v, err := strconv.ParseInt(string(text), 10, 0)
	*n = int(v)
	return err == nil
}

func (d *decoder) catalog(c *Catalog) bool {
	var seen [7]bool
	once := func(i int) bool {
		if seen[i] {
			return false // a repeated key: encoding/json's merging rules apply
		}
		seen[i] = true
		return true
	}
	return d.members(func(key []byte) bool {
		var ok bool
		switch string(key) {
		case "version":
			ok = once(0) && d.integer(&c.Version)
		case "profile":
			ok = once(1)
			c.Profile, ok = d.strIf(ok)
		case "aliasLanguage":
			ok = once(2)
			c.AliasLanguage, ok = d.strIf(ok)
		case "aliasRegion":
			ok = once(3)
			c.AliasRegion, ok = d.strIf(ok)
		case "builtAt":
			ok = once(4) && d.time(c)
		case "source":
			ok = once(5)
			c.Source, ok = d.strIf(ok)
		case "titles":
			ok = once(6) && d.titles(c)
		}
		return ok
	})
}

func (d *decoder) strIf(ok bool) (string, bool) {
	if !ok {
		return "", false
	}
	return d.str()
}

func (d *decoder) time(c *Catalog) bool {
	d.space()
	start := d.i
	if _, ok := d.rawString(); !ok {
		return false
	}
	return c.BuiltAt.UnmarshalJSON(d.data[start:d.i]) == nil
}

func (d *decoder) titles(c *Catalog) bool {
	if !d.next('[') {
		return false
	}
	// A title takes at least 40 bytes in a snapshot, so this rarely grows
	// and never more than doubles the need.
	c.Titles = make([]Title, 0, min((len(d.data)-d.i)/120+1, MaxTitles+1))
	if d.next(']') {
		return true
	}
	for {
		c.Titles = append(c.Titles, Title{})
		if !d.title(&c.Titles[len(c.Titles)-1]) {
			return false
		}
		if d.next(',') {
			continue
		}
		return d.next(']')
	}
}

func (d *decoder) title(t *Title) bool {
	var seen [8]bool
	return d.members(func(key []byte) bool {
		var field int
		switch string(key) {
		case "id":
			field = 0
		case "name":
			field = 1
		case "original":
			field = 2
		case "type":
			field = 3
		case "year":
			field = 4
		case "votes":
			field = 5
		case "rating":
			field = 6
		case "aliases":
			field = 7
		default:
			return false
		}
		if seen[field] {
			return false
		}
		seen[field] = true
		var ok bool
		switch field {
		case 0:
			t.ID, ok = d.str()
		case 1:
			t.Name, ok = d.str()
		case 2:
			t.Original, ok = d.str()
		case 3:
			t.Type, ok = d.kind()
		case 4:
			ok = d.integer(&t.Year)
		case 5:
			ok = d.integer(&t.Votes)
		case 6:
			var text []byte
			if text, ok = d.number(); ok {
				var err error
				t.Rating, err = strconv.ParseFloat(string(text), 64)
				ok = err == nil
			}
		case 7:
			ok = d.aliases(t)
		}
		return ok
	})
}

// kind reads a title type, sharing one copy of each common value.
func (d *decoder) kind() (string, bool) {
	raw, ok := d.rawString()
	if !ok {
		return "", false
	}
	switch string(raw) {
	case "movie":
		if d.movie == "" {
			d.movie = "movie"
		}
		return d.movie, true
	case "series":
		if d.series == "" {
			d.series = "series"
		}
		return d.series, true
	}
	return string(raw), true
}

func (d *decoder) aliases(t *Title) bool {
	if !d.next('[') {
		return false
	}
	if d.next(']') {
		t.Aliases = []string{}
		return true
	}
	for {
		s, ok := d.str()
		if !ok {
			return false
		}
		t.Aliases = append(t.Aliases, s)
		if d.next(',') {
			continue
		}
		return d.next(']')
	}
}
