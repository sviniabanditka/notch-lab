package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/dbconn"
	ed "github.com/sviniabanditka/notch-lab/internal/editor"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// data is a db Tab's type data (R-DB-040).
type data struct {
	Connection string `json:"connection"`
	File       string `json:"file,omitempty"`
	SQL        string `json:"sql,omitempty"`
	ReadOnly   bool   `json:"readOnly,omitempty"` // a DDL view
}

func readData(t *model.Tab) data {
	var d data
	_ = json.Unmarshal([]byte(t.Data), &d)
	return d
}

func writeData(t *model.Tab, d data) {
	raw, _ := json.Marshal(d)
	t.Data = string(raw)
}

// tab is a Query Tab's live part.
type tab struct {
	// opts shows the popover of the run options: limit and timeout.
	opts     bool
	t        *model.Tab
	doc      *ed.Doc
	view     *ed.View
	hl       *ed.Highlighter
	saved    string
	cancel   context.CancelFunc
	results  []*dbconn.Result
	shown    int
	messages []string
	limit    bool
	timeout  float64
	split    float32
	history  bool
	grid     ui.ListState
	sort     ui.SortOrder
	filters  []string
	cell     *string
	sel      int
	errAt    int
}

// title is the file name, else the SQL's start, else "Query" (R-DB-040).
func title(d data, sql string) string {
	if d.File != "" {
		return filepath.Base(d.File)
	}
	s := strings.Join(strings.Fields(sql), " ")
	if s == "" {
		return "Query"
	}
	if utf8.RuneCountInString(s) > 40 {
		s = string([]rune(s)[:40])
	}
	return s
}

func (b *Bench) liveTab(dark bool, t *model.Tab) *tab {
	if x := b.tabs[t]; x != nil {
		return x
	}
	d := readData(t)
	text := d.SQL
	if d.File != "" {
		if data, err := os.ReadFile(d.File); err == nil {
			text = string(data)
		}
	}
	x := &tab{t: t, limit: true, timeout: 30, split: 220, saved: text, sel: -1}
	x.doc = ed.NewDoc(text)
	x.hl = &ed.Highlighter{Lexer: ed.Lexer("query.sql", ""), Pal: b.palette(dark), Post: b.Post}
	x.view = ed.NewView(x.doc, ui.Font{Family: "SF Mono, Menlo", Size: 13})
	x.view.HL, x.view.LineNumbers, x.view.ReadOnly = x.hl, true, d.ReadOnly
	x.doc.Changed = func() {
		x.hl.Changed(x.doc.Snapshot(), false)
		if d := readData(t); d.File == "" {
			d.SQL = x.doc.String()
			writeData(t, d)
			t.Title = title(d, d.SQL)
			b.St.Changed()
		}
	}
	x.hl.Changed(x.doc.Snapshot(), true)
	x.grid.Selected, x.grid.Sort = &x.sel, &x.sort
	b.tabs[t] = x
	return x
}

// NewTab opens an empty Query on the selected Connection (R-DB-041).
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	conn := ""
	if p := b.panel[bench]; p != nil && p.sel != "" {
		conn = p.sel
	} else if f := bench.FocusedTab(); f != nil {
		conn = readData(f).Connection
	} else if list := b.Connections(bench); len(list) > 0 {
		conn = list[0].ID
	}
	t.Title = "Query"
	writeData(t, data{Connection: conn})
}

// OpenQuery opens a Tab with SQL on a Connection; run runs it at once.
func (b *Bench) OpenQuery(bench *model.Bench, conn, sql string, readOnly bool) *model.Tab {
	t := &model.Tab{ID: model.NewID(), Bench: bench, Data: "{}"}
	d := data{Connection: conn, SQL: sql, ReadOnly: readOnly}
	writeData(t, d)
	t.Title = title(d, sql)
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t
}

// statementsToRun is the selection, else the statement under the cursor,
// else every statement (all) (R-DB-045).
func (x *tab) statementsToRun(all bool) []dbconn.Statement {
	text := x.doc.String()
	if all {
		return dbconn.Split(text)
	}
	if sel := x.doc.Selected(); len(sel) > 0 && strings.TrimSpace(sel[0]) != "" {
		return dbconn.Split(sel[0])
	}
	pos := 0
	if len(x.doc.Cursors) > 0 {
		pos = byteOffset(text, x.doc.Cursors[0].Pos)
	}
	if s, ok := dbconn.At(text, pos); ok {
		return []dbconn.Statement{s}
	}
	return nil
}

func byteOffset(s string, runes int) int {
	i := 0
	for n := range s {
		if i == runes {
			return n
		}
		i++
	}
	return len(s)
}

// ErrRefused is a statement the Connection's rules stop (R-DB-051).
var ErrRefused = errors.New("Read-only connection. Turn read-only off in the Connection settings to run this.")

// Check applies R-DB-050 … R-DB-052 to statements: a read-only Connection
// refuses all but reads; ask confirms a dangerous one, nil skips that.
func Check(c *dbconn.Connection, stmts []dbconn.Statement, ask func(stmt, why string) bool) error {
	for _, s := range stmts {
		cls := dbconn.Classify(s.SQL)
		if c.ReadOnly && cls != dbconn.Read {
			return ErrRefused
		}
		if why := dbconn.Dangerous(s.SQL); why != "" && ask != nil && !ask(s.SQL, why) {
			return errors.New("Not run")
		}
	}
	return nil
}

// Run runs statements in a Tab one after another; each with rows makes a
// Result (R-DB-060). done gets the Results on the main goroutine.
func (b *Bench) Run(t *model.Tab, stmts []dbconn.Statement, limit, timeout int, done func([]*dbconn.Result)) error {
	x := b.tabs[t]
	if x == nil {
		return errors.New("the Tab is not open")
	}
	if x.cancel != nil {
		return errors.New("a statement runs in this Tab")
	}
	c := b.Conns.Get(readData(t).Connection)
	if c == nil {
		return errors.New("Choose a Connection")
	}
	ctx, cancel := context.WithCancel(context.Background())
	x.cancel, x.messages, x.errAt = cancel, nil, 0
	b.Open(t.Bench, c, func(d *dbconn.DB, err error) {
		if err != nil {
			x.cancel = nil
			x.messages = []string{err.Error()}
			if done != nil {
				done(nil)
			}
			return
		}
		go func() {
			var out []*dbconn.Result
			for _, s := range stmts {
				sctx, scancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
				r := d.Run(sctx, s.SQL, limit, 0)
				scancel()
				if r.Err == "Timed out" {
					r.Err = fmt.Sprintf("Timed out after %d s", timeout) // R-DB-048
				}
				if r.ErrPos > 0 {
					r.ErrPos += s.Offset
				}
				out = append(out, r)
				rows := int64(len(r.Rows))
				if !r.HasRows {
					rows = r.Affected
				}
				dbconn.AppendHistory(filepath.Join(b.Dir, "db", "history"), c.ID, dbconn.HistoryEntry{At: time.Now(), SQL: s.SQL, Ms: r.Ms, Rows: rows, Error: r.Err})
				if r.Err != "" || ctx.Err() != nil {
					break
				}
			}
			b.Post(func() {
				x.cancel = nil
				b.show(x, out)
				if b.Visible != nil && !b.Visible(t) {
					t.Unread = true // R-DB-071
				}
				if done != nil {
					done(out)
				}
			})
		}()
	})
	return nil
}

// show puts Results in the Tab: one tab per statement with rows, messages
// for the rest; an error position moves the cursor (R-DB-070).
func (b *Bench) show(x *tab, out []*dbconn.Result) {
	x.results = nil
	for _, r := range out {
		x.messages = append(x.messages, r.Notices...)
		if r.Err != "" {
			x.messages = append(x.messages, "Error: "+r.Err)
			if r.ErrPos > 0 {
				text := x.doc.String()
				p := utf8.RuneCountInString(text[:min(len(text), max(0, r.ErrPos-1))])
				x.doc.SetCursors(ed.Cursor{Pos: p, Anchor: p, Goal: -1})
				x.view.RevealCursor()
			}
			continue
		}
		if r.HasRows {
			x.results = append(x.results, r)
		} else {
			x.messages = append(x.messages, r.Message())
		}
	}
	x.shown = len(x.results) - 1
	x.sel, x.sort, x.filters = -1, ui.SortOrder{}, nil
}

// LoadMore runs the shown Result's statement again with the next 500 rows
// and appends them (R-DB-046).
func (b *Bench) loadMore(x *tab, r *dbconn.Result) {
	c := b.Conns.Get(readData(x.t).Connection)
	l := b.live[readData(x.t).Connection]
	if c == nil || l == nil || l.db == nil || x.cancel != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(x.timeout)*time.Second)
	x.cancel = cancel
	d := l.db
	off := r.Offset + len(r.Rows)
	go func() {
		more := d.Run(ctx, r.SQL, r.Limit, off)
		cancel()
		b.Post(func() {
			x.cancel = nil
			if more.Err != "" {
				x.messages = append(x.messages, "Error: "+more.Err)
				return
			}
			r.Rows = append(r.Rows, more.Rows...)
			r.Limited = more.Limited
		})
	}()
}

// Cancel stops the running statement (R-DB-049).
func (b *Bench) Cancel(t *model.Tab) {
	if x := b.tabs[t]; x != nil && x.cancel != nil {
		x.cancel()
	}
}

// Save writes the SQL to the Tab's file, or asks a name (R-DB-055).
func (b *Bench) Save(t *model.Tab) {
	x := b.tabs[t]
	if x == nil {
		return
	}
	d := readData(t)
	write := func(p string) error {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(x.doc.String()), 0o644); err != nil {
			return err
		}
		x.saved = x.doc.String()
		d.File, d.SQL = p, ""
		writeData(t, d)
		t.Title = title(d, "")
		b.St.Changed()
		return nil
	}
	if d.File != "" {
		if err := write(d.File); err != nil {
			b.toast(err.Error())
		}
		return
	}
	b.AskText("Save Query as", "", func(name string) string {
		name = strings.TrimSuffix(strings.TrimSpace(name), ".sql")
		if name == "" || strings.ContainsAny(name, `/\:`) {
			return "Not a file name"
		}
		if err := write(filepath.Join(b.Folder(t.Bench), name+".sql")); err != nil {
			return err.Error()
		}
		return ""
	})
}

// LastResult is a Tab's last Result (db.result.last).
func (b *Bench) LastResult(t *model.Tab) *dbconn.Result {
	x := b.tabs[t]
	if x == nil || len(x.results) == 0 {
		return nil
	}
	return x.results[len(x.results)-1]
}

// Live gives a Tab its live part without a frame (for MCP).
func (b *Bench) Live(t *model.Tab) { b.liveTab(false, t) }

func cursorAt(p int) ed.Cursor { return ed.Cursor{Pos: p, Anchor: p, Goal: -1} }
