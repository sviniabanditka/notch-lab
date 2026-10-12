// Package db is the db Bench: Connections in the Panel with their schema,
// a Tab is one Query with its Results (docs/spec/09-db.md).
package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/dbconn"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
)

// Bench draws every db Tab. It belongs to the main goroutine.
type Bench struct {
	St   *model.State
	Home string
	Dir  string // NOTCH_LAB_HOME
	Post func(func())
	Show func(t *model.Tab)
	// Toast, Confirm and AskText are the Shell's.
	Toast   func(string)
	Confirm func(question, yes string) bool
	AskText func(title, current string, check func(string) string)
	// Visible reports whether a Tab shows in a Leaf now (R-DB-071).
	Visible func(t *model.Tab) bool
	Win     *mygo.Window
	// TunnelFor resolves a Tunnel that names an ssh Bench Host (R-DB-009).
	TunnelFor func(name string) (spec, identity string, ok bool)

	Conns   *dbconn.Store
	live    map[string]*live
	tabs    map[*model.Tab]*tab
	pal     [2]*syntax.Palette
	sheet   *sheet
	panel   map[*model.Bench]*panelState
	focused *model.Tab
	mu      sync.Mutex
}

// live is a Connection's open database and its loaded tree.
type live struct {
	c       *dbconn.Connection
	db      *dbconn.DB
	state   string // idle, connecting, connected, error
	err     string
	waiters []func(*dbconn.DB, error)
	schemas []string
	objects map[string][]dbconn.Object
	columns map[string][]dbconn.ColumnInfo
	indexes map[string][]dbconn.Index
	loading map[string]bool
	open    map[string]bool
}

type panelState struct {
	query string
	sel   string // the selected Connection id
}

// New returns the db Bench.
func New(st *model.State, home, dir string) *Bench {
	return &Bench{St: st, Home: home, Dir: dir, Conns: dbconn.Load(filepath.Join(dir, "db", "connections.json")),
		live: map[string]*live{}, tabs: map[*model.Tab]*tab{}, panel: map[*model.Bench]*panelState{}}
}

func (b *Bench) toast(s string) {
	if b.Toast != nil {
		b.Toast(s)
	}
}

func (b *Bench) confirm(q, yes string) bool { return b.Confirm == nil || b.Confirm(q, yes) }

// Connections lists the Connections of a Bench's Lab, in order.
func (b *Bench) Connections(bench *model.Bench) []*dbconn.Connection {
	var out []*dbconn.Connection
	for _, c := range b.Conns.List {
		if c.Lab == "" || bench.Lab == nil || c.Lab == bench.Lab.ID {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

func (b *Bench) liveOf(c *dbconn.Connection) *live {
	l := b.live[c.ID]
	if l == nil {
		l = &live{c: c, state: "idle", objects: map[string][]dbconn.Object{}, columns: map[string][]dbconn.ColumnInfo{},
			indexes: map[string][]dbconn.Index{}, loading: map[string]bool{}, open: map[string]bool{}}
		b.live[c.ID] = l
	}
	l.c = c
	return l
}

// root resolves a SQLite path: the Bench's effective root folder (R-DB-004).
func (b *Bench) root(bench *model.Bench) string {
	if bench == nil {
		return b.Home
	}
	return bench.EffectiveRoot(b.Home)
}

// Open connects a Connection, once; done runs on the main goroutine.
func (b *Bench) Open(bench *model.Bench, c *dbconn.Connection, done func(*dbconn.DB, error)) {
	l := b.liveOf(c)
	if l.db != nil {
		done(l.db, nil)
		return
	}
	l.waiters = append(l.waiters, done)
	if l.state == "connecting" {
		return
	}
	l.state, l.err = "connecting", ""
	conn, root := *c, b.root(bench)
	if conn.Tunnel != "" && b.TunnelFor != nil {
		if spec, id, ok := b.TunnelFor(conn.Tunnel); ok {
			conn.Tunnel = spec
			if conn.Identity == "" {
				conn.Identity = id
			}
		}
	}
	go func() {
		d, err := dbconn.Open(context.Background(), &conn, root, conn.Password())
		b.Post(func() {
			if err != nil {
				l.state, l.err = "error", err.Error() // R-DB-021
			} else {
				l.db, l.state = d, "connected"
			}
			ws := l.waiters
			l.waiters = nil
			for _, w := range ws {
				w(l.db, err)
			}
		})
	}()
}

// Disconnect closes a Connection's database (R-DB-023).
func (b *Bench) Disconnect(id string) {
	if l := b.live[id]; l != nil {
		if l.db != nil {
			go l.db.Close()
		}
		l.db, l.state, l.schemas = nil, "idle", nil
		l.objects, l.columns, l.indexes = map[string][]dbconn.Object{}, map[string][]dbconn.ColumnInfo{}, map[string][]dbconn.Index{}
	}
}

// State is a Connection's state for lists (R-DB-030).
func (b *Bench) State(id string) string {
	if l := b.live[id]; l != nil {
		return l.state
	}
	return "idle"
}

// load reads one level of the tree off the main goroutine (R-DB-032).
func (b *Bench) load(l *live, key string, read func(ctx context.Context, d *dbconn.DB) error) {
	if l.loading[key] || l.db == nil {
		return
	}
	l.loading[key] = true
	d := l.db
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := read(ctx, d)
		b.Post(func() {
			delete(l.loading, key)
			if err != nil {
				l.err = err.Error()
			}
		})
	}()
}

// Refresh reads the tree again, keeping what is open (R-DB-034).
func (b *Bench) Refresh(id string) {
	if l := b.live[id]; l != nil {
		l.schemas = nil
		l.objects, l.columns, l.indexes = map[string][]dbconn.Object{}, map[string][]dbconn.ColumnInfo{}, map[string][]dbconn.Index{}
	}
}

// Folder is the Bench folder for saved Queries (R-DB-055): R-HTTP-002 with db.
func (b *Bench) Folder(bench *model.Bench) string {
	var s struct {
		Folder string `json:"folder"`
	}
	_ = json.Unmarshal([]byte(bench.Settings), &s)
	if s.Folder != "" {
		return s.Folder
	}
	if root := bench.EffectiveRoot(b.Home); root != "" && root != b.Home {
		return filepath.Join(root, ".db")
	}
	return filepath.Join(b.Dir, "db", bench.ID)
}

func (b *Bench) palette(dark bool) *syntax.Palette {
	i := 0
	if dark {
		i = 1
	}
	if b.pal[i] == nil {
		if dark {
			b.pal[i] = syntax.NewPalette("github-dark", ui.RGB(0xe6, 0xed, 0xf3))
		} else {
			b.pal[i] = syntax.NewPalette("github", ui.RGB(0x1f, 0x23, 0x28))
		}
	}
	return b.pal[i]
}

func lower(s string) string { return strings.ToLower(s) }
