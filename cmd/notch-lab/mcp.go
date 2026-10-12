package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/bench/browser"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// The MCP server of the app (docs/spec/13-mcp.md): the socket, the Access
// levels from Settings, Toasts, own Tabs, and the tools of each domain.

// startMCP listens on $NOTCH_LAB_HOME/mcp.sock (R-MCP-001).
func (a *app) startMCP() {
	var tools []*mcp.Tool
	tools = append(tools, a.labTools()...)
	tools = append(tools, a.terminalTools()...)
	tools = append(tools, a.browserTools()...)
	tools = append(tools, a.editorTools()...)
	tools = append(tools, a.httpTools()...)
	tools = append(tools, a.dbTools()...)
	tools = append(tools, a.sshTools()...)
	tools = append(tools, a.notesTools()...)
	tools = append(tools, a.prTools()...)
	tools = append(tools, a.agentTools()...)
	a.mcp = &mcp.Server{
		Path:     mcp.SocketPath(a.dir),
		Tools:    tools,
		Main:     func(fn func()) { a.main.Update(fn) },
		Access:   a.access,
		Resource: a.resource,
		DB:       a.store.DB(),
		Changed:  a.agentToast,
		Ended:    a.clientEnded,
		ClientsChanged: func() {
			a.main.Invalidate()
			if a.settings != nil && !a.settings.IsDestroyed() {
				a.settings.Invalidate()
			}
		},
	}
	if err := a.mcp.Listen(); err != nil {
		logf("error", "mcp: %v", err)
		a.mcp = nil
	}
}

// domainDefaults are the levels a domain has when Settings name none and
// the global level is wider (R-MCP-041).
var domainDefaults = map[string]string{"ssh": "off"}

// access resolves a domain's Level: its override, else the global level
// (R-MCP-040).
func (a *app) access(domain string) mcp.Level {
	m := a.set.S.MCP
	if o := m.Domains[domain]; o != "" && o != "inherit" {
		return mcp.ParseLevel(o)
	}
	if d, ok := domainDefaults[domain]; ok {
		return min(mcp.ParseLevel(d), mcp.ParseLevel(m.Access))
	}
	return mcp.ParseLevel(m.Access)
}

// agentToast shows "<client>: <tool>" for 2 s, counting repeats (R-MCP-044).
func (a *app) agentToast(c *mcp.Client, tool string) {
	if !a.set.S.MCP.ToastOnWrite {
		return
	}
	key := c.ID + tool
	if a.lastToast.key == key && time.Since(a.lastToast.at) < 2*time.Second {
		a.lastToast.n++
	} else {
		a.lastToast.key, a.lastToast.n = key, 1
	}
	a.lastToast.at = time.Now()
	msg := c.Name + ": " + tool
	if a.lastToast.n > 1 {
		msg += fmt.Sprintf(" ×%d", a.lastToast.n)
	}
	a.sh.Toast(msg)
}

// clientEnded closes the Client's own Tabs when Settings say so, and
// clears its mark from the rest (R-MCP-010, R-MCP-032, R-WEB-213).
func (a *app) clientEnded(c *mcp.Client) {
	for _, l := range a.st.Labs {
		for _, b := range l.Benches {
			for _, t := range append([]*model.Tab(nil), b.Tabs...) {
				if t.Owner != c.ID {
					continue
				}
				t.Owner = ""
				if a.set.S.MCP.CloseOwnTabs && !(b.Type == model.Editor && a.ed.Dirty(t)) {
					a.closeQuietly(t)
				}
			}
		}
	}
	a.st.Changed()
}

// closeQuietly closes a Tab with no question: the caller checked.
func (a *app) closeQuietly(t *model.Tab) {
	tt := a.sh.Types[t.Bench.Type]
	a.st.CloseTab(t)
	if tt != nil {
		tt.Closed(t)
	}
}

// on runs fn on the main goroutine for a tool and returns its outcome.
func on(c *mcp.Call, fn func() (any, error)) (v any, err error) {
	c.Main(func() { v, err = fn() })
	return
}

// seen reports whether agents may see a Tab (R-MCP-033).
func seen(t *model.Tab) bool {
	return t.Bench != nil && (t.Bench.Type != model.Browser || browser.AgentsMayUse(t.Bench))
}

// tab finds a Tab agents may see; main goroutine.
func (a *app) tab(id string) (*model.Tab, error) {
	for _, l := range a.st.Labs {
		for _, b := range l.Benches {
			if t := b.TabByID(id); t != nil && seen(t) {
				return t, nil
			}
		}
	}
	return nil, mcp.Errf(mcp.CodeNotFound, "no Tab %s", id)
}

func (a *app) labOf(id string) (*model.Lab, error) {
	if id == "" {
		if l := a.st.Window("main").Lab; l != nil {
			return l, nil
		}
		return a.st.Labs[0], nil
	}
	if l := a.st.LabByID(id); l != nil {
		return l, nil
	}
	return nil, mcp.Errf(mcp.CodeNotFound, "no Lab %s", id)
}

// bench is the Bench a call names, or the Lab's first of a type, made at
// Full level when missing (R-MCP-072). Main goroutine.
func (a *app) bench(c *mcp.Call, typ model.BenchType) (*model.Bench, error) {
	if id := c.Str("bench"); id != "" {
		b := a.st.BenchByID(id)
		if b == nil || b.Type != typ || typ == model.Browser && !browser.AgentsMayUse(b) {
			return nil, mcp.Errf(mcp.CodeNotFound, "no %s Bench %s", typ, id)
		}
		return b, nil
	}
	l, err := a.labOf(c.Str("lab"))
	if err != nil {
		return nil, err
	}
	if w := a.st.Window("main"); w.Lab == l && w.Bench != nil && w.Bench.Type == typ && (typ != model.Browser || browser.AgentsMayUse(w.Bench)) {
		return w.Bench, nil
	}
	for _, b := range l.Benches {
		if b.Type == typ && (typ != model.Browser || browser.AgentsMayUse(b)) {
			return b, nil
		}
	}
	if a.access(domainOf(typ)) < mcp.Full {
		return nil, mcp.Errf(mcp.CodeNoBench, "no %s Bench", typ)
	}
	return a.st.NewBench(l, typ, typ.Title(), ""), nil
}

func domainOf(t model.BenchType) string {
	if t == model.Editor || t == model.Files {
		return "editor"
	}
	if t == model.SSH {
		return "ssh"
	}
	if t == model.PullRequests {
		return "pull-requests"
	}
	return string(t)
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func (a *app) labTools() []*mcp.Tool {
	tabsOf := func(b *model.Bench) []map[string]any {
		var out []map[string]any
		for _, t := range b.Ordered() {
			if !seen(t) {
				continue
			}
			m := map[string]any{"id": t.ID, "title": t.DisplayTitle(), "type": string(b.Type), "pinned": t.Pinned, "unread": t.Unread,
				"in_layout": b.Layout.LeafOf(t) != nil, "focused": b.FocusedTab() == t}
			if t.Owner != "" {
				m["owner"] = t.Owner
			}
			out = append(out, m)
		}
		return out
	}
	benchInfo := func(b *model.Bench) map[string]any {
		return map[string]any{"id": b.ID, "type": string(b.Type), "name": b.Name, "root": b.EffectiveRoot(a.sh.Home), "tabs": len(b.Tabs),
			"selected": a.st.Window("main").Bench == b}
	}
	return []*mcp.Tool{
		{Name: "lab.list", Domain: "lab", Level: mcp.Read, Desc: "List the Labs.", Run: func(c *mcp.Call) (any, error) {
			return on(c, func() (any, error) {
				var out []map[string]any
				for _, l := range a.st.Labs {
					unread := 0
					for _, b := range l.Benches {
						for _, t := range b.Tabs {
							if t.Unread {
								unread++
							}
						}
					}
					out = append(out, map[string]any{"id": l.ID, "name": l.Name, "root": l.Root, "selected": a.st.Window("main").Lab == l,
						"benches": len(l.Benches), "unread": unread})
				}
				return out, nil
			})
		}},
		{Name: "lab.get", Domain: "lab", Level: mcp.Read, Desc: "A Lab with its Benches.", Params: map[string]any{"id": str("Lab id")}, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					l, err := a.labOf(c.Str("id"))
					if err != nil {
						return nil, err
					}
					var benches []map[string]any
					for _, b := range l.Benches {
						benches = append(benches, benchInfo(b))
					}
					return map[string]any{"id": l.ID, "name": l.Name, "root": l.Root, "benches": benches}, nil
				})
			}},
		{Name: "lab.create", Domain: "lab", Level: mcp.Full, Desc: "Create a Lab.", Params: map[string]any{"name": str("name"), "root": str("root folder")}, Required: []string{"name"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					return map[string]any{"id": a.st.NewLab(model.Clip(c.Str("name")), c.Str("root")).ID}, nil
				})
			}},
		{Name: "lab.select", Domain: "lab", Level: mcp.Full, Desc: "Show a Lab in the main window or the notch panel.",
			Params: map[string]any{"id": str("Lab id"), "window": map[string]any{"type": "string", "enum": []string{"main", "notch"}}}, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					l, err := a.labOf(c.Str("id"))
					if err != nil {
						return nil, err
					}
					if c.Str("window") == "notch" { // R-SHELL-011: MCP shows the panel
						a.st.Window("notch").SelectLab(l)
						a.notch.show()
					} else {
						a.sh.SelectLab(l)
					}
					a.st.Changed()
					return map[string]bool{"ok": true}, nil
				})
			}},
		{Name: "lab.benches", Domain: "lab", Level: mcp.Read, Desc: "List the Benches of a Lab (default: the selected Lab).", Params: map[string]any{"lab": str("Lab id")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					l, err := a.labOf(c.Str("lab"))
					if err != nil {
						return nil, err
					}
					var out []map[string]any
					for _, b := range l.Benches {
						out = append(out, benchInfo(b))
					}
					return out, nil
				})
			}},
		{Name: "lab.bench_create", Domain: "lab", Level: mcp.Full, Desc: "Create a Bench: type terminal, editor, files or browser.",
			Params:   map[string]any{"lab": str("Lab id"), "type": map[string]any{"type": "string", "enum": []string{"terminal", "editor", "files", "browser"}}, "name": str("name"), "root": str("root folder")},
			Required: []string{"type"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					l, err := a.labOf(c.Str("lab"))
					if err != nil {
						return nil, err
					}
					typ := model.BenchType(c.Str("type"))
					if !typ.Valid() {
						return nil, mcp.Errf(mcp.CodeInvalid, "unknown Bench type %s", typ)
					}
					name := c.Str("name")
					if name == "" {
						name = typ.Title()
					}
					return map[string]any{"id": a.st.NewBench(l, typ, model.Clip(name), c.Str("root")).ID}, nil
				})
			}},
		{Name: "lab.tabs", Domain: "lab", Level: mcp.Read, Desc: "List the Tabs of a Bench.", Params: map[string]any{"bench": str("Bench id")}, Required: []string{"bench"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b := a.st.BenchByID(c.Str("bench"))
					if b == nil {
						return nil, mcp.Errf(mcp.CodeNotFound, "no Bench %s", c.Str("bench"))
					}
					return tabsOf(b), nil
				})
			}},
		{Name: "lab.tab_focus", Domain: "lab", Level: mcp.Full, Desc: "Show a Tab in the focused Leaf of its Bench and select its Bench and Lab.", Params: map[string]any{"id": str("Tab id")}, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.tab(c.Str("id"))
					if err != nil {
						return nil, err
					}
					a.sh.Show(t)
					return map[string]bool{"ok": true}, nil
				})
			}},
		{Name: "lab.tab_close", Domain: "lab", Level: mcp.Full, Destructive: true, Desc: "Close a Tab. A Tab with unsaved changes is refused.", Params: map[string]any{"id": str("Tab id")}, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.tab(c.Str("id"))
					if err != nil {
						return nil, err
					}
					if t.Bench.Type == model.Editor && a.ed.Dirty(t) {
						return nil, mcp.Errf(mcp.CodeRefused, "unsaved changes")
					}
					a.closeQuietly(t)
					return map[string]bool{"ok": true}, nil
				})
			}},
		{Name: "lab.layout", Domain: "lab", Level: mcp.Read, Desc: "The Layout of a Bench as JSON.", Params: map[string]any{"bench": str("Bench id")}, Required: []string{"bench"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b := a.st.BenchByID(c.Str("bench"))
					if b == nil {
						return nil, mcp.Errf(mcp.CodeNotFound, "no Bench %s", c.Str("bench"))
					}
					return json.RawMessage(b.MarshalLayout()), nil
				})
			}},
		{Name: "lab.layout_split", Domain: "lab", Level: mcp.Full, Desc: "Put a Tab beside the focused Leaf, or beside the Leaf of `at`.",
			Params:   map[string]any{"bench": str("Bench id"), "tab": str("Tab id"), "direction": map[string]any{"type": "string", "enum": []string{"row", "column"}}, "at": str("Tab id whose Leaf splits")},
			Required: []string{"bench", "tab", "direction"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.tab(c.Str("tab"))
					if err != nil || t.Bench.ID != c.Str("bench") {
						return nil, mcp.Errf(mcp.CodeNotFound, "no Tab %s in Bench %s", c.Str("tab"), c.Str("bench"))
					}
					if at := c.Str("at"); at != "" {
						x, err := a.tab(at)
						if err != nil || x.Bench != t.Bench || t.Bench.Layout.LeafOf(x) == nil {
							return nil, mcp.Errf(mcp.CodeNotFound, "Tab %s is in no Leaf of the Bench", at)
						}
						a.st.FocusLeaf(t.Bench, t.Bench.Layout.LeafOf(x))
					}
					dir := model.Row
					if c.Str("direction") == "column" {
						dir = model.Column
					}
					a.st.Split(t, dir)
					return map[string]bool{"ok": true}, nil
				})
			}},
	}
}

// resource reads notch://lab/<id> and notch://tab/<id> (R-MCP-060).
func (a *app) resource(uri string) (any, error) {
	kind, id, _ := strings.Cut(strings.TrimPrefix(uri, "notch://"), "/")
	switch kind {
	case "lab":
		l, err := a.labOf(id)
		if err != nil {
			return nil, err
		}
		var benches []map[string]any
		for _, b := range l.Benches {
			benches = append(benches, map[string]any{"id": b.ID, "type": string(b.Type), "name": b.Name, "root": b.EffectiveRoot(a.sh.Home)})
		}
		return map[string]any{"id": l.ID, "name": l.Name, "root": l.Root, "benches": benches}, nil
	case "tab":
		t, err := a.tab(id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": t.ID, "bench": t.Bench.ID, "type": string(t.Bench.Type), "title": t.DisplayTitle(),
			"data": json.RawMessage(mcp.Mask(t.Data))}, nil
	}
	return nil, mcp.Errf(mcp.CodeNotFound, "no resource %s", uri)
}
