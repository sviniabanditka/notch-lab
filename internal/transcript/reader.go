package transcript

import (
	"bytes"
	"io"
	"os"
	"syscall"
)

// tailWindow is how much of a transcript's end the chat opens with, and
// how much more before it each scroll to the top reads.
const tailWindow = 2 << 20

// reader reads a transcript's whole lines as Claude appends them. Claude
// may be midway through a line: a read keeps its offset before it and
// reads the line whole next time.
type reader struct {
	path   string
	window int64
	// offset is just past the last whole line read; -1 before the first.
	offset int64
	ino    uint64
	// start is where the first whole line read begins; cut is set while
	// it lies past the file's start.
	start int64
	cut   bool
}

func newReader(path string, window int64) *reader {
	return &reader{path: path, window: window, offset: -1}
}

// read returns the lines written since the last read. reset is set when
// the file was replaced or shrank: lines are then the new file's tail.
func (r *reader) read() (lines [][]byte, reset bool, err error) {
	f, err := os.Open(r.path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	var ino uint64
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		ino = st.Ino
	}
	size := info.Size()
	if r.offset >= 0 && (ino != r.ino || size < r.offset) {
		r.offset, reset = -1, true
	}
	r.ino = ino
	if r.offset < 0 {
		lines, err = r.tail(f, size)
		return lines, reset, err
	}
	if size == r.offset {
		return nil, reset, nil
	}
	buf := make([]byte, size-r.offset)
	if _, err := f.ReadAt(buf, r.offset); err != nil && err != io.EOF {
		return nil, reset, err
	}
	end := bytes.LastIndexByte(buf, '\n')
	if end < 0 {
		return nil, reset, nil
	}
	r.offset += int64(end + 1)
	return split(buf[:end+1]), reset, nil
}

// tail reads the file's last window, grown until it holds a whole line
// past the one it cut into.
func (r *reader) tail(f *os.File, size int64) ([][]byte, error) {
	for window := max(r.window, 1); ; window *= 2 {
		start := max(size-window, 0)
		buf := make([]byte, size-start)
		if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
			return nil, err
		}
		first := 0
		if start > 0 {
			i := bytes.IndexByte(buf, '\n')
			if i < 0 || bytes.IndexByte(buf[i+1:], '\n') < 0 {
				continue
			}
			first = i + 1
		}
		r.start, r.cut = start+int64(first), start > 0
		end := bytes.LastIndexByte(buf, '\n')
		if end < 0 {
			r.offset, r.start = 0, 0
			return nil, nil
		}
		r.offset = start + int64(end+1)
		return split(buf[first : end+1]), nil
	}
}

// earlier returns the window of whole lines before those read, grown
// until it holds one. It returns none at the file's start, or when the
// file was replaced, which the next read resets.
func (r *reader) earlier() ([][]byte, error) {
	if !r.cut {
		return nil, nil
	}
	f, err := os.Open(r.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Ino != r.ino || info.Size() < r.offset {
		return nil, nil
	}
	end := r.start
	for window := max(r.window, 1); ; window *= 2 {
		start := max(end-window, 0)
		buf := make([]byte, end-start)
		if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
			return nil, err
		}
		first := 0
		if start > 0 {
			// buf ends with the newline before r.start, so the line it
			// cuts into ends at its first newline: one past it must be whole.
			i := bytes.IndexByte(buf, '\n')
			if i < 0 || i == len(buf)-1 {
				continue
			}
			first = i + 1
		}
		r.start, r.cut = start+int64(first), start > 0
		return split(buf[first:]), nil
	}
}

// split cuts newline-ended text into its non-empty lines.
func split(b []byte) [][]byte {
	var out [][]byte
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		if i > 0 {
			out = append(out, b[:i])
		}
		b = b[i+1:]
	}
	return out
}
