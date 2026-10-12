package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	notesbench "github.com/sviniabanditka/notch-lab/internal/bench/notes"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// The tools of the domain `notes` (07-notes.md, "MCP tools").

// notesBench is the notes Bench of the Lab a call names (or the selected
// one), made at Full when missing.
func (a *app) notesBench(c *mcp.Call) (*model.Bench, error) {
	b, err := a.bench(c, model.Notes)
	if err == nil {
		return b, nil
	}
	// No notes Bench at Read: the Lab's default folder still holds Notes.
	l, lerr := a.labOf(c.Str("lab"))
	if lerr != nil {
		return nil, lerr
	}
	return &model.Bench{ID: "notes-" + l.ID, Lab: l, Type: model.Notes, Settings: "{}"}, nil
}

// scoped lists the Notes of a scope: lab, global or all.
func (a *app) scoped(b *model.Bench, scope string) []*notesbench.Note {
	lab, global := a.notes.Indexes(b)
	var out []*notesbench.Note
	if scope != "global" {
		out = append(out, lab.Sorted()...)
	}
	if scope != "lab" {
		out = append(out, global.Sorted()...)
	}
	return out
}

// notePath checks a tool's path (R-NOTES-040).
func (a *app) notePath(b *model.Bench, p string) (string, error) {
	if p == "" || !filepath.IsAbs(p) || !strings.HasSuffix(p, ".md") || !a.notes.Inside(b, p) {
		return "", mcp.Errf(mcp.CodeRefused, "path outside notes")
	}
	return filepath.Clean(p), nil
}

func (a *app) notesTools() []*mcp.Tool {
	scope := map[string]any{"type": "string", "enum": []string{"lab", "global", "all"}}
	return []*mcp.Tool{
		{Name: "notes.list", Domain: "notes", Level: mcp.Read, Desc: "The Notes of the Lab and the Global folder, newest first.",
			Params: map[string]any{"lab": str("Lab id"), "scope": scope},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := a.notesBench(c)
					if err != nil {
						return nil, err
					}
					out := []map[string]any{}
					for _, n := range a.scoped(b, c.Str("scope")) {
						out = append(out, map[string]any{"path": n.Path, "title": n.Title, "modified": n.Mod.Format(time.RFC3339), "size": n.Size})
					}
					return out, nil
				})
			}},
		{Name: "notes.read", Domain: "notes", Level: mcp.Read, Desc: "A Note's text, cut at 1 MB.", Params: map[string]any{"path": str("absolute path")}, Required: []string{"path"},
			Run: func(c *mcp.Call) (any, error) {
				var p string
				_, err := on(c, func() (any, error) {
					b, err := a.notesBench(c)
					if err != nil {
						return nil, err
					}
					p, err = a.notePath(b, c.Str("path"))
					return nil, err
				})
				if err != nil {
					return nil, err
				}
				data, err := os.ReadFile(p)
				if err != nil {
					return nil, mcp.Errf(mcp.CodeNotFound, "no Note %s", p)
				}
				if len(data) > 1<<20 {
					data = data[:1<<20]
				}
				title, _ := notesbench.TitleOf(string(data), p)
				return map[string]any{"path": p, "title": title, "text": string(data)}, nil
			}},
		{Name: "notes.search", Domain: "notes", Level: mcp.Read, Desc: "Notes whose title or text holds every word of the query, with the first matching line.",
			Params: map[string]any{"query": str("words"), "lab": str("Lab id"), "scope": scope, "limit": num("default 20")}, Required: []string{"query"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := a.notesBench(c)
					if err != nil {
						return nil, err
					}
					out := []map[string]any{}
					for _, h := range notesbench.Search(a.scoped(b, c.Str("scope")), c.Str("query"), c.Int("limit", 20)) {
						out = append(out, map[string]any{"path": h.Path, "title": h.Title, "line": h.Line, "text": h.Text})
					}
					return out, nil
				})
			}},
		{Name: "notes.write", Domain: "notes", Level: mcp.Full, Desc: "Write a whole Note: by path, or by title (a new file in the Lab's notes folder). Refused while the Note has unsaved edits in a Tab.",
			Params: map[string]any{"path": str("absolute path"), "title": str("title for a new Note"), "text": str("markdown")}, Required: []string{"text"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := a.notesBench(c)
					if err != nil {
						return nil, err
					}
					p := c.Str("path")
					if p == "" {
						if c.Str("title") == "" {
							return nil, mcp.Errf(mcp.CodeInvalid, "give path or title")
						}
						dir := a.notes.Folder(b)
						_ = os.MkdirAll(dir, 0o755)
						p = notesbench.FileName(dir, c.Str("title"))
					}
					if p, err = a.notePath(b, p); err != nil {
						return nil, err
					}
					if a.dirtyNote(p) { // R-NOTES-041
						return nil, mcp.Errf(mcp.CodeRefused, "note has unsaved changes")
					}
					if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
						return nil, err
					}
					if err := os.WriteFile(p, []byte(c.Str("text")), 0o644); err != nil {
						return nil, err
					}
					return map[string]string{"path": p}, nil
				})
			}},
		{Name: "notes.append", Domain: "notes", Level: mcp.Full, Desc: "Add text at the end of a Note, after a line break.",
			Params: map[string]any{"path": str("absolute path"), "text": str("markdown")}, Required: []string{"path", "text"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := a.notesBench(c)
					if err != nil {
						return nil, err
					}
					p, err := a.notePath(b, c.Str("path"))
					if err != nil {
						return nil, err
					}
					if a.dirtyNote(p) {
						return nil, mcp.Errf(mcp.CodeRefused, "note has unsaved changes")
					}
					old, err := os.ReadFile(p)
					if err != nil {
						return nil, mcp.Errf(mcp.CodeNotFound, "no Note %s", p)
					}
					text := string(old)
					if text != "" && !strings.HasSuffix(text, "\n") {
						text += "\n"
					}
					if err := os.WriteFile(p, []byte(text+c.Str("text")), 0o644); err != nil {
						return nil, err
					}
					return map[string]string{"path": p}, nil
				})
			}},
	}
}

// dirtyNote reports a Note open with unsaved edits.
func (a *app) dirtyNote(p string) bool {
	for _, l := range a.st.Labs {
		for _, b := range l.Benches {
			if b.Type != model.Notes && b.Type != model.Editor {
				continue
			}
			for _, t := range b.Tabs {
				if strings.Contains(t.Data, `"path":"`+p+`"`) && a.ed.Dirty(t) {
					return true
				}
			}
		}
	}
	return false
}
