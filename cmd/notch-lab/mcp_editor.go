package main

import (
	"path/filepath"

	editorbench "github.com/sviniabanditka/notch-lab/internal/bench/editor"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// The tools of the domain `editor` (05-editor.md, "MCP tools"). No tool
// writes a Buffer (R-EDIT-180).

func (a *app) edTab(c *mcp.Call) (*model.Tab, error) {
	if id := c.Str("tab"); id != "" {
		t, err := a.tab(id)
		if err != nil {
			return nil, err
		}
		if t.Bench.Type != model.Editor {
			return nil, mcp.Errf(mcp.CodeNotFound, "Tab %s is no editor Tab", id)
		}
		return t, nil
	}
	if p := c.Str("path"); p != "" {
		for _, l := range a.st.Labs {
			for _, b := range l.Benches {
				if b.Type != model.Editor {
					continue
				}
				for _, t := range b.Tabs {
					if a.ed.Describe(t).Path == filepath.Clean(p) {
						return t, nil
					}
				}
			}
		}
		return nil, mcp.Errf(mcp.CodeNotFound, "no editor Tab has %s", p)
	}
	if w := a.st.Window("main"); w.Bench != nil && w.Bench.Type == model.Editor && w.Bench.FocusedTab() != nil {
		return w.Bench.FocusedTab(), nil
	}
	return nil, mcp.Errf(mcp.CodeNotFound, "no editor Tab is focused; give tab")
}

func abs(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", mcp.Errf(mcp.CodeInvalid, "path must be absolute")
	}
	return filepath.Clean(p), nil
}

func (a *app) editorTools() []*mcp.Tool {
	return []*mcp.Tool{
		{Name: "editor.open", Domain: "editor", Level: mcp.Full, Desc: "Open a file in an editor Tab at a line; returns the Tab id.",
			Params: map[string]any{"path": str("absolute path"), "line": num("1-based line"), "col": num("1-based column"), "bench": str("editor Bench id")}, Required: []string{"path"},
			Run: func(c *mcp.Call) (any, error) {
				p, err := abs(c.Str("path"))
				if err != nil {
					return nil, err
				}
				return on(c, func() (any, error) {
					var b *model.Bench
					if c.Str("bench") != "" {
						if b, err = a.bench(c, model.Editor); err != nil {
							return nil, err
						}
					} else {
						l, err := a.labOf("")
						if err != nil {
							return nil, err
						}
						b = a.ed.BenchFor(l, p)
					}
					t := a.ed.Open(b, p, c.Int("line", 0), c.Int("col", 0), false)
					if t.Owner == "" {
						t.Owner = c.Client.ID
					}
					return map[string]string{"tab": t.ID}, nil
				})
			}},
		{Name: "editor.reveal", Domain: "editor", Level: mcp.Full, Desc: "Select a file in the Explorer and open its folders.", Params: map[string]any{"path": str("absolute path")}, Required: []string{"path"},
			Run: func(c *mcp.Call) (any, error) {
				p, err := abs(c.Str("path"))
				if err != nil {
					return nil, err
				}
				return on(c, func() (any, error) {
					l, err := a.labOf("")
					if err != nil {
						return nil, err
					}
					a.ed.Reveal(a.ed.BenchFor(l, p), p)
					return ok(), nil
				})
			}},
		{Name: "editor.get_open_files", Domain: "editor", Level: mcp.Read, Desc: "The files open in editor Tabs.", Params: map[string]any{"bench": str("editor Bench id")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					out := []editorbench.FileInfo{}
					for _, l := range a.st.Labs {
						for _, b := range l.Benches {
							if b.Type != model.Editor || c.Str("bench") != "" && b.ID != c.Str("bench") {
								continue
							}
							for _, t := range b.Ordered() {
								if in := a.ed.Describe(t); in.Path != "" {
									out = append(out, in)
								}
							}
						}
					}
					return out, nil
				})
			}},
		{Name: "editor.get_selection", Domain: "editor", Level: mcp.Read, Desc: "The selection of the focused editor Tab, or of `tab`; text cut at 64 KB.", Params: map[string]any{"tab": str("Tab id")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.edTab(c)
					if err != nil {
						return nil, err
					}
					path, sl, sc, el, ec, text, found := a.ed.Selection(t)
					if !found {
						return nil, mcp.Errf(mcp.CodeNotFound, "the Tab's file is not loaded; focus it first")
					}
					if len(text) > 64<<10 {
						text = text[:64<<10]
					}
					return map[string]any{"path": path, "start": map[string]int{"line": sl, "col": sc}, "end": map[string]int{"line": el, "col": ec}, "text": text}, nil
				})
			}},
		{Name: "editor.goto", Domain: "editor", Level: mcp.Full, Desc: "Move the cursor of an editor Tab (by tab or path) to a line and centre it.",
			Params: map[string]any{"tab": str("Tab id"), "path": str("absolute path"), "line": num("1-based line"), "col": num("1-based column")}, Required: []string{"line"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.edTab(c)
					if err != nil {
						return nil, err
					}
					a.sh.Show(t)
					a.ed.Goto(t, max(1, c.Int("line", 1)), max(1, c.Int("col", 1)))
					return ok(), nil
				})
			}},
	}
}
