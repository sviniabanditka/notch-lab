package diffview

import (
	"fmt"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

type rowKind uint8

const (
	rowFile rowKind = iota
	rowHunk
	rowContext
	rowAdd
	rowDel
	rowNote
)

// row is one painted line of the unified diff. old and new are 1-based
// line numbers, 0 where the line has none on that side.
type row struct {
	text     string
	old, new int32
	file     int32
	kind     rowKind
}

type fileInfo struct {
	path string
	// name is the path the file has after the change, which picks its lexer.
	name       string
	adds, dels int
	// first is the file's header row; its body runs to end.
	first, end int32
	collapsed  bool
}

// chunk is a run of rows of one hunk that is highlighted at once.
type chunk struct{ file, start, end int32 }

// chunkRows caps a chunk, so that a huge hunk highlights in parts and the
// part on screen comes first; a token that spans the cut loses its color.
const chunkRows = 1000

type model struct {
	rows   []row
	files  []fileInfo
	chunks []chunk
	// vis maps a visible row, after collapsed files, to its index in rows.
	vis     []int32
	maxNo   int32
	maxCols int
}

func newModel(files []*gitdiff.File, notes []Note) *model {
	m := &model{}
	noted := make(map[string]string, len(notes))
	for _, n := range notes {
		noted[n.Path] = n.Text
	}
	for fi, f := range files {
		name := f.NewName
		if f.IsDelete {
			name = f.OldName
		}
		info := fileInfo{path: displayPath(f), name: name, first: int32(len(m.rows))}
		m.rows = append(m.rows, row{kind: rowFile, file: int32(fi)})
		if note, ok := noted[name]; ok && len(f.TextFragments) == 0 {
			m.rows = append(m.rows, row{kind: rowNote, file: int32(fi), text: note})
		} else if f.IsBinary {
			m.rows = append(m.rows, row{kind: rowNote, file: int32(fi), text: "Binary file not shown"})
		} else if len(f.TextFragments) == 0 {
			m.rows = append(m.rows, row{kind: rowNote, file: int32(fi), text: "No content changes"})
		}
		for _, frag := range f.TextFragments {
			m.rows = append(m.rows, row{kind: rowHunk, file: int32(fi), text: hunkHeader(frag)})
			old, nw := int32(frag.OldPosition), int32(frag.NewPosition)
			start := int32(len(m.rows))
			for _, ln := range frag.Lines {
				r := row{file: int32(fi), text: strings.TrimRight(ln.Line, "\r\n")}
				switch ln.Op {
				case gitdiff.OpContext:
					r.kind, r.old, r.new = rowContext, old, nw
					old++
					nw++
				case gitdiff.OpDelete:
					r.kind, r.old = rowDel, old
					old++
					info.dels++
				case gitdiff.OpAdd:
					r.kind, r.new = rowAdd, nw
					nw++
					info.adds++
				}
				m.maxCols = max(m.maxCols, displayCols(r.text))
				m.rows = append(m.rows, r)
				if int32(len(m.rows))-start == chunkRows {
					m.chunks = append(m.chunks, chunk{int32(fi), start, int32(len(m.rows))})
					start = int32(len(m.rows))
				}
			}
			if end := int32(len(m.rows)); end > start {
				m.chunks = append(m.chunks, chunk{int32(fi), start, end})
			}
			m.maxNo = max(m.maxNo, old, nw)
		}
		info.end = int32(len(m.rows))
		m.files = append(m.files, info)
	}
	m.reindex()
	return m
}

func (m *model) reindex() {
	m.vis = m.vis[:0]
	for _, f := range m.files {
		if f.collapsed {
			m.vis = append(m.vis, f.first)
			continue
		}
		for i := f.first; i < f.end; i++ {
			m.vis = append(m.vis, i)
		}
	}
}

func (m *model) toggle(file int32) {
	m.files[file].collapsed = !m.files[file].collapsed
	m.reindex()
}

// chunkOf returns the chunk holding row i, or -1 for headers and notes.
func (m *model) chunkOf(i int32) int {
	lo, hi := 0, len(m.chunks)
	for lo < hi {
		mid := (lo + hi) / 2
		if m.chunks[mid].end <= i {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(m.chunks) && m.chunks[lo].start <= i {
		return lo
	}
	return -1
}

func displayPath(f *gitdiff.File) string {
	switch {
	case f.IsDelete:
		return f.OldName
	case f.IsRename && f.OldName != f.NewName:
		return f.OldName + " → " + f.NewName
	}
	return f.NewName
}

func hunkHeader(f *gitdiff.TextFragment) string {
	h := fmt.Sprintf("@@ -%d,%d +%d,%d @@", f.OldPosition, f.OldLines, f.NewPosition, f.NewLines)
	if f.Comment != "" {
		h += " " + f.Comment
	}
	return h
}

const tabWidth = 4

// displayCols is how many cells a line takes once tabs are expanded,
// counting every rune as one cell.
func displayCols(s string) int {
	col := 0
	for _, r := range s {
		if r == '\t' {
			col += tabWidth - col%tabWidth
		} else {
			col++
		}
	}
	return col
}
