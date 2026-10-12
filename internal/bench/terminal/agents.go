package terminal

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"

	"github.com/sviniabanditka/notch-lab/internal/daemon"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// What the MCP tools of the domain `terminal` use (13-mcp.md). Main
// goroutine only, except Processes.

// ErrNoDaemon is returned while the Daemon is not connected.
var ErrNoDaemon = errors.New("the Daemon is not running")

// Info describes a terminal Tab for terminal.list.
type Info struct {
	ID    string        `json:"id"`
	Tab   string        `json:"tab"`
	Title string        `json:"title"`
	Cwd   string        `json:"cwd"`
	PID   int           `json:"pid"`
	Agent *daemon.Agent `json:"agent,omitempty"`
	Rows  int           `json:"rows"`
	Cols  int           `json:"cols"`
}

// Ensure attaches a Tab's Session, or creates it, without a view.
func (b *Bench) Ensure(t *model.Tab) error {
	if b.client == nil {
		return ErrNoDaemon
	}
	if b.live(t) == nil {
		return errors.New("the Session could not start")
	}
	return nil
}

// Describe is the Tab's Info; the id is the Tab id.
func (b *Bench) Describe(t *model.Tab) Info {
	in := Info{ID: t.ID, Tab: t.ID, Title: t.DisplayTitle(), Cwd: readData(t).Cwd}
	if x := b.tabs[t]; x != nil {
		in.PID, in.Agent = x.st.PID, x.st.Agent
		if x.term != nil {
			in.Cols, in.Rows = x.term.Size()
		}
	}
	return in
}

// Write sends bytes to the Session as typed.
func (b *Bench) Write(t *model.Tab, text string) error {
	if err := b.Ensure(t); err != nil {
		return err
	}
	b.tabs[t].term.Send([]byte(text))
	return nil
}

var keys = map[string]string{
	"enter": "\r", "return": "\r", "tab": "\t", "escape": "\x1b", "esc": "\x1b", "backspace": "\x7f",
	"space": " ", "up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
	"home": "\x1b[H", "end": "\x1b[F", "pageup": "\x1b[5~", "pagedown": "\x1b[6~", "delete": "\x1b[3~",
}

// KeyBytes is what a named key sends: Enter, Tab, Escape, Up, ctrl+c…
func KeyBytes(name string) (string, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if s, ok := keys[n]; ok {
		return s, true
	}
	if l, ok := strings.CutPrefix(n, "ctrl+"); ok && len(l) == 1 && l[0] >= 'a' && l[0] <= 'z' {
		return string(rune(l[0] - 'a' + 1)), true
	}
	if l, ok := strings.CutPrefix(n, "alt+"); ok && len(l) == 1 {
		return "\x1b" + l, true
	}
	return "", false
}

// Text is the Screen and the scrollback as text, and the size.
func (b *Bench) Text(t *model.Tab) (text string, cols, rows int, err error) {
	if err := b.Ensure(t); err != nil {
		return "", 0, 0, err
	}
	x := b.tabs[t]
	cols, rows = x.term.Size()
	return x.term.Text(), cols, rows, nil
}

// Idle reports a Session with no program other than the shell in front,
// or an Agent that waits (idle, blocked or done).
func (b *Bench) Idle(t *model.Tab) bool {
	x := b.tabs[t]
	if x == nil {
		return false
	}
	if a := x.st.Agent; a != nil {
		return a.Status != "working"
	}
	return x.st.Shell || x.st.Exited
}

// Create opens a terminal Tab in a Bench; argv runs in place of the shell.
func (b *Bench) Create(bench *model.Bench, cwd string, argv []string) *model.Tab {
	if cwd == "" {
		cwd = bench.EffectiveRoot(b.Home)
	}
	title := cwd
	if len(argv) > 0 {
		title = argv[0]
	}
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: title, Data: "{}"}
	writeData(t, data{Cwd: cwd, Argv: argv})
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t
}

// Process is one process under a Session's shell.
type Process struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// Processes lists the shell and every process under it. Any goroutine.
func Processes(shell int) []Process {
	out, err := exec.Command("/bin/ps", "-A", "-o", "pid=,ppid=,comm=,args=").Output()
	if err != nil || shell == 0 {
		return nil
	}
	type row struct {
		ppid int
		p    Process
	}
	rows := map[int]row{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		rows[pid] = row{ppid, Process{PID: pid, Name: f[2], Args: strings.Join(f[3:], " ")}}
	}
	var list []Process
	var walk func(pid int)
	walk = func(pid int) {
		if r, ok := rows[pid]; ok {
			list = append(list, r.p)
		}
		for child, r := range rows {
			if r.ppid == pid {
				walk(child)
			}
		}
	}
	walk(shell)
	return list
}
