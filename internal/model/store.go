package model

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store keeps the State in SQLite (R-MODEL-050 … R-MODEL-056).
type Store struct {
	db   *sql.DB
	path string
	mu   sync.Mutex // serializes writes

	pending   bool   // main goroutine only
	backupDay string // the day of the last backup; guarded by mu
}

// ErrNewer means the database was written by a newer notch-lab (R-MODEL-055).
var ErrNewer = errors.New("This state was written by a newer notch-lab")

const schemaVersion = 2

var migrations = []string{
	// 0 → 1
	`CREATE TABLE lab   (id TEXT PRIMARY KEY, name TEXT NOT NULL, root TEXT, color TEXT, ord INTEGER NOT NULL);
	 CREATE TABLE bench (id TEXT PRIMARY KEY, lab TEXT NOT NULL REFERENCES lab(id) ON DELETE CASCADE,
	                     type TEXT NOT NULL, name TEXT NOT NULL, root TEXT, icon TEXT, ord INTEGER NOT NULL,
	                     layout TEXT NOT NULL, settings TEXT NOT NULL DEFAULT '{}');
	 CREATE TABLE tab   (id TEXT PRIMARY KEY, bench TEXT NOT NULL REFERENCES bench(id) ON DELETE CASCADE,
	                     title TEXT, custom_title TEXT, ord INTEGER NOT NULL, pinned INTEGER NOT NULL DEFAULT 0,
	                     unread INTEGER NOT NULL DEFAULT 0, data TEXT NOT NULL DEFAULT '{}');
	 CREATE TABLE closed_tab (bench TEXT NOT NULL, closed_at INTEGER NOT NULL, tab TEXT NOT NULL);
	 CREATE TABLE window (key TEXT PRIMARY KEY, lab TEXT, bench TEXT, focused_tab TEXT, frame TEXT, sidebar INTEGER);
	 CREATE TABLE kv    (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
	// 1 → 2: browser history (R-WEB-080)
	`CREATE TABLE browser_history (url TEXT PRIMARY KEY, title TEXT, visited_at INTEGER NOT NULL, count INTEGER NOT NULL DEFAULT 1, profile TEXT NOT NULL);
	 CREATE INDEX browser_history_visited ON browser_history(visited_at);`,
}

// Open opens or creates the database at path and loads the State. fresh
// reports a database that did not exist (first run, R-SHELL-180).
func Open(path string) (store *Store, st *State, fresh bool, err error) {
	_, statErr := os.Stat(path)
	fresh = errors.Is(statErr, os.ErrNotExist)
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, nil, false, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, nil, false, err
	}
	st, err = s.load()
	if err != nil {
		db.Close()
		return nil, nil, false, err
	}
	return s, st, fresh, nil
}

// DB is the state database, for tables a Bench keeps itself (browser
// history); the State's own tables are the Store's.
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > schemaVersion {
		return ErrNewer
	}
	for ; v < schemaVersion; v++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate to %d: %w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) load() (*State, error) {
	st := New()
	labs := map[string]*Lab{}
	benches := map[string]*Bench{}
	layouts := map[*Bench]string{}

	rows, err := s.db.Query(`SELECT id, name, coalesce(root,''), coalesce(color,'') FROM lab ORDER BY ord`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		l := &Lab{}
		if err := rows.Scan(&l.ID, &l.Name, &l.Root, &l.Color); err != nil {
			rows.Close()
			return nil, err
		}
		labs[l.ID] = l
		st.Labs = append(st.Labs, l)
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id, lab, type, name, coalesce(root,''), coalesce(icon,''), layout, settings FROM bench ORDER BY ord`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		b := &Bench{}
		var lab, layout string
		if err := rows.Scan(&b.ID, &lab, &b.Type, &b.Name, &b.Root, &b.Icon, &layout, &b.Settings); err != nil {
			rows.Close()
			return nil, err
		}
		if l := labs[lab]; l != nil && b.Type.Valid() {
			b.Lab = l
			l.Benches = append(l.Benches, b)
			benches[b.ID] = b
			layouts[b] = layout
		}
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT id, bench, coalesce(title,''), coalesce(custom_title,''), pinned, unread, data FROM tab ORDER BY ord`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		t := &Tab{}
		var bench string
		if err := rows.Scan(&t.ID, &bench, &t.Title, &t.CustomTitle, &t.Pinned, &t.Unread, &t.Data); err != nil {
			rows.Close()
			return nil, err
		}
		if b := benches[bench]; b != nil {
			t.Bench = b
			b.Tabs = append(b.Tabs, t)
		}
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT bench, tab FROM closed_tab ORDER BY closed_at`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var bench, data string
		if err := rows.Scan(&bench, &data); err != nil {
			rows.Close()
			return nil, err
		}
		var t Tab
		if b := benches[bench]; b != nil && json.Unmarshal([]byte(data), &t) == nil {
			b.closed = append(b.closed, &t)
		}
	}
	rows.Close()

	for b, layout := range layouts {
		b.Tabs = b.Ordered()
		if err := b.UnmarshalLayout(layout); err != nil {
			b.Layout = &Node{}
			b.Focus = b.Layout
		}
	}

	rows, err = s.db.Query(`SELECT key, coalesce(lab,''), coalesce(bench,''), coalesce(sidebar,1) FROM window`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key, lab, bench string
		var sidebar bool
		if err := rows.Scan(&key, &lab, &bench, &sidebar); err != nil {
			rows.Close()
			return nil, err
		}
		w := &Window{Key: key, Sidebar: sidebar}
		if l := labs[lab]; l != nil {
			w.SelectLab(l)
			if b := benches[bench]; b != nil && b.Lab == l {
				w.SelectBench(b)
			}
		}
		st.Windows[key] = w
	}
	rows.Close()

	rows, err = s.db.Query(`SELECT key, value FROM kv`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return nil, err
		}
		if k == "backup.day" {
			s.backupDay = v
		} else {
			st.KV[k] = v
		}
	}
	rows.Close()
	return st, nil
}

// snapshot is the State as rows, taken on the main goroutine.
type snapshot struct {
	labs    [][]any
	benches [][]any
	tabs    [][]any
	closed  [][]any
	windows [][]any
	kv      [][]any
}

func (s *Store) snapshot(st *State) snapshot {
	var sn snapshot
	now := time.Now().UnixNano()
	for i, l := range st.Labs {
		sn.labs = append(sn.labs, []any{l.ID, l.Name, l.Root, l.Color, i})
		for j, b := range l.Benches {
			sn.benches = append(sn.benches, []any{b.ID, l.ID, string(b.Type), b.Name, b.Root, b.Icon, j, b.MarshalLayout(), b.Settings})
			for k, t := range b.Tabs {
				sn.tabs = append(sn.tabs, []any{t.ID, b.ID, t.Title, t.CustomTitle, k, t.Pinned, t.Unread, t.Data})
			}
			for k, t := range b.closed {
				data, _ := json.Marshal(Tab{ID: t.ID, Title: t.Title, CustomTitle: t.CustomTitle, Pinned: t.Pinned, Data: t.Data})
				sn.closed = append(sn.closed, []any{b.ID, now + int64(k), string(data)})
			}
		}
	}
	for _, w := range st.Windows {
		var lab, bench, focused any
		if w.Lab != nil {
			lab = w.Lab.ID
		}
		if w.Bench != nil {
			bench = w.Bench.ID
			if t := w.Bench.FocusedTab(); t != nil {
				focused = t.ID
			}
		}
		sn.windows = append(sn.windows, []any{w.Key, lab, bench, focused, w.Sidebar})
	}
	for k, v := range st.KV {
		sn.kv = append(sn.kv, []any{k, v})
	}
	return sn
}

// write replaces every row in one transaction.
// ponytail: full rewrite per save; per-row upserts when State reaches thousands of Tabs.
func (s *Store) write(sn snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM closed_tab`, `DELETE FROM tab`, `DELETE FROM bench`, `DELETE FROM lab`, `DELETE FROM window`, `DELETE FROM kv`} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	inserts := []struct {
		q    string
		rows [][]any
	}{
		{`INSERT INTO lab VALUES (?,?,?,?,?)`, sn.labs},
		{`INSERT INTO bench (id, lab, type, name, root, icon, ord, layout, settings) VALUES (?,?,?,?,?,?,?,?,?)`, sn.benches},
		{`INSERT INTO tab VALUES (?,?,?,?,?,?,?,?)`, sn.tabs},
		{`INSERT INTO closed_tab VALUES (?,?,?)`, sn.closed},
		{`INSERT INTO window (key, lab, bench, focused_tab, sidebar) VALUES (?,?,?,?,?)`, sn.windows},
		{`INSERT INTO kv VALUES (?,?)`, sn.kv},
	}
	for _, in := range inserts {
		for _, r := range in.rows {
			if _, err := tx.Exec(in.q, r...); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.backup()
}

// backup copies the database to state.db.bak once a day (R-MODEL-056).
func (s *Store) backup() error {
	day := time.Now().Format("2006-01-02")
	if s.backupDay == day {
		_, err := s.db.Exec(`INSERT OR REPLACE INTO kv VALUES ('backup.day', ?)`, day)
		return err
	}
	bak := s.path + ".bak"
	_ = os.Remove(bak)
	if _, err := s.db.Exec(`VACUUM INTO ?`, bak); err != nil {
		return err
	}
	s.backupDay = day
	_, err := s.db.Exec(`INSERT OR REPLACE INTO kv VALUES ('backup.day', ?)`, day)
	return err
}

// Autosave writes the State at most once per 100 ms after a change
// (R-MODEL-052). onMain runs a function on the main goroutine; onErr hears
// a failed write (R-MODEL-053).
func (s *Store) Autosave(st *State, onMain func(func()), onErr func(error)) {
	st.OnChange = func() {
		if s.pending {
			return
		}
		s.pending = true
		time.AfterFunc(100*time.Millisecond, func() {
			onMain(func() {
				s.pending = false
				sn := s.snapshot(st)
				go func() {
					if err := s.write(sn); err != nil {
						onErr(err)
					}
				}()
			})
		})
	}
}

// Flush writes the State now, on the calling goroutine (main). Use it at quit.
func (s *Store) Flush(st *State) error {
	return s.write(s.snapshot(st))
}
