package catalog

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestReadAheadCopiesAndKeepsErrors(t *testing.T) {
	data := strings.Repeat("tt0000001\t7.5\t100\n", 100000) // several chunks
	r, stop := readAhead(iotest.OneByteReader(strings.NewReader(data)))
	got, err := io.ReadAll(r)
	stop()
	if err != nil || string(got) != data {
		t.Fatalf("read %d bytes, err %v", len(got), err)
	}
	r, stop = readAhead(io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(io.ErrUnexpectedEOF)))
	got, err = io.ReadAll(r)
	stop()
	if !errors.Is(err, io.ErrUnexpectedEOF) || string(got) != "partial" {
		t.Fatalf("truncated source must keep its error: %q %v", got, err)
	}
	// Stopping early must not block or leak the reading goroutine.
	r, stop = readAhead(strings.NewReader(data))
	_, _ = r.Read(make([]byte, 10))
	stop()
	stop()
}

func TestSplitTabs(t *testing.T) {
	var dst [][]byte
	for _, tc := range []struct {
		line string
		n    int
		want []string
	}{{"a\tb\tc", 2, []string{"a", "b"}}, {"a\tb\tc", 5, []string{"a", "b", "c"}}, {"", 2, []string{""}}, {"a\t\tc\t", 4, []string{"a", "", "c", ""}}} {
		dst = splitTabs([]byte(tc.line), dst, tc.n)
		if len(dst) != len(tc.want) {
			t.Fatalf("%q: got %q", tc.line, dst)
		}
		for i := range dst {
			if !bytes.Equal(dst[i], []byte(tc.want[i])) {
				t.Fatalf("%q: got %q", tc.line, dst)
			}
		}
		if all := strings.Split(tc.line, "\t"); len(all) < tc.n && len(all) != len(dst) {
			t.Fatalf("%q: short-row detection differs from strings.Split", tc.line)
		}
	}
}
