package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/dbconn"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// stateColor is the color of a Connection's state: brighter in dark.
func stateColor(th *ui.Theme, state string) ui.Color {
	light := map[string]string{"connected": "#2e9e58", "connecting": "#c9a227", "error": "#d64545"}
	dark := map[string]string{"connected": "#4cc27a", "connecting": "#e0bd45", "error": "#f06a6a"}
	m := light
	if th.Dark {
		m = dark
	}
	h, ok := m[state]
	if !ok {
		return th.TextMuted
	}
	return ui.Hex(h)
}

// Panel lists the Connections with their schema trees and the saved
// Queries (R-DB-030 … R-DB-037, R-DB-055).
func (b *Bench) Panel(c *ui.Context, bench *model.Bench) {
	th := c.Theme()
	p := b.panel[bench]
	if p == nil {
		p = &panelState{}
		b.panel[bench] = p
	}
	ui.Column(c.Key("dbpanel")).Grow(1).Basis(0).MinHeight(0).Padding(4, 6).Gap(4).AlignItems(ui.Stretch).Children(func() {
		kit.PanelHead(c, &p.query, "Search tables", func() {
			if kit.IconButton(c, "new", "New Connection", kit.Plus, false).Clicked() {
				b.EditConnection(bench, nil)
			}
		})
		ui.Scroll(c.Key("tree")).Grow(1).Basis(0).MinHeight(0).Children(func() {
			conns := b.Connections(bench)
			if len(conns) == 0 {
				ui.Text(c, "No Connections. + adds one.").FontSize(12).TextColor(th.TextMuted)
			}
			for _, cn := range conns {
				b.connRow(c, bench, p, cn)
			}
			b.saved(c, bench, p)
		})
	})
	if c.Shortcut(ui.Cmd, ui.KeyR) && p.sel != "" {
		b.Refresh(p.sel) // R-DB-034
	}
}

func (b *Bench) connRow(c *ui.Context, bench *model.Bench, p *panelState, cn *dbconn.Connection) {
	th := c.Theme()
	l := b.liveOf(cn)
	open := l.open[""]
	r := kit.Row(c, "c"+cn.ID, 0, p.sel == cn.ID)
	r.Children(func() {
		kit.Chevron(c, open)
		ui.Box(c).Size(8, 8).Radius(4).Background(dotColor(th, cn.Color))
		ui.Text(c, cn.Name).Bold().SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if l.state != "idle" {
			ui.Text(c, l.state).FontSize(10).TextColor(stateColor(th, l.state))
		}
	})
	if r.Clicked() {
		p.sel = cn.ID
		l.open[""] = !open
		if !open {
			b.Open(bench, cn, func(*dbconn.DB, error) {})
		}
	}
	r.ContextMenu(func(m *ui.Menu) {
		if m.Item("New Query").Chosen() {
			b.Show(b.OpenQuery(bench, cn.ID, "", false))
		}
		if m.Item("Edit…").Chosen() {
			b.EditConnection(bench, cn)
		}
		if m.Item("Refresh").Chosen() {
			b.Refresh(cn.ID)
		}
		if m.Item("Disconnect").Chosen() {
			b.Disconnect(cn.ID)
		}
		m.Separator()
		if m.Item("Delete…").Chosen() { // R-DB-011
			b.confirm("Delete the Connection “"+cn.Name+"” and its password?", "Delete", func() {
				b.Disconnect(cn.ID)
				b.Conns.Remove(cn.ID)
				delete(b.live, cn.ID)
			})
		}
	})
	if !open {
		return
	}
	if l.state == "error" {
		ui.Text(c.Key("err"+cn.ID), l.err).FontSize(11).TextColor(th.Danger).Padding(0, 6, 0, 26)
		return
	}
	if l.db == nil {
		return
	}
	if l.schemas == nil {
		b.load(l, "schemas", func(ctx context.Context, d *dbconn.DB) error {
			s, err := d.Schemas(ctx)
			b.Post(func() { l.schemas = s })
			if err == nil && s == nil {
				b.Post(func() { l.schemas = []string{} })
			}
			return err
		})
		if l.loading["schemas"] {
			ui.Spinner(c.Key("sp"+cn.ID)).Size(12, 12)
		}
		return
	}
	q := strings.ToLower(p.query)
	for _, sc := range l.schemas {
		sc := sc
		key := sc
		sOpen := l.open[key] || q != "" || len(l.schemas) == 1
		objs, loaded := l.objects[sc]
		if (sOpen || q != "") && !loaded {
			b.load(l, "o"+sc, func(ctx context.Context, d *dbconn.DB) error {
				o, err := d.Objects(ctx, sc)
				b.Post(func() {
					if o == nil {
						o = []dbconn.Object{}
					}
					l.objects[sc] = o
				})
				return err
			})
		}
		var hits []dbconn.Object
		for _, o := range objs {
			if q == "" || strings.Contains(strings.ToLower(o.Name), q) {
				hits = append(hits, o)
			}
		}
		if q != "" && len(hits) == 0 {
			continue // R-DB-033
		}
		if len(l.schemas) > 1 {
			sr := kit.Row(c, cn.ID+"/"+sc, 1, false)
			sr.Children(func() {
				kit.Chevron(c, sOpen)
				ui.Text(c, sc).SingleLine().Grow(1)
				if l.loading["o"+sc] {
					ui.Spinner(c).Size(10, 10)
				}
			})
			if sr.Clicked() {
				l.open[key] = !l.open[key]
			}
			if !sOpen {
				continue
			}
		}
		depth := 2
		if len(l.schemas) == 1 {
			depth = 1
		}
		for _, group := range []string{"table", "view", "function"} {
			var list []dbconn.Object
			for _, o := range hits {
				if o.Kind == group {
					list = append(list, o)
				}
			}
			if len(list) == 0 {
				continue
			}
			gkey := sc + "/" + group
			gOpen := l.open[gkey] || q != "" || group == "table" && !l.open[gkey+"/closed"]
			names := map[string]string{"table": "Tables", "view": "Views", "function": "Functions"}
			if cn.Engine == dbconn.MySQL && group == "function" {
				names[group] = "Routines"
			}
			gr := kit.Row(c, cn.ID+"/"+gkey, depth, false)
			gr.Children(func() {
				kit.Chevron(c, gOpen)
				ui.Text(c, names[group]).SingleLine().Grow(1).TextColor(th.TextMuted)
				ui.Text(c, fmt.Sprint(len(list))).FontSize(11).TextColor(th.TextMuted)
			})
			if gr.Clicked() {
				if group == "table" {
					l.open[gkey+"/closed"] = gOpen
				}
				l.open[gkey] = !gOpen
			}
			if !gOpen {
				continue
			}
			for _, o := range list {
				b.objectRow(c, bench, l, cn, sc, o, depth+1)
			}
		}
	}
}

func (b *Bench) objectRow(c *ui.Context, bench *model.Bench, l *live, cn *dbconn.Connection, sc string, o dbconn.Object, depth int) {
	th := c.Theme()
	key := sc + "." + o.Name
	open := l.open[key]
	r := kit.Row(c, cn.ID+"/"+key, depth, false)
	r.Children(func() {
		if o.Kind == "function" {
			ui.Text(c, "ƒ").FontSize(11).TextColor(kit.Faint(th)).Width(12).Shrink(0)
		} else {
			kit.Chevron(c, open)
		}
		ui.Text(c, o.Name).SingleLine().Grow(1).Shrink(1).MinWidth(0)
	})
	qualified := dbconn.Quote(cn.Engine, schemaFor(cn, sc), o.Name)
	if r.DoubleClicked() && o.Kind != "function" { // R-DB-037
		b.selectRows(bench, cn, qualified)
		return
	}
	if r.Clicked() && o.Kind != "function" {
		l.open[key] = !open
	}
	if o.Kind != "function" {
		r.ContextMenu(func(m *ui.Menu) { // R-DB-035
			if m.Item("Select 500 rows").Chosen() {
				b.selectRows(bench, cn, qualified)
			}
			if m.Item("Count").Chosen() {
				b.count(bench, cn, qualified)
			}
			if m.Item("Show DDL").Chosen() {
				b.showDDL(bench, cn, sc, o.Name)
			}
			if m.Item("Copy name").Chosen() {
				mygo.Clipboard.WriteText(qualified)
			}
		})
	}
	if !open || o.Kind == "function" {
		return
	}
	cols, ok := l.columns[key]
	if !ok {
		b.load(l, "c"+key, func(ctx context.Context, d *dbconn.DB) error {
			cs, err := d.Columns(ctx, schemaFor(cn, sc), o.Name)
			ix, _ := d.Indexes(ctx, schemaFor(cn, sc), o.Name)
			b.Post(func() {
				if cs == nil {
					cs = []dbconn.ColumnInfo{}
				}
				l.columns[key], l.indexes[key] = cs, ix
			})
			return err
		})
		return
	}
	for _, col := range cols {
		col := col
		cr := kit.Row(c, cn.ID+"/"+key+"/"+col.Name, depth+1, false)
		cr.Children(func() {
			mark := ""
			if col.PK {
				mark = "PK"
			} else if col.FK != "" {
				mark = "FK"
			}
			ui.Text(c, mark).FontSize(9).Bold().TextColor(th.Accent).Width(16)
			ui.Text(c, col.Name).SingleLine().Shrink(1).MinWidth(0)
			null := ""
			if !col.Nullable {
				null = " NOT NULL"
			}
			ui.Text(c, col.Type+null).FontSize(11).TextColor(th.TextMuted).SingleLine().Shrink(1).MinWidth(0)
		})
		if cr.DoubleClicked() {
			mygo.Clipboard.WriteText(col.Name)
		}
		if col.FK != "" {
			cr.Tooltip("→ " + col.FK)
		}
	}
	for _, ix := range l.indexes[key] {
		u := ""
		if ix.Unique {
			u = " unique"
		}
		kit.Row(c, cn.ID+"/"+key+"/ix/"+ix.Name, depth+1, false).Children(func() {
			ui.Text(c, "ix").FontSize(9).TextColor(th.TextMuted).Width(16)
			ui.Text(c, ix.Name+" ("+ix.Columns+")"+u).FontSize(11).TextColor(th.TextMuted).SingleLine()
		})
	}
}

// schemaFor is the schema a query names: SQLite's is empty.
func schemaFor(cn *dbconn.Connection, sc string) string {
	if cn.Engine == dbconn.SQLite {
		return ""
	}
	return sc
}

func (b *Bench) selectRows(bench *model.Bench, cn *dbconn.Connection, qualified string) {
	sql := "SELECT * FROM " + qualified + " LIMIT 500"
	t := b.OpenQuery(bench, cn.ID, sql, false)
	b.Show(t)
	b.liveTab(false, t)
	_ = b.Run(t, []dbconn.Statement{{SQL: sql}}, 0, 30, nil)
}

func (b *Bench) count(bench *model.Bench, cn *dbconn.Connection, qualified string) {
	b.Open(bench, cn, func(d *dbconn.DB, err error) {
		if err != nil {
			b.toast(err.Error())
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			r := d.Run(ctx, "SELECT count(*) FROM "+qualified, 0, 0)
			b.Post(func() {
				if r.Err != "" {
					b.toast(r.Err)
				} else if len(r.Rows) > 0 {
					b.toast(fmt.Sprintf("%s: %s rows", qualified, r.Rows[0][0].Text))
				}
			})
		}()
	})
}

func (b *Bench) showDDL(bench *model.Bench, cn *dbconn.Connection, sc, table string) {
	b.Open(bench, cn, func(d *dbconn.DB, err error) {
		if err != nil {
			b.toast(err.Error())
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			ddl, err := d.DDL(ctx, schemaFor(cn, sc), table)
			b.Post(func() {
				if err != nil {
					b.toast(err.Error())
					return
				}
				t := b.OpenQuery(bench, cn.ID, ddl, true)
				t.Title = table + " DDL"
				b.Show(t)
			})
		}()
	})
}

// saved lists the Bench folder's .sql files (R-DB-055).
func (b *Bench) saved(c *ui.Context, bench *model.Bench, p *panelState) {
	entries, _ := os.ReadDir(b.Folder(bench))
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		return
	}
	kit.Caption(c, "Saved").Padding(8, 8, 2, 8)
	for _, f := range files {
		path := filepath.Join(b.Folder(bench), f)
		r := kit.Row(c, "s"+f, 0, false)
		r.Children(func() { ui.Text(c, f).SingleLine() })
		if r.Clicked() {
			for _, t := range bench.Tabs {
				if readData(t).File == path {
					b.Show(t)
					return
				}
			}
			t := &model.Tab{ID: model.NewID(), Bench: bench, Data: "{}"}
			d := data{Connection: p.sel, File: path}
			if d.Connection == "" {
				if list := b.Connections(bench); len(list) > 0 {
					d.Connection = list[0].ID
				}
			}
			writeData(t, d)
			t.Title = f
			bench.Tabs = append(bench.Tabs, t)
			b.St.Changed()
			b.Show(t)
		}
	}
}
