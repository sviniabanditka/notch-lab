package daemon

import (
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/egoist/mygo/plugins/terminal"
	"golang.org/x/sys/unix"
)

// session is one pseudo-terminal and the headless terminal that keeps its
// Screen and Scrollback.
type session struct {
	d    *Daemon
	id   string
	argv []string
	ptmx *os.File
	cmd  *exec.Cmd
	head *terminal.Terminal

	mu      sync.Mutex // guards clients, st and the order of Feed and fan-out
	clients map[*conn]bool
	st      State
	nclient atomic.Int32
	quit    chan struct{}
}

// headConn is the headless terminal's Conn: it never sends output (the
// session Feeds it), and its answers to device queries reach the program
// only while no client is attached, since an attached client answers
// them itself.
type headConn struct {
	s    *session
	done chan struct{}
}

func (h *headConn) Read([]byte) (int, error) { <-h.done; return 0, io.EOF }
func (h *headConn) Write(p []byte) (int, error) {
	if h.s.nclient.Load() == 0 && h.s.ptmx != nil {
		return h.s.ptmx.Write(p)
	}
	return len(p), nil
}
func (h *headConn) Close() error { return nil }

func (d *Daemon) newSession(id, cwd string, argv []string, env map[string]string, cols, rows int) (*session, error) {
	if cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}
	if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
		cwd, _ = os.UserHomeDir()
	}
	s := &session{d: d, id: id, argv: argv, clients: map[*conn]bool{}, quit: make(chan struct{})}
	hc := &headConn{s: s, done: s.quit}
	head, err := terminal.New(terminal.Options{Conn: hc, Scrollback: d.bufferBytes})
	if err != nil {
		return nil, err
	}
	head.Resize(cols, rows)
	s.head = head

	path, args := d.command(argv)
	cmd := exec.Command(path)
	cmd.Args = args
	cmd.Dir = cwd
	cmd.Env = d.environment(id, env)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		head.Close()
		return nil, err
	}
	s.ptmx, s.cmd = ptmx, cmd
	s.st = State{ID: id, Cwd: cwd, Argv: argv, Cols: cols, Rows: rows, PID: cmd.Process.Pid, Shell: true, Fg: shellName(path)}
	go s.read()
	go s.wait()
	go s.poll()
	return s, nil
}

func shellName(path string) string {
	i := strings.LastIndexByte(path, '/')
	return path[i+1:]
}

// read feeds the output to the headless terminal and to every attached
// client, in one critical section, so an attach sees each byte once:
// in its Snapshot or after it.
func (s *session) read() {
	buf := make([]byte, 64<<10)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.head.Feed(chunk)
			for c := range s.clients {
				c.send(&Msg{T: "session.data", Session: s.id, B: chunk})
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *session) wait() {
	err := s.cmd.Wait()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	}
	time.Sleep(50 * time.Millisecond) // let read() take the last output
	s.mu.Lock()
	s.st.Exited, s.st.Code, s.st.Agent = true, code, nil
	s.st.Fg, s.st.Shell = "", false
	st := s.st
	for c := range s.clients {
		c.send(&Msg{T: "session.state", State: &st})
	}
	s.mu.Unlock()
	log.Printf("session %s exited with %d", s.id, code)
}

// poll refreshes the foreground process, the folder, the title and the
// Agent once a second, and tells the clients of a change (R-TERM-030).
func (s *session) poll() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var since time.Time // when the Agent went working
	for {
		select {
		case <-s.quit:
			return
		case <-tick.C:
		}
		s.mu.Lock()
		if s.st.Exited {
			s.mu.Unlock()
			return
		}
		next := s.st
		s.mu.Unlock()

		if pgid, err := unix.IoctlGetInt(int(s.ptmx.Fd()), unix.TIOCGPGRP); err == nil && pgid > 0 {
			procs := groupProcs(pgid)
			next.Shell = pgid == s.cmd.Process.Pid
			next.Fg = ""
			next.Agent = nil
			for _, p := range procs {
				if p.PID == pgid {
					next.Fg = p.Name
				}
				if k := agentKind(p); k != "" && next.Agent == nil {
					next.Agent = &Agent{Kind: k, PID: p.PID}
				}
			}
			if cwd := procCwd(pgid); cwd != "" {
				next.Cwd = cwd
			}
		}
		if dir := s.head.Dir(); dir != "" && next.Shell {
			next.Cwd = dir
		}
		next.Title = s.head.Title()
		if next.Agent != nil {
			status := screenStatus(lastLines(s.head.Text(), 40))
			prev := ""
			if s.st.Agent != nil {
				prev = s.st.Agent.Status
			}
			switch {
			case status == "working":
				since = time.Now()
			case status == "idle" && (prev == "working" || prev == "blocked" || prev == "done") && !since.IsZero():
				status = "done" // stays done until it works again
			}
			next.Agent.Status = status
		}
		s.mu.Lock()
		if !sameState(s.st, next) {
			s.st = next
			st := next
			for c := range s.clients {
				c.send(&Msg{T: "session.state", State: &st})
			}
		}
		s.mu.Unlock()
	}
}

func sameState(a, b State) bool {
	if (a.Agent == nil) != (b.Agent == nil) || a.Agent != nil && *a.Agent != *b.Agent {
		return false
	}
	return a.Cwd == b.Cwd && a.Title == b.Title && a.Fg == b.Fg && a.Shell == b.Shell && a.Cols == b.Cols && a.Rows == b.Rows && a.Exited == b.Exited
}

// lastLines returns the last n lines of text.
// ponytail: Text() renders the whole Scrollback each second while an Agent
// runs; ask the Screen only when libghostty-vt exposes it through the plugin.
func lastLines(text string, n int) string {
	text = strings.TrimRight(text, "\n ")
	i := len(text)
	for ; n > 0; n-- {
		i = strings.LastIndexByte(text[:i], '\n')
		if i < 0 {
			return text
		}
	}
	return text[i+1:]
}

// screenStatus reads an Agent's status from the text on its Screen, as
// herdr does (12-agents.md R-AGENT-010).
func screenStatus(screen string) string {
	low := strings.ToLower(screen)
	for _, s := range []string{"do you want to proceed?", "do you want to make this edit", "do you want to create", "tab to amend", "allow command?", "approve this", "press enter to confirm"} {
		if strings.Contains(low, s) {
			return "blocked"
		}
	}
	if strings.Contains(low, "esc to interrupt") || strings.Contains(low, "esc to cancel") {
		return "working"
	}
	return "idle"
}

// attach adds a client, sized to cols and rows, and sends it the Snapshot.
func (s *session) attach(c *conn, cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cols > 0 && rows > 0 {
		s.resizeLocked(cols, rows)
	}
	snap := s.head.Snapshot()
	if !s.clients[c] {
		s.clients[c] = true
		s.nclient.Add(1)
	}
	st := s.st
	c.send(&Msg{T: "session.state", State: &st})
	c.send(&Msg{T: "session.data", Session: s.id, B: snap})
}

func (s *session) detach(c *conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[c] {
		delete(s.clients, c)
		s.nclient.Add(-1)
	}
}

func (s *session) resizeLocked(cols, rows int) {
	if cols == s.st.Cols && rows == s.st.Rows {
		return
	}
	s.st.Cols, s.st.Rows = cols, rows
	s.head.Resize(cols, rows)
	_ = pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *session) resize(cols, rows int) {
	s.mu.Lock()
	s.resizeLocked(cols, rows)
	s.mu.Unlock()
}

// write sends input to the program; a write that blocks 5 s fails (R-TERM-027).
func (s *session) write(b []byte) error {
	done := make(chan error, 1)
	go func() { _, err := s.ptmx.Write(b); done <- err }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		return &Error{Code: "stalled", Message: "the program does not read its input"}
	}
}

func (s *session) signal(name string) {
	sig := map[string]syscall.Signal{"int": syscall.SIGINT, "term": syscall.SIGTERM, "kill": syscall.SIGKILL, "hup": syscall.SIGHUP}[name]
	if sig == 0 || s.cmd.Process == nil {
		return
	}
	if pgid, err := unix.IoctlGetInt(int(s.ptmx.Fd()), unix.TIOCGPGRP); err == nil && pgid > 0 {
		_ = syscall.Kill(-pgid, sig)
		return
	}
	_ = syscall.Kill(-s.cmd.Process.Pid, sig)
}

// close hangs up the program, kills it after 2 s, and frees the Session.
func (s *session) close() {
	select {
	case <-s.quit:
		return
	default:
	}
	close(s.quit)
	if s.cmd.ProcessState == nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGHUP)
		go func() {
			time.Sleep(2 * time.Second)
			if s.cmd.ProcessState == nil {
				_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			}
		}()
	}
	s.ptmx.Close()
	s.head.Close()
	s.mu.Lock()
	for c := range s.clients {
		c.send(&Msg{T: "session.closed", Session: s.id})
	}
	s.clients = map[*conn]bool{}
	s.mu.Unlock()
}

func (s *session) state() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}
