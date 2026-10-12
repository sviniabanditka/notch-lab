// Package http is the http Bench: Collections are .http files in the
// Bench folder, a Tab is one Request with its Response
// (docs/spec/08-http.md).
package http

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/sviniabanditka/notch-lab/internal/httpclient"
	"github.com/sviniabanditka/notch-lab/internal/httpfile"
	"github.com/sviniabanditka/notch-lab/internal/keychain"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// Bench draws every http Tab. It belongs to the main goroutine.
type Bench struct {
	St   *model.State
	Home string
	Dir  string // NOTCH_LAB_HOME
	Post func(func())
	Show func(t *model.Tab)
	// Toast, Confirm and AskText are the Shell's.
	Toast   func(string)
	Confirm func(question, yes string, then func(ok bool))
	AskText func(title, current string, check func(string) string)
	// Visible reports whether a Tab shows in a Leaf now (R-HTTP-099).
	Visible func(t *model.Tab) bool
	Win     *mygo.Window

	benches map[*model.Bench]*store
	tabs    map[*model.Tab]*tab
}

// store is one Bench's files, Environments and cookie jar.
type store struct {
	folder  string
	files   []*httpfile.File
	env     httpfile.Environments
	envErr  string
	jar     *httpclient.Jar
	scanned time.Time
	mods    map[string]time.Time
	query   string
	sel     string // the selected Collection's path
	open    map[string]bool
	envEdit bool
}

// benchSettings are an http Bench's own (R-HTTP-001, R-HTTP-031).
type benchSettings struct {
	Folder      string `json:"folder,omitempty"`
	Environment string `json:"environment,omitempty"`
}

func readSettings(b *model.Bench) benchSettings {
	var s benchSettings
	_ = json.Unmarshal([]byte(b.Settings), &s)
	return s
}

func (b *Bench) writeSettings(bench *model.Bench, s benchSettings) {
	var m map[string]any
	_ = json.Unmarshal([]byte(bench.Settings), &m)
	if m == nil {
		m = map[string]any{}
	}
	m["folder"], m["environment"] = s.Folder, s.Environment
	raw, _ := json.Marshal(m)
	bench.Settings = string(raw)
	b.St.Changed()
}

// New returns the http Bench.
func New(st *model.State, home, dir string) *Bench {
	httpclient.CleanTmp(filepath.Join(dir, "http", "tmp"))
	return &Bench{St: st, Home: home, Dir: dir, benches: map[*model.Bench]*store{}, tabs: map[*model.Tab]*tab{}}
}

// Folder is a Bench's folder (R-HTTP-002).
func (b *Bench) Folder(bench *model.Bench) string {
	if f := readSettings(bench).Folder; f != "" {
		return f
	}
	if root := bench.EffectiveRoot(b.Home); root != "" && root != b.Home {
		return filepath.Join(root, ".http")
	}
	return filepath.Join(b.Dir, "http", bench.ID)
}

// store returns a Bench's files, read again when the folder changed
// (R-HTTP-006: every 500 ms at most).
func (b *Bench) store(bench *model.Bench) *store {
	s := b.benches[bench]
	if s == nil {
		s = &store{open: map[string]bool{}, mods: map[string]time.Time{},
			jar: httpclient.LoadJar(filepath.Join(b.Dir, "http", "cookies", bench.ID+".json"))}
		b.benches[bench] = s
	}
	folder := b.Folder(bench)
	if folder != s.folder || time.Since(s.scanned) > 500*time.Millisecond {
		b.scan(s, folder)
	}
	return s
}

func (b *Bench) scan(s *store, folder string) {
	s.scanned = time.Now()
	entries, _ := os.ReadDir(folder)
	changed := folder != s.folder
	s.folder = folder
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".http") {
			continue
		}
		p := filepath.Join(folder, e.Name())
		seen[p] = true
		if info, err := e.Info(); err == nil && !info.ModTime().Equal(s.mods[p]) {
			s.mods[p] = info.ModTime()
			changed = true
		}
	}
	if len(seen) != len(s.files) {
		changed = true
	}
	envMod := time.Time{}
	if info, err := os.Stat(filepath.Join(folder, httpfile.EnvFile)); err == nil {
		envMod = info.ModTime()
	}
	if !envMod.Equal(s.mods[httpfile.EnvFile]) || s.env == nil {
		s.mods[httpfile.EnvFile] = envMod
		env, err := httpfile.LoadEnvironments(folder)
		s.env, s.envErr = env, ""
		if err != nil {
			s.env, s.envErr = httpfile.Environments{}, err.Error()
		}
	}
	if !changed {
		return
	}
	var paths []string
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	s.files = nil
	for _, p := range paths {
		if f, err := httpfile.Load(p); err == nil {
			s.files = append(s.files, f)
		}
	}
	for t, x := range b.tabs {
		if t.Bench != nil && b.benches[t.Bench] == s {
			b.diskChanged(x)
		}
	}
}

// file finds a loaded Collection by path.
func (s *store) file(path string) *httpfile.File {
	for _, f := range s.files {
		if f.Path == path {
			return f
		}
	}
	return nil
}

// secretAccount is a secret's Keychain account (R-SHELL-165, R-HTTP-032).
func secretAccount(env, name string) string { return "http/env/" + env + "/" + name }

// resolver resolves a Request's variables in its Bench's Environment.
func (b *Bench) resolver(bench *model.Bench, f *httpfile.File, r *httpfile.Request) *httpfile.Resolver {
	return b.resolverIn(bench, f, r, readSettings(bench).Environment)
}

func (b *Bench) resolverIn(bench *model.Bench, f *httpfile.File, r *httpfile.Request, envName string) *httpfile.Resolver {
	s := b.store(bench)
	res := &httpfile.Resolver{Request: r, Env: s.env[envName], Shared: s.env["$shared"]}
	if f != nil {
		res.File = f.FileVars()
	}
	res.Secret = func(name string) (string, bool) {
		for _, e := range []string{envName, "$shared"} {
			if v, err := keychain.Get(secretAccount(e, name)); err == nil {
				return v, true
			}
		}
		return "", false
	}
	return res
}

// NewCollection makes `<name>.http` with no Requests (R-HTTP-043).
func (b *Bench) NewCollection(bench *model.Bench, name string) (*httpfile.File, error) {
	name = strings.TrimSpace(strings.TrimSuffix(name, ".http"))
	if name == "" || strings.ContainsAny(name, `/\:`) {
		return nil, fmt.Errorf("Not a file name")
	}
	folder := b.Folder(bench)
	p := filepath.Join(folder, name+".http")
	if _, err := os.Stat(p); err == nil {
		return nil, fmt.Errorf("A Collection with that name exists")
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		return nil, err
	}
	s := b.store(bench)
	s.scanned = time.Time{}
	s.sel = p
	b.store(bench)
	return s.file(p), nil
}

// NewRequest appends a Request to a Collection and opens it (R-HTTP-043,
// R-HTTP-044); with no Collection it makes "Requests" first.
func (b *Bench) NewRequest(bench *model.Bench, path string, r *httpfile.Request) (*model.Tab, error) {
	s := b.store(bench)
	f := s.file(path)
	if f == nil && path != "" && filepath.Dir(path) == s.folder {
		var err error // a Collection named by the caller, as "Agent" (R-HTTP-113)
		if f, err = b.NewCollection(bench, strings.TrimSuffix(filepath.Base(path), ".http")); err != nil {
			return nil, err
		}
	}
	if f == nil && len(s.files) > 0 {
		f = s.files[0]
	}
	if f == nil {
		var err error
		if f, err = b.NewCollection(bench, "Requests"); err != nil {
			return nil, err
		}
	}
	if r == nil {
		r = &httpfile.Request{Method: "GET", URL: "https://"}
	}
	f.Add(r)
	if err := f.Save(); err != nil {
		return nil, err
	}
	s.scanned = time.Time{}
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: r.Title(), Data: "{}"}
	writeData(t, data{File: f.Path, Name: r.Title(), Index: len(f.Blocks) - 1})
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t, nil
}

// OpenRequest opens a Request in its Tab, or a new one.
func (b *Bench) OpenRequest(bench *model.Bench, path string, index int) *model.Tab {
	for _, t := range bench.Tabs {
		if d := readData(t); d.File == path && d.Index == index {
			return t
		}
	}
	f := b.store(bench).file(path)
	if f == nil || index >= len(f.Blocks) {
		return nil
	}
	r := f.Blocks[index].Req
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: r.Title(), Data: "{}"}
	writeData(t, data{File: path, Name: r.Title(), Index: index})
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t
}

// Environment is a Bench's selected Environment.
func Environment(bench *model.Bench) string { return readSettings(bench).Environment }

// Request finds a Request by its id `<file>#<index>` (R-HTTP-111).
func (b *Bench) Request(bench *model.Bench, id string) (*httpfile.File, int, *httpfile.Request) {
	i := strings.LastIndex(id, "#")
	if i < 0 {
		return nil, 0, nil
	}
	n, err := strconv.Atoi(id[i+1:])
	f := b.store(bench).file(id[:i])
	if err != nil || f == nil || n < 0 || n >= len(f.Blocks) || f.Blocks[n].Req.Method == "" {
		return nil, 0, nil
	}
	return f, n, f.Blocks[n].Req
}

// Files lists a Bench's Collections.
func (b *Bench) Files(bench *model.Bench) []*httpfile.File { return b.store(bench).files }
