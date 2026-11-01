package catalog

import (
	"io"
	"strconv"
)

// readAhead moves reading (gzip decompression, and the download itself) onto
// its own goroutine, so it overlaps with parsing on the helper's second thread
// instead of alternating with it. Decompression is the largest single cost of
// a catalogue build. Call stop when finished with the reader; it does not
// close the source.
func readAhead(source io.Reader) (reader io.Reader, stop func()) {
	const chunkSize, chunks = 256 << 10, 4
	type chunk struct {
		data []byte
		err  error
	}
	full := make(chan chunk, chunks)
	free := make(chan []byte, chunks)
	done := make(chan struct{})
	for i := 0; i < chunks; i++ {
		free <- make([]byte, chunkSize)
	}
	go func() {
		defer close(full)
		for {
			var buffer []byte
			select {
			case buffer = <-free:
			case <-done:
				return
			}
			// Fill the chunk, keeping the source's own error (a truncated gzip
			// stream reports io.ErrUnexpectedEOF, which must not become EOF).
			n := 0
			var err error
			for n < len(buffer) && err == nil {
				var read int
				read, err = source.Read(buffer[n:])
				n += read
			}
			select {
			case full <- chunk{buffer[:n], err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	r := &aheadReader{}
	r.next = func() ([]byte, error) {
		if r.current != nil {
			free <- r.current[:cap(r.current)]
			r.current = nil
		}
		c, ok := <-full
		if !ok {
			return nil, io.EOF
		}
		r.current = c.data
		return c.data, c.err
	}
	stopped := false
	return r, func() {
		if !stopped {
			stopped = true
			close(done)
		}
	}
}

type aheadReader struct {
	next    func() ([]byte, error)
	current []byte
	pending []byte
	err     error
}

func (r *aheadReader) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		r.pending, r.err = r.next()
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

// splitTabs cuts the first n tab-separated fields of line into dst without
// allocating. Fewer than n fields means the row is short.
func splitTabs(line []byte, dst [][]byte, n int) [][]byte {
	dst = dst[:0]
	for len(dst) < n {
		i := 0
		for i < len(line) && line[i] != '\t' {
			i++
		}
		dst = append(dst, line[:i])
		if i == len(line) {
			break
		}
		line = line[i+1:]
	}
	return dst
}

// atoi matches strconv.Atoi, without allocating for plain digits.
func atoi(field []byte) (int, error) {
	if len(field) > 0 && len(field) <= 18 {
		n := 0
		for _, c := range field {
			if c < '0' || c > '9' {
				return strconv.Atoi(string(field))
			}
			n = n*10 + int(c-'0')
		}
		return n, nil
	}
	return strconv.Atoi(string(field))
}
