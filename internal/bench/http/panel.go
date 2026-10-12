package http

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/httpclient"
	"github.com/sviniabanditka/notch-lab/internal/httpfile"
	"github.com/sviniabanditka/notch-lab/internal/keychain"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// Panel is the Environment select, the search and the tree of Collections
// and Requests (R-HTTP-031, R-HTTP-040 … R-HTTP-045).
func (b *Bench) Panel(c *ui.Context, bench *model.Bench) {
	th := c.Theme()
	s := b.store(bench)
	set := readSettings(bench)
	ui.Column(c.Key("hpanel")).Grow(1).Basis(0).MinHeight(0).Padding(4, 6).Gap(4).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c.Key("head")).Gap(4).AlignItems(ui.Center).Children(func() {
			env := set.Environment
			if env == "" {
				env = "No environment"
			}
			if ui.Select(c.Key("env"), &env, append([]string{"No environment"}, s.env.Names()...)).Grow(1).Changed() {
				if env == "No environment" {
					env = ""
				}
				set.Environment = env
				b.writeSettings(bench, set)
			}
			if kit.IconButton(c, "gear", "Environments", kit.Gear, false).Clicked() {
				s.envEdit = !s.envEdit
			}
			kit.IconButton(c, "more", "New", kit.Plus, false).Menu(func(m *ui.Menu) {
				if m.Item("New Collection…").Chosen() {
					b.AskText("New Collection", "", func(name string) string {
						if _, err := b.NewCollection(bench, name); err != nil {
							return err.Error()
						}
						return ""
					})
				}
				if m.Item("Import curl…").Chosen() {
					b.importCurl(bench, s)
				}
				m.Separator()
				if m.Item("Clear cookies").Chosen() { // R-HTTP-084
					b.confirm("Clear the cookies of this Bench?", "Clear", func() {
						s.jar.Clear()
					})
				}
				if m.Item("Clear history").Chosen() { // R-HTTP-103
					b.confirm("Clear the history of this Bench?", "Clear", func() {
						for _, f := range s.files {
							for _, bl := range f.Blocks {
								os.Remove(historyFile(b, f.Path, bl.Req.Title()))
							}
						}
					})
				}
				if m.Item("Open the Bench folder").Chosen() {
					_ = os.MkdirAll(s.folder, 0o755)
					_ = mygo.Shell.OpenPath(s.folder)
				}
			})
		})
		if s.envErr != "" {
			ui.Text(c, s.envErr).FontSize(11).TextColor(th.Danger)
		}
		if s.envEdit {
			b.envEditor(c, bench, s, set.Environment)
		}
		ui.TextInput(c.Key("q"), &s.query).Placeholder("Search requests")
		ui.Scroll(c.Key("tree")).Grow(1).Basis(0).MinHeight(0).Children(func() {
			if len(s.files) == 0 {
				ui.Text(c, "No Collections. + makes one; ⌘T makes a Request.").FontSize(12).TextColor(th.TextMuted)
			}
			q := strings.ToLower(s.query)
			for _, f := range s.files {
				f := f
				var hits []int
				for i, bl := range f.Blocks {
					r := bl.Req
					if r.Method == "" {
						continue
					}
					if q == "" || strings.Contains(strings.ToLower(r.Title()+" "+r.Method+" "+r.URL), q) {
						hits = append(hits, i)
					}
				}
				if q != "" && len(hits) == 0 {
					continue // R-HTTP-042
				}
				open := !s.open[f.Path+"/closed"] || q != ""
				row := ui.Row(c.Key(f.Path)).Height(24).Padding(0, 6).Gap(6).Radius(5).AlignItems(ui.Center)
				if s.sel == f.Path {
					row.Background(th.Accent.Alpha(0.15))
				} else if row.Hovered() {
					row.Background(th.SurfaceHover)
				}
				row.Children(func() {
					arrow := "▸"
					if open {
						arrow = "▾"
					}
					ui.Text(c, arrow).FontSize(10).TextColor(th.TextMuted)
					ui.Text(c, f.Name()).Bold().SingleLine().Grow(1)
					ui.Text(c, itoa(len(hits))).FontSize(11).TextColor(th.TextMuted)
				})
				if row.Clicked() {
					s.sel = f.Path
					s.open[f.Path+"/closed"] = open
				}
				row.ContextMenu(func(m *ui.Menu) {
					if m.Item("New Request").Chosen() {
						if t, err := b.NewRequest(bench, f.Path, nil); err == nil {
							b.Show(t)
						} else {
							b.toast(err.Error())
						}
					}
					if m.Item("Rename…").Chosen() {
						b.AskText("Rename Collection", f.Name(), func(name string) string {
							name = strings.TrimSuffix(strings.TrimSpace(name), ".http")
							if name == "" || strings.ContainsAny(name, `/\:`) {
								return "Not a file name"
							}
							to := filepath.Join(filepath.Dir(f.Path), name+".http")
							if err := os.Rename(f.Path, to); err != nil {
								return err.Error()
							}
							b.moved(bench, f.Path, to)
							return ""
						})
					}
					if m.Item("Reveal in Finder").Chosen() {
						mygo.Shell.ShowItemInFolder(f.Path)
					}
					m.Separator()
					if m.Item("Delete Collection").Chosen() {
						b.confirm("Move “"+f.Name()+"” to the Trash?", "Move to Trash", func() {
							if err := mygo.Shell.TrashItem(f.Path); err != nil {
								b.toast(err.Error())
							}
							s.scanned = time.Time{}
						})
					}
				})
				if !open {
					continue
				}
				for _, i := range hits {
					i := i
					r := f.Blocks[i].Req
					rr := ui.Row(c.Key(f.Path+"#"+itoa(i))).Height(24).Padding(0, 6, 0, 20).Gap(6).Radius(5).AlignItems(ui.Center)
					if t := bench.FocusedTab(); t != nil && readData(t).File == f.Path && readData(t).Index == i {
						rr.Background(th.Accent.Alpha(0.15))
					} else if rr.Hovered() {
						rr.Background(th.SurfaceHover)
					}
					rr.Children(func() {
						ui.Text(c, r.Method).FontSize(10).Bold().TextColor(methodColor(th, r.Method)).Width(46)
						ui.Text(c, r.Title()).SingleLine().Grow(1).Shrink(1).MinWidth(0)
					})
					if rr.Clicked() {
						s.sel = f.Path
						if t := b.OpenRequest(bench, f.Path, i); t != nil {
							b.Show(t)
						}
					}
					rr.ContextMenu(func(m *ui.Menu) { b.requestMenu(m, bench, s, f, i) })
				}
			}
		})
	})
}

func historyFile(b *Bench, file, name string) string {
	return httpclient.HistoryFile(filepath.Join(b.Dir, "http", "history"), file, name)
}

// requestMenu is a Request row's menu (R-HTTP-043).
func (b *Bench) requestMenu(m *ui.Menu, bench *model.Bench, s *store, f *httpfile.File, i int) {
	r := f.Blocks[i].Req
	if m.Item("Rename…").Chosen() {
		b.AskText("Rename Request", r.Title(), func(name string) string {
			name = strings.TrimSpace(name)
			if name == "" {
				return "A name is needed"
			}
			old := r.Title()
			r.Name = name
			f.Blocks[i].Dirty = true
			if err := f.Save(); err != nil {
				return err.Error()
			}
			b.renamed(bench, f.Path, i, old, name)
			s.scanned = time.Time{}
			return ""
		})
	}
	if m.Item("Duplicate").Chosen() {
		cp := r.Clone()
		cp.Name = r.Title() + " copy"
		f.Add(cp)
		if err := f.Save(); err != nil {
			b.toast(err.Error())
		}
		s.scanned = time.Time{}
	}
	if m.Item("Copy as curl").Chosen() {
		if t := b.OpenRequest(bench, f.Path, i); t != nil {
			if out, err := b.Curl(t); err == nil {
				mygo.Clipboard.WriteText(out)
			}
		}
	}
	if len(s.files) > 1 {
		m.Submenu("Move to", func(sm *ui.Menu) { // R-HTTP-045 (a menu, not a drag)
			for _, other := range s.files {
				other := other
				if other == f {
					continue
				}
				if sm.Item(other.Name()).Chosen() {
					other.Add(r.Clone())
					f.Remove(i)
					if err := other.Save(); err == nil {
						_ = f.Save()
					}
					s.scanned = time.Time{}
				}
			}
		})
	}
	m.Separator()
	if m.Item("Delete Request").Chosen() {
		b.confirm("Delete “"+r.Title()+"”?", "Delete", func() {
			f.Remove(i)
			if err := f.Save(); err != nil {
				b.toast(err.Error())
			}
			s.scanned = time.Time{}
		})
	}
}

// renamed keeps the open Tab of a renamed Request on it.
func (b *Bench) renamed(bench *model.Bench, path string, i int, old, name string) {
	for _, t := range bench.Tabs {
		if d := readData(t); d.File == path && d.Name == old {
			d.Name = name
			writeData(t, d)
			t.Title = name
			if x := b.tabs[t]; x != nil && x.req != nil {
				x.req.Name = name
				x.base = x.req.Text()
			}
		}
	}
	b.St.Changed()
}

// moved points Tabs at a renamed Collection.
func (b *Bench) moved(bench *model.Bench, from, to string) {
	for _, t := range bench.Tabs {
		if d := readData(t); d.File == from {
			d.File = to
			writeData(t, d)
		}
	}
	b.store(bench).scanned = time.Time{}
	b.St.Changed()
}

// importCurl asks for a curl command and makes a Request (R-HTTP-050).
func (b *Bench) importCurl(bench *model.Bench, s *store) {
	b.AskText("Paste a curl command", "", func(cmd string) string {
		r, ignored, err := httpfile.FromCurl(cmd)
		if err != nil {
			return err.Error()
		}
		t, err := b.NewRequest(bench, s.sel, r)
		if err != nil {
			return err.Error()
		}
		if len(ignored) > 0 {
			b.toast("Ignored: " + strings.Join(ignored, ", ")) // R-HTTP-052
		}
		b.Show(t)
		return ""
	})
}

// envEditor edits the variables of the selected Environment and $shared;
// a secret goes to the Keychain (R-HTTP-033).
func (b *Bench) envEditor(c *ui.Context, bench *model.Bench, s *store, active string) {
	th := c.Theme()
	ui.Column(c.Key("envs")).Gap(4).Padding(6).Radius(6).Background(th.Surface).Children(func() {
		for _, name := range []string{active, "$shared"} {
			if name == "" {
				continue
			}
			name := name
			vars := s.env[name]
			ui.Text(c, name).Bold()
			keys := sortedKeys(vars)
			for _, k := range keys {
				k := k
				ui.Row(c.Key(name + k)).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Text(c, k).Width(110).SingleLine().FontSize(12)
					if httpfile.IsSecret(k) {
						ui.Text(c, httpfile.Mask).Grow(1).TextColor(th.TextMuted)
						if ui.Button(c.Key("set"), "Set").Padding(0, 6).Clicked() {
							b.AskText("Value of "+k, "", func(v string) string {
								if err := keychain.Set(secretAccount(name, k), v); err != nil {
									return err.Error()
								}
								return ""
							})
						}
						return
					}
					v := vars[k]
					if ui.TextInput(c.Key("v"), &v).Grow(1).FontSize(12).Changed() {
						vars[k] = v
						_ = s.env.Save(s.folder)
					}
				})
			}
			if ui.Button(c.Key("add"+name), "+ Variable").Background(ui.Transparent).Clicked() {
				b.AskText("New variable in "+name+" (secret.* goes to the Keychain)", "", func(k string) string {
					k = strings.TrimSpace(k)
					if k == "" || strings.ContainsAny(k, " {}") {
						return "Not a variable name"
					}
					if s.env[name] == nil {
						s.env[name] = map[string]string{}
					}
					s.env[name][k] = ""
					if err := s.env.Save(s.folder); err != nil {
						return err.Error()
					}
					return ""
				})
			}
		}
		if ui.Button(c.Key("newenv"), "+ Environment").Background(ui.Transparent).Clicked() {
			b.AskText("New Environment", "", func(n string) string {
				n = strings.TrimSpace(n)
				if n == "" || n == "$shared" {
					return "Not an Environment name"
				}
				if s.env[n] == nil {
					s.env[n] = map[string]string{}
				}
				if err := s.env.Save(s.folder); err != nil {
					return err.Error()
				}
				set := readSettings(bench)
				set.Environment = n
				b.writeSettings(bench, set)
				return ""
			})
		}
	})
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// confirm runs then when the user answers yes; at once without Confirm.
func (b *Bench) confirm(q, yes string, then func()) {
	if b.Confirm == nil {
		then()
		return
	}
	b.Confirm(q, yes, func(ok bool) {
		if ok {
			then()
		}
	})
}
