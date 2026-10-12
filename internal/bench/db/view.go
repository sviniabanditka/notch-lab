package db

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/dbconn"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// View draws a Query Tab (R-DB-042).
func (b *Bench) View(c *ui.Context, t *model.Tab, focused bool) bool {
	x := b.liveTab(c.Theme().Dark, t)
	if p := b.palette(c.Theme().Dark); x.hl.Pal != p {
		x.hl.Pal = p
	}
	has := false
	ui.Column(c.Key("db-" + t.ID)).Grow(1).Basis(0).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		b.toolbar(c, x)
		ui.SplitVertical(c.Key("split"), &x.split, func() {
			ui.Column(c.Key("sql")).Fill().Children(func() {
				x.view.Build(c).Grow(1)
				has = x.view.Focused(c)
			})
		}, func() {
			ui.Column(c.Key("results")).Fill().Children(func() { b.results(c, x) })
		}).Grow(1).Basis(0).MinHeight(0)
	})
	if focused && !has && b.focused != t {
		b.focused = t
		x.view.Focus()
	}
	// Cmd+Enter, Cmd+Shift+Enter, Esc, Cmd+E (R-DB shortcuts).
	if c.Shortcut(ui.Cmd, ui.KeyEnter) {
		b.runFromUI(x, false)
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyEnter) {
		b.runFromUI(x, true)
	}
	if x.cancel != nil && c.Shortcut(0, ui.KeyEscape) {
		x.cancel()
	}
	if c.Shortcut(ui.Cmd, ui.KeyE) {
		b.export(x, "csv")
	}
	return has
}

func (b *Bench) runFromUI(x *tab, all bool) {
	c := b.Conns.Get(readData(x.t).Connection)
	if c == nil {
		b.toast("Choose a Connection")
		return
	}
	stmts := x.statementsToRun(all)
	if len(stmts) == 0 {
		return
	}
	var asks []string
	if err := Check(c, stmts, func(stmt, why string) bool {
		asks = append(asks, why+":\n"+stmt)
		return true
	}); err != nil {
		x.messages = []string{err.Error()}
		return
	}
	run := func() {
		limit := 0
		if x.limit {
			limit = 500
		}
		if err := b.Run(x.t, stmts, limit, int(x.timeout), nil); err != nil {
			b.toast(err.Error())
		}
	}
	if len(asks) == 0 {
		run()
		return
	}
	b.confirm(fmt.Sprintf("Run on “%s”?\n\n%s", c.Name, strings.Join(asks, "\n\n")), "Run", run) // R-DB-052
}

// toolbar is the Connection, Run, Run all, Cancel, read-only, Limit,
// timeout, History, Save (R-DB-043).
func (b *Bench) toolbar(c *ui.Context, x *tab) {
	th := c.Theme()
	d := readData(x.t)
	conns := b.Connections(x.t.Bench)
	ui.Row(c.Key("bar")).Height(38).Padding(0, 8).Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(th.Border).Children(func() {
		names := []string{}
		cur := ""
		var conn *dbconn.Connection
		for _, cn := range conns {
			names = append(names, cn.Name)
			if cn.ID == d.Connection {
				cur, conn = cn.Name, cn
			}
		}
		if conn != nil {
			ui.Box(c).Size(8, 8).Radius(4).Background(dotColor(th, conn.Color))
		}
		if len(names) == 0 {
			ui.Text(c, "No Connection: + in the Panel").TextColor(th.TextMuted)
		} else if ui.Select(c.Key("conn"), &cur, names).Width(180).Changed() {
			for _, cn := range conns {
				if cn.Name == cur {
					d.Connection = cn.ID
					writeData(x.t, d)
					b.St.Changed()
				}
			}
		}
		if x.cancel != nil {
			ui.Spinner(c).Size(14, 14)
			if kit.IconButton(c, "cancel", "Cancel", kit.Stop, false).Clicked() {
				x.cancel()
			}
		} else {
			if kit.PrimaryIcon(c, "run", "Run (⌘↩ the selection or the statement at the cursor)", kit.Play, "Run").Clicked() {
				b.runFromUI(x, false)
			}
			if kit.IconButton(c, "all", "Run all (⇧⌘↩)", kit.PlayAll, false).Clicked() {
				b.runFromUI(x, true)
			}
		}
		if conn != nil && conn.ReadOnly {
			ui.Text(c, "read-only").FontSize(11).Padding(1, 6).Radius(4).Background(th.Accent.Alpha(0.15)).TextColor(th.Accent)
		}
		ui.Box(c).Grow(1)
		ui.Text(c, "Limit 500").FontSize(12).TextColor(th.TextMuted)
		ui.Switch(c.Key("limit"), &x.limit).Label("Limit 500")
		ui.Text(c, "Timeout").FontSize(12).TextColor(th.TextMuted)
		ui.NumberInput(c.Key("timeout"), &x.timeout, 1, 3600, 5).Width(120).Shrink(0)
		ui.Box(c).Width(8).Shrink(0)
		if kit.IconButton(c, "hist", "History", kit.History, x.history).Clicked() {
			x.history = !x.history
		}
		if x.doc.String() != x.saved || d.File == "" {
			if kit.IconButton(c, "save", "Save (⌘S)", kit.Save, false).Clicked() {
				b.Save(x.t)
			}
		}
	})
}

func dotColor(th *ui.Theme, c string) ui.Color {
	if c == "" {
		return th.TextMuted
	}
	return ui.Hex(c)
}

// results are the Result tabs, the grid and the messages (R-DB-060 … R-DB-070).
func (b *Bench) results(c *ui.Context, x *tab) {
	th := c.Theme()
	if x.history {
		b.historyList(c, x)
		return
	}
	if len(x.results) > 0 {
		ui.Row(c.Key("rtabs")).Padding(4, 8).Gap(6).AlignItems(ui.Center).Children(func() {
			if len(x.results) > 1 {
				labels := make([]string, len(x.results))
				for i := range labels {
					labels[i] = fmt.Sprint(i + 1)
				}
				ui.Segmented(c.Key("seg"), &x.shown, labels...)
			}
			r := x.results[x.shown]
			ui.Text(c, fmt.Sprintf("%d rows, %d ms", len(r.Rows), r.Ms)).FontSize(12).TextColor(th.TextMuted)
			ui.Box(c).Grow(1)
			kit.IconButton(c, "export", "Export", kit.Download, false).Menu(func(m *ui.Menu) {
				for _, f := range []string{"CSV", "JSON", "SQL INSERTs"} {
					if m.Item(f).Chosen() {
						b.export(x, strings.ToLower(strings.Fields(f)[0]))
					}
				}
			})
		})
		b.grid(c, x, x.results[x.shown])
	} else if x.cancel == nil && len(x.messages) == 0 {
		ui.Column(c.Key("none")).Grow(1).Center().Children(func() {
			ui.Text(c, "Run a statement (⌘↩)").TextColor(th.TextMuted)
		})
	}
	if len(x.messages) > 0 {
		ui.Column(c.Key("msgs")).Padding(4, 10).Gap(2).MaxHeight(120).BorderWidth(1, 0, 0, 0).BorderColor(th.Border).Children(func() {
			for i, m := range x.messages {
				col := th.TextMuted
				if strings.HasPrefix(m, "Error") || m == ErrRefused.Error() {
					col = th.Danger
				}
				ui.Text(c.Key(i), m).FontSize(12).TextColor(col).Selectable()
			}
		})
	}
	if x.cell != nil {
		open := true
		ui.Modal(c.Key("cell"), &open, func() {
			ui.Column(c).Width(560).Gap(8).Padding(12).Children(func() {
				text := *x.cell
				var buf bytes.Buffer
				if json.Indent(&buf, []byte(text), "", "  ") == nil {
					text = buf.String()
				}
				ui.Scroll(c).MaxHeight(420).Children(func() {
					ui.Text(c, text).Font("SF Mono, Menlo").FontSize(12).Selectable()
				})
				ui.Row(c).Gap(8).Children(func() {
					ui.Box(c).Grow(1)
					if kit.IconButton(c, "copy", "Copy", kit.Copy, false).Clicked() {
						mygo.Clipboard.WriteText(*x.cell)
					}
					if ui.PrimaryButton(c, "Close").Clicked() {
						open = false
					}
				})
			})
		})
		if !open {
			x.cell = nil
		}
	}
}

// visibleRows applies the filters and the sort to loaded rows (R-DB-063,
// R-DB-064); it returns row indexes.
func visibleRows(r *dbconn.Result, filters []string, sortBy ui.SortOrder) []int {
	idx := make([]int, 0, len(r.Rows))
rows:
	for i, row := range r.Rows {
		for j, f := range filters {
			if f != "" && j < len(row) && !strings.Contains(strings.ToLower(row[j].Text), strings.ToLower(f)) {
				continue rows
			}
		}
		idx = append(idx, i)
	}
	if sortBy.Column != "" {
		col := -1
		for i, cn := range r.Columns {
			if fmt.Sprint(i) == sortBy.Column {
				col = i
			}
			_ = cn
		}
		if col >= 0 {
			sort.SliceStable(idx, func(a, b int) bool {
				x, y := r.Rows[idx[a]][col], r.Rows[idx[b]][col]
				less := lessCell(x, y)
				if sortBy.Descending {
					return lessCell(y, x)
				}
				return less
			})
		}
	}
	return idx
}

func lessCell(a, b dbconn.Cell) bool {
	if a.Null != b.Null {
		return a.Null
	}
	var fa, fb float64
	if _, e1 := fmt.Sscan(a.Text, &fa); e1 == nil {
		if _, e2 := fmt.Sscan(b.Text, &fb); e2 == nil {
			return fa < fb
		}
	}
	return a.Text < b.Text
}

// grid draws the rows in view only (R-DB-062), with a filter per column.
func (b *Bench) grid(c *ui.Context, x *tab, r *dbconn.Result) {
	th := c.Theme()
	if len(x.filters) != len(r.Columns) {
		x.filters = make([]string, len(r.Columns))
	}
	cols := make([]ui.TableColumn, len(r.Columns))
	for i, cn := range r.Columns {
		cols[i] = ui.TableColumn{Title: cn.Name + "  " + cn.Type, ID: fmt.Sprint(i), Width: 160, MinWidth: 60, Sortable: true}
	}
	ui.Row(c.Key("filters")).Padding(0, 8).Gap(2).Children(func() {
		for i := range x.filters {
			ui.TextInput(c.Key(i), &x.filters[i]).Placeholder("filter").FontSize(11).Width(160)
		}
	})
	rows := visibleRows(r, x.filters, x.sort)
	tb := ui.Table(c.Key("grid"), &x.grid, cols, len(rows), func(row, col int) {
		cell := r.Rows[rows[row]][col]
		switch {
		case cell.Null:
			ui.Text(c, "NULL").Italic().FontSize(12).TextColor(th.TextMuted)
		default:
			text := cell.Text
			if utf := []rune(text); len(utf) > 200 {
				text = string(utf[:200]) + "…" // R-DB-066
			}
			ui.Text(c, strings.ReplaceAll(text, "\n", " ")).FontSize(12).SingleLine()
		}
	}).Grow(1).Basis(0).MinHeight(0)
	if tb.Submitted() && x.sel >= 0 && x.sel < len(rows) {
		row := r.Rows[rows[x.sel]]
		text := rowJSON(r, row)
		x.cell = &text
	}
	tb.ContextMenu(func(m *ui.Menu) {
		if x.sel < 0 || x.sel >= len(rows) {
			return
		}
		row := r.Rows[rows[x.sel]]
		if m.Item("Copy row as JSON").Chosen() {
			mygo.Clipboard.WriteText(rowJSON(r, row))
		}
		if m.Item("Copy row as TSV").Chosen() {
			var parts []string
			for _, c := range row {
				parts = append(parts, c.Text)
			}
			mygo.Clipboard.WriteText(strings.Join(parts, "\t"))
		}
		m.Submenu("Copy column", func(sm *ui.Menu) {
			for i, cn := range r.Columns {
				if sm.Item(cn.Name).Chosen() {
					var vals []string
					for _, ri := range rows {
						vals = append(vals, r.Rows[ri][i].Text)
					}
					mygo.Clipboard.WriteText(strings.Join(vals, "\n"))
				}
			}
		})
		m.Submenu("Show cell", func(sm *ui.Menu) {
			for i, cn := range r.Columns {
				if sm.Item(cn.Name).Chosen() {
					v := row[i].Text
					x.cell = &v
				}
			}
		})
		if m.Item("Copy as INSERT").Chosen() {
			mygo.Clipboard.WriteText(insert(r, [][]dbconn.Cell{row}))
		}
	})
	if r.Limited {
		ui.Row(c.Key("more")).Padding(4, 10).Gap(8).AlignItems(ui.Center).Background(th.Surface).Children(func() {
			ui.Text(c, fmt.Sprintf("%d rows shown", len(r.Rows))).Grow(1)
			if ui.Button(c, "Load more").Clicked() {
				b.loadMore(x, r)
			}
		})
	}
}

func rowJSON(r *dbconn.Result, row []dbconn.Cell) string {
	m := map[string]any{}
	for i, c := range r.Columns {
		if row[i].Null {
			m[c.Name] = nil
		} else {
			m[c.Name] = row[i].Text
		}
	}
	out, _ := json.MarshalIndent(m, "", "  ")
	return string(out)
}

func sqlValue(c dbconn.Cell) string {
	if c.Null {
		return "NULL"
	}
	return "'" + strings.ReplaceAll(c.Text, "'", "''") + "'"
}

// insert writes rows as INSERT statements.
func insert(r *dbconn.Result, rows [][]dbconn.Cell) string {
	table := "table_name"
	if f := strings.Fields(r.SQL); len(f) > 3 && strings.EqualFold(f[2], "FROM") {
		table = f[3]
	}
	names := make([]string, len(r.Columns))
	for i, c := range r.Columns {
		names[i] = c.Name
	}
	var b strings.Builder
	for _, row := range rows {
		vals := make([]string, len(row))
		for i, c := range row {
			vals[i] = sqlValue(c)
		}
		fmt.Fprintf(&b, "INSERT INTO %s (%s) VALUES (%s);\n", table, strings.Join(names, ", "), strings.Join(vals, ", "))
	}
	return b.String()
}

// Export writes the loaded rows as CSV, JSON or INSERTs (R-DB-068).
func Export(r *dbconn.Result, format string) []byte {
	switch format {
	case "json":
		var out []map[string]any
		for _, row := range r.Rows {
			m := map[string]any{}
			for i, c := range r.Columns {
				if row[i].Null {
					m[c.Name] = nil
				} else {
					m[c.Name] = row[i].Text
				}
			}
			out = append(out, m)
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		return data
	case "sql":
		return []byte(insert(r, r.Rows))
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	names := make([]string, len(r.Columns))
	for i, c := range r.Columns {
		names[i] = c.Name
	}
	_ = w.Write(names)
	for _, row := range r.Rows {
		rec := make([]string, len(row))
		for i, c := range row {
			rec[i] = c.Text
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return buf.Bytes()
}

func (b *Bench) export(x *tab, format string) {
	if len(x.results) == 0 {
		return
	}
	r := x.results[x.shown]
	ext := map[string]string{"csv": ".csv", "json": ".json", "sql": ".sql"}[format]
	p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: b.Win, DefaultPath: "result" + ext})
	if err != nil || p == "" {
		return
	}
	if err := os.WriteFile(p, Export(r, format), 0o644); err != nil {
		b.toast(err.Error())
	}
}

func (b *Bench) historyList(c *ui.Context, x *tab) {
	th := c.Theme()
	ui.Scroll(c.Key("hist")).Grow(1).Basis(0).MinHeight(0).Padding(4, 10).Children(func() {
		list := dbconn.History(b.Dir+"/db/history", readData(x.t).Connection)
		if len(list) == 0 {
			ui.Text(c, "No history yet").TextColor(th.TextMuted)
		}
		for i, e := range list {
			e := e
			row := ui.Row(c.Key(i)).Gap(10).Padding(3, 6).Radius(5).Cursor(ui.CursorPointer)
			if row.Hovered() {
				row.Background(th.SurfaceHover)
			}
			row.Children(func() {
				ui.Text(c, e.At.Format("01-02 15:04")).TextColor(th.TextMuted).FontSize(12)
				ui.Text(c, strings.Join(strings.Fields(e.SQL), " ")).SingleLine().Grow(1).Shrink(1).MinWidth(0).FontSize(12)
				info := fmt.Sprintf("%d ms", e.Ms)
				if e.Error != "" {
					info = "error"
				}
				ui.Text(c, info).FontSize(12).TextColor(th.TextMuted)
			})
			if row.Clicked() { // R-DB-081: insert at the cursor
				x.doc.Paste(e.SQL)
				x.history = false
			}
		}
	})
}

// CloseQuestion asks before closing a Query with unsaved changes to its file.
func (b *Bench) CloseQuestion(t *model.Tab) string {
	if x := b.tabs[t]; x != nil && readData(t).File != "" && x.doc.String() != x.saved {
		return "Close “" + t.DisplayTitle() + "” without saving?"
	}
	return ""
}

// Closed stops the Tab's statement.
func (b *Bench) Closed(t *model.Tab) {
	b.Cancel(t)
	delete(b.tabs, t)
}

// Status is the Connection and the last Result.
func (b *Bench) Status(t *model.Tab) string {
	c := b.Conns.Get(readData(t).Connection)
	if c == nil {
		return "No Connection"
	}
	s := c.Name + " · " + c.Engine + " · " + b.State(c.ID)
	if r := b.LastResult(t); r != nil {
		s += " · " + r.Message()
	}
	return s
}

// Glyph marks a running statement.
func (b *Bench) Glyph(t *model.Tab) (string, bool) {
	if x := b.tabs[t]; x != nil && x.cancel != nil {
		return "⋯", false
	}
	return "", false
}

func (b *Bench) Zoom(*model.Bench, int) {}

// Command takes the menu's save.
func (b *Bench) Command(bench *model.Bench, t *model.Tab, name string) bool {
	if t != nil && name == "save" {
		b.Save(t)
		return true
	}
	return false
}
