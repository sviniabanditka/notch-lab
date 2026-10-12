package editor

import "github.com/sviniabanditka/notch-lab/internal/model"

// What the MCP tools of the domain `editor` use (05-editor.md, "MCP
// tools"). Main goroutine only.

// FileInfo describes an editor Tab for editor.get_open_files.
type FileInfo struct {
	Tab   string `json:"tab"`
	Path  string `json:"path"`
	Dirty bool   `json:"dirty"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

// Describe is a Tab's FileInfo; the cursor comes from the live Buffer.
func (b *Bench) Describe(t *model.Tab) FileInfo {
	d := readData(t)
	in := FileInfo{Tab: t.ID, Path: d.Path, Dirty: b.dirty(t), Line: d.Line, Col: d.Col}
	if x := b.tabs[t]; x != nil && x.doc != nil && len(x.doc.Cursors) > 0 {
		in.Line, in.Col = x.doc.Position(x.doc.Cursors[0].Pos)
		in.Line++
		in.Col++
	}
	return in
}

// Dirty reports unsaved changes in a Tab.
func (b *Bench) Dirty(t *model.Tab) bool { return b.dirty(t) }

// Selection is the first selection of a Tab: 1-based line and column of
// its start and end, and its text; ok is false before the File loads.
func (b *Bench) Selection(t *model.Tab) (path string, sl, sc, el, ec int, text string, ok bool) {
	x := b.tabs[t]
	if x == nil || x.doc == nil || len(x.doc.Cursors) == 0 {
		return "", 0, 0, 0, 0, "", false
	}
	c := x.doc.Cursors[0]
	from, to := min(c.Pos, c.Anchor), max(c.Pos, c.Anchor)
	sl, sc = x.doc.Position(from)
	el, ec = x.doc.Position(to)
	s := x.doc.String()
	text = s[from:to]
	return readData(t).Path, sl + 1, sc + 1, el + 1, ec + 1, text, true
}

// Goto moves a Tab's cursor to a 1-based line and column and centres it.
func (b *Bench) Goto(t *model.Tab, line, col int) { b.gotoLine(t, line, max(1, col)) }
