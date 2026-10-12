package main

import (
	"errors"
	"strings"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/bench/terminal"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// The tools of the domain `terminal` (13-mcp.md). A terminal's id is its
// Tab's id.

func (a *app) termTab(c *mcp.Call) (*model.Tab, error) {
	t, err := a.tab(c.Str("id"))
	if err != nil {
		return nil, err
	}
	if t.Bench.Type != model.Terminal && t.Bench.Type != model.SSH { // an SSH session is a terminal Session
		return nil, mcp.Errf(mcp.CodeNotFound, "Tab %s is no terminal", t.ID)
	}
	return t, nil
}

func termErr(err error) error {
	if errors.Is(err, terminal.ErrNoDaemon) {
		return mcp.Errf(mcp.CodeNoDaemon, "the Daemon is not running")
	}
	return err
}

// screenText is the last rows lines of the text: the Screen.
func screenText(text string, rows int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if rows > 0 && len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	return strings.Join(lines, "\n")
}

func (a *app) terminalTools() []*mcp.Tool {
	idParam := map[string]any{"id": str("terminal id (its Tab id)")}
	withID := func(p map[string]any) map[string]any {
		out := map[string]any{"id": str("terminal id (its Tab id)")}
		for k, v := range p {
			out[k] = v
		}
		return out
	}
	return []*mcp.Tool{
		{Name: "terminal.list", Domain: "terminal", Level: mcp.Read, Desc: "List the terminals of a Bench (default: every terminal Bench of the selected Lab).",
			Params: map[string]any{"bench": str("Bench id")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					l, err := a.labOf("")
					if err != nil {
						return nil, err
					}
					out := []terminal.Info{}
					for _, b := range l.Benches {
						if b.Type != model.Terminal || c.Str("bench") != "" && b.ID != c.Str("bench") {
							continue
						}
						for _, t := range b.Ordered() {
							out = append(out, a.term.Describe(t))
						}
					}
					return out, nil
				})
			}},
		{Name: "terminal.create", Domain: "terminal", Level: mcp.Full, Desc: "Open a terminal Tab; `command` runs in place of the shell.",
			Params: map[string]any{"bench": str("Bench id"), "cwd": str("folder"), "command": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			Run: func(c *mcp.Call) (any, error) {
				var argv []string
				_ = c.Decode(&struct {
					Command *[]string `json:"command"`
				}{&argv})
				return on(c, func() (any, error) {
					b, err := a.bench(c, model.Terminal)
					if err != nil {
						return nil, err
					}
					t := a.term.Create(b, c.Str("cwd"), argv)
					t.Owner = c.Client.ID
					if err := a.term.Ensure(t); err != nil {
						return nil, termErr(err)
					}
					return map[string]string{"id": t.ID, "tab": t.ID}, nil
				})
			}},
		{Name: "terminal.write", Domain: "terminal", Level: mcp.Full, Desc: "Write text to the terminal as typed; no Enter is added.",
			Params: withID(map[string]any{"text": str("text")}), Required: []string{"id", "text"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.termTab(c)
					if err != nil {
						return nil, err
					}
					return map[string]bool{"ok": true}, termErr(a.term.Write(t, c.Str("text")))
				})
			}},
		{Name: "terminal.key", Domain: "terminal", Level: mcp.Full, Desc: "Press a key: Enter, Tab, Escape, Backspace, Up, Down, Left, Right, Home, End, PageUp, PageDown, ctrl+c, ctrl+d, alt+x…",
			Params: withID(map[string]any{"key": str("key name")}), Required: []string{"id", "key"},
			Run: func(c *mcp.Call) (any, error) {
				seq, ok := terminal.KeyBytes(c.Str("key"))
				if !ok {
					return nil, mcp.Errf(mcp.CodeInvalid, "unknown key %q", c.Str("key"))
				}
				return on(c, func() (any, error) {
					t, err := a.termTab(c)
					if err != nil {
						return nil, err
					}
					return map[string]bool{"ok": true}, termErr(a.term.Write(t, seq))
				})
			}},
		{Name: "terminal.read_screen", Domain: "terminal", Level: mcp.Read, Desc: "The text on the terminal's Screen.", Params: idParam, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.termTab(c)
					if err != nil {
						return nil, err
					}
					text, cols, rows, err := a.term.Text(t)
					if err != nil {
						return nil, termErr(err)
					}
					return map[string]any{"text": screenText(text, rows), "rows": rows, "cols": cols}, nil
				})
			}},
		{Name: "terminal.read_scrollback", Domain: "terminal", Level: mcp.Read, Desc: "The last lines of the terminal, scrollback included (default 200, at most 5000).",
			Params: withID(map[string]any{"lines": num("lines")}), Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				n := min(max(c.Int("lines", 200), 1), 5000)
				return on(c, func() (any, error) {
					t, err := a.termTab(c)
					if err != nil {
						return nil, err
					}
					text, _, _, err := a.term.Text(t)
					if err != nil {
						return nil, termErr(err)
					}
					total := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
					return map[string]any{"text": screenText(text, n), "total": total}, nil
				})
			}},
		{Name: "terminal.wait_idle", Domain: "terminal", Level: mcp.Read, Timeout: 30 * time.Second,
			Desc:   "Wait until the terminal is idle: no output for 500 ms and only the shell in front, or an agent that waits. Default timeout 30 s.",
			Params: idParam, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				var last string
				quiet := time.Now()
				for {
					var text string
					var rows int
					var idle bool
					_, err := on(c, func() (any, error) {
						t, err := a.termTab(c)
						if err != nil {
							return nil, err
						}
						var e error
						text, _, rows, e = a.term.Text(t)
						idle = a.term.Idle(t)
						return nil, termErr(e)
					})
					if err != nil {
						return nil, err
					}
					if text != last {
						last, quiet = text, time.Now()
					}
					if idle && time.Since(quiet) >= 500*time.Millisecond {
						return map[string]any{"idle": true, "screen": screenText(text, rows)}, nil
					}
					select {
					case <-c.Ctx.Done():
						return map[string]any{"idle": false, "screen": screenText(text, rows)}, nil
					case <-time.After(100 * time.Millisecond):
					}
				}
			}},
		{Name: "terminal.cwd", Domain: "terminal", Level: mcp.Read, Desc: "The terminal's current folder.", Params: idParam, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.termTab(c)
					if err != nil {
						return nil, err
					}
					return map[string]string{"cwd": a.term.Describe(t).Cwd}, nil
				})
			}},
		{Name: "terminal.processes", Domain: "terminal", Level: mcp.Read, Desc: "The shell and the processes under it.", Params: idParam, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				v, err := on(c, func() (any, error) {
					t, err := a.termTab(c)
					if err != nil {
						return nil, err
					}
					if err := a.term.Ensure(t); err != nil {
						return nil, termErr(err)
					}
					return a.term.Describe(t).PID, nil
				})
				if err != nil {
					return nil, err
				}
				return terminal.Processes(v.(int)), nil
			}},
		{Name: "terminal.close", Domain: "terminal", Level: mcp.Full, Destructive: true, Desc: "Close the terminal Tab and its Session.", Params: idParam, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.termTab(c)
					if err != nil {
						return nil, err
					}
					a.closeQuietly(t)
					return map[string]bool{"ok": true}, nil
				})
			}},
	}
}
