package daemon

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// maxSessions limits the Sessions of a Daemon (R-TERM-039).
const maxSessions = 256

// Daemon owns the Sessions and serves clients on a Unix socket.
type Daemon struct {
	Version     string
	home        string
	bufferBytes int
	ln          net.Listener

	mu       sync.Mutex
	sessions map[string]*session
	shell    string
	envOnce  sync.Once
	baseEnv  []string
}

// SocketPath is where the Daemon of a home listens (R-TERM-003).
func SocketPath(home string) string { return filepath.Join(home, "daemon.sock") }

// Listen takes the lock and the socket of a home. A second Daemon fails
// with ErrRunning (R-TERM-008).
func Listen(home, version string) (*Daemon, error) {
	lock, err := os.OpenFile(filepath.Join(home, "daemon.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		return nil, ErrRunning
	}
	// lock stays open for the Daemon's life.
	sock := SocketPath(home)
	_ = os.Remove(sock)
	old := syscall.Umask(0o177)
	ln, err := net.Listen("unix", sock)
	syscall.Umask(old)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(sock, 0o600)
	d := &Daemon{Version: version, home: home, ln: ln, sessions: map[string]*session{}, bufferBytes: 8 << 20}
	d.readConfig()
	return d, nil
}

// ErrRunning means another Daemon holds the home's lock.
var ErrRunning = errors.New("another notch-labd runs for this home")

// readConfig reads daemon.json: {"buffer_bytes": n} (R-TERM-035).
func (d *Daemon) readConfig() {
	var cfg struct {
		BufferBytes int `json:"buffer_bytes"`
	}
	if data, err := os.ReadFile(filepath.Join(d.home, "daemon.json")); err == nil && json.Unmarshal(data, &cfg) == nil && cfg.BufferBytes > 0 {
		d.bufferBytes = min(64<<20, max(1<<20, cfg.BufferBytes))
	}
}

// Serve accepts clients until ctx ends, then closes every Session (R-TERM-038).
func (d *Daemon) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		d.ln.Close()
	}()
	for {
		nc, err := d.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				d.closeAll()
				return nil
			}
			return err
		}
		if !sameUser(nc) {
			nc.Close() // R-TERM-003: before a byte is read
			continue
		}
		c := newConn(d, nc)
		go c.serve()
	}
}

func sameUser(nc net.Conn) bool {
	uc, ok := nc.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	same := false
	_ = raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		same = err == nil && int(cred.Uid) == os.Getuid()
	})
	return same
}

func (d *Daemon) closeAll() {
	d.mu.Lock()
	list := d.sessions
	d.sessions = map[string]*session{}
	d.mu.Unlock()
	for _, s := range list {
		s.close()
	}
}

func (d *Daemon) session(id string) *session {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessions[id]
}

// loginShell is the user's shell (R-TERM-032).
func (d *Daemon) loginShell() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.shell != "" {
		return d.shell
	}
	if u, err := user.Current(); err == nil {
		if out, err := exec.Command("/usr/bin/dscl", ".", "-read", "/Users/"+u.Username, "UserShell").Output(); err == nil {
			if f := strings.Fields(string(out)); len(f) == 2 {
				d.shell = f[1]
			}
		}
	}
	if d.shell == "" {
		d.shell = os.Getenv("SHELL")
	}
	if d.shell == "" {
		d.shell = "/bin/zsh"
	}
	return d.shell
}

// command returns the program path and argv: the login shell as "-zsh"
// when argv is empty.
func (d *Daemon) command(argv []string) (string, []string) {
	if len(argv) > 0 {
		if p, err := exec.LookPath(argv[0]); err == nil {
			return p, argv
		}
		return argv[0], argv
	}
	sh := d.loginShell()
	return sh, []string{"-" + filepath.Base(sh)}
}

// environment is the child's environment (R-TERM-033, R-TERM-034, R-TERM-134).
func (d *Daemon) environment(id string, extra map[string]string) []string {
	d.envOnce.Do(func() {
		d.baseEnv = os.Environ()
		if os.Getenv("NOTCH_LAB_LAUNCHD") == "" {
			return // started from the app: its environment is the user's
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, d.loginShell(), "-l", "-c", "env").Output()
		path, home := "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin", os.Getenv("HOME")
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if v, ok := strings.CutPrefix(line, "PATH="); ok {
					path = v
				}
				if v, ok := strings.CutPrefix(line, "HOME="); ok {
					home = v
				}
			}
		}
		d.baseEnv = append(d.baseEnv, "PATH="+path, "HOME="+home)
	})
	set := map[string]string{}
	var order []string
	put := func(k, v string) {
		if strings.HasPrefix(k, "DYLD_") || strings.HasPrefix(k, "LD_") {
			return
		}
		if _, ok := set[k]; !ok {
			order = append(order, k)
		}
		set[k] = v
	}
	for _, kv := range d.baseEnv {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "NOTCH_LAB_LAUNCHD" && k != "MYGO_GHOSTTY_VT" {
			put(k, v)
		}
	}
	put("TERM", "xterm-256color")
	put("COLORTERM", "truecolor")
	put("NOTCH_LAB", "1")
	put("NOTCH_LAB_SESSION", id)
	put("TERM_PROGRAM", "notch-lab")
	put("TERM_PROGRAM_VERSION", d.Version)
	if set["LANG"] == "" {
		put("LANG", "en_US.UTF-8")
	}
	for k, v := range extra {
		put(k, v)
	}
	out := make([]string, 0, len(order))
	for _, k := range order {
		out = append(out, k+"="+set[k])
	}
	return out
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// conn is one client connection.
type conn struct {
	d    *Daemon
	nc   net.Conn
	out  chan *Msg
	done chan struct{}
	once sync.Once
}

func newConn(d *Daemon, nc net.Conn) *conn {
	return &conn{d: d, nc: nc, out: make(chan *Msg, 4096), done: make(chan struct{})}
}

// send queues a frame; a client too slow to take 4096 frames is dropped.
func (c *conn) send(m *Msg) {
	select {
	case c.out <- m:
	case <-c.done:
	default:
		log.Printf("client too slow; closing it")
		c.close()
	}
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		c.nc.Close()
	})
}

func (c *conn) serve() {
	defer func() {
		c.close()
		c.d.mu.Lock()
		list := make([]*session, 0, len(c.d.sessions))
		for _, s := range c.d.sessions {
			list = append(list, s)
		}
		c.d.mu.Unlock()
		for _, s := range list {
			s.detach(c)
		}
	}()
	go func() {
		for {
			select {
			case m := <-c.out:
				if err := writeMsg(c.nc, m); err != nil {
					c.close()
					return
				}
			case <-c.done:
				return
			}
		}
	}()
	hello, err := readMsg(c.nc)
	if err != nil || hello.T != "hello" {
		return
	}
	if hello.Protocol != Protocol {
		c.send(&Msg{T: "error", Err: &Error{Code: "protocol", Message: fmt.Sprintf("the daemon speaks protocol %d, the client %d", Protocol, hello.Protocol)}})
		time.Sleep(50 * time.Millisecond)
		return
	}
	c.send(&Msg{T: "hello", Protocol: Protocol, Daemon: "notch-labd " + c.d.Version, PID: os.Getpid()})
	for {
		m, err := readMsg(c.nc)
		if err != nil {
			return
		}
		ok, rerr := c.handle(m)
		ans := &Msg{T: m.T, ID: m.ID}
		var e *Error
		switch {
		case rerr == nil:
			ans.OK, _ = json.Marshal(ok)
			if ans.OK == nil || string(ans.OK) == "null" {
				ans.OK = json.RawMessage("{}")
			}
		case errors.As(rerr, &e):
			ans.Err = e
		default:
			ans.Err = &Error{Code: "failed", Message: rerr.Error()}
		}
		if m.T != "session.attach" {
			c.send(ans)
		}
	}
}

func (c *conn) handle(m *Msg) (any, error) {
	d := c.d
	if m.T == "session.create" {
		d.mu.Lock()
		n := len(d.sessions)
		d.mu.Unlock()
		if n >= maxSessions {
			return nil, ErrLimit
		}
		id := m.Session
		if id == "" || d.session(id) != nil {
			id = newID()
		}
		s, err := d.newSession(id, m.Cwd, m.Argv, m.Env, m.Cols, m.Rows)
		if err != nil {
			return nil, err
		}
		d.mu.Lock()
		d.sessions[id] = s
		d.mu.Unlock()
		log.Printf("session %s created in %s", id, m.Cwd)
		return map[string]string{"id": id}, nil
	}
	if m.T == "session.list" {
		d.mu.Lock()
		list := make([]State, 0, len(d.sessions))
		for _, s := range d.sessions {
			list = append(list, s.state())
		}
		d.mu.Unlock()
		return map[string]any{"sessions": list}, nil
	}
	s := d.session(m.Session)
	if s == nil {
		return nil, ErrUnknown
	}
	switch m.T {
	case "session.attach":
		// The answer goes first, then the state and the Snapshot, in order.
		c.send(&Msg{T: m.T, ID: m.ID, OK: json.RawMessage("{}")})
		s.attach(c, m.Cols, m.Rows)
		return nil, nil
	case "session.detach":
		s.detach(c)
	case "session.write":
		if len(m.B) > maxWrite {
			return nil, ErrTooBig
		}
		return nil, s.write(m.B)
	case "session.resize":
		s.resize(m.Cols, m.Rows)
	case "session.signal":
		s.signal(m.Signal)
	case "session.close":
		d.mu.Lock()
		delete(d.sessions, s.id)
		d.mu.Unlock()
		s.close()
		log.Printf("session %s closed", s.id)
	case "session.state":
		st := s.state()
		return st, nil
	default:
		return nil, &Error{Code: "unsupported", Message: m.T}
	}
	return nil, nil
}
