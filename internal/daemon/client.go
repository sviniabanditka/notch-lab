package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Client is the app's connection to the Daemon.
type Client struct {
	nc     net.Conn
	wmu    sync.Mutex
	mu     sync.Mutex
	next   int64
	wait   map[int64]chan *Msg
	conns  map[string]*SessionConn
	closed chan struct{}
	err    error

	// OnState hears every session.state; OnLost hears the connection end.
	// Both run on the Client's goroutine.
	OnState func(State)
	OnLost  func(error)
}

// Dial connects to the Daemon of a home and says hello (R-TERM-010).
func Dial(home, version string) (*Client, error) {
	nc, err := net.DialTimeout("unix", SocketPath(home), time.Second)
	if err != nil {
		return nil, err
	}
	if err := writeMsg(nc, &Msg{T: "hello", Protocol: Protocol, Client: "notch-lab " + version}); err != nil {
		nc.Close()
		return nil, err
	}
	_ = nc.SetReadDeadline(time.Now().Add(3 * time.Second))
	hello, err := readMsg(nc)
	_ = nc.SetReadDeadline(time.Time{})
	if err != nil {
		nc.Close()
		return nil, err
	}
	if hello.T == "error" && hello.Err != nil {
		nc.Close()
		return nil, hello.Err
	}
	c := &Client{nc: nc, wait: map[int64]chan *Msg{}, conns: map[string]*SessionConn{}, closed: make(chan struct{})}
	go c.read()
	return c, nil
}

// Closed is closed once the connection ended.
func (c *Client) Closed() <-chan struct{} { return c.closed }

// Close ends the connection; Sessions stay in the Daemon.
func (c *Client) Close() error { return c.nc.Close() }

func (c *Client) read() {
	var err error
	for {
		var m *Msg
		if m, err = readMsg(c.nc); err != nil {
			break
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.wait[m.ID]
			delete(c.wait, m.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
			continue
		}
		c.mu.Lock()
		sc := c.conns[m.Session]
		if m.State != nil {
			sc = c.conns[m.State.ID]
		}
		c.mu.Unlock()
		switch m.T {
		case "session.data":
			if sc != nil {
				sc.push(m.B)
			}
		case "session.state":
			if sc != nil {
				sc.setState(*m.State)
			}
			if c.OnState != nil {
				c.OnState(*m.State)
			}
		case "session.closed":
			if sc != nil {
				sc.end()
			}
		}
	}
	c.mu.Lock()
	c.err = err
	for id, ch := range c.wait {
		close(ch)
		delete(c.wait, id)
	}
	conns := c.conns
	c.mu.Unlock()
	close(c.closed)
	for _, sc := range conns {
		sc.lost()
	}
	if c.OnLost != nil {
		c.OnLost(err)
	}
}

// call sends a request and waits for its answer, 10 s at most.
func (c *Client) call(m *Msg, out any) error {
	ch := make(chan *Msg, 1)
	c.mu.Lock()
	c.next++
	m.ID = c.next
	c.wait[m.ID] = ch
	c.mu.Unlock()
	c.wmu.Lock()
	err := writeMsg(c.nc, m)
	c.wmu.Unlock()
	if err != nil {
		return err
	}
	select {
	case ans, ok := <-ch:
		if !ok {
			return errors.New("the terminal daemon went away")
		}
		if ans.Err != nil {
			return ans.Err
		}
		if out != nil {
			return json.Unmarshal(ans.OK, out)
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("the terminal daemon did not answer %s", m.T)
	}
}

// Create starts a Session and returns its id. An empty argv runs the login
// shell; id may name the Session, as for a Tab that lost its own.
func (c *Client) Create(id, cwd string, argv []string, env map[string]string, cols, rows int) (string, error) {
	var ok struct{ ID string }
	err := c.call(&Msg{T: "session.create", Session: id, Cwd: cwd, Argv: argv, Env: env, Cols: cols, Rows: rows}, &ok)
	return ok.ID, err
}

// List returns every Session's State.
func (c *Client) List() ([]State, error) {
	var ok struct{ Sessions []State }
	err := c.call(&Msg{T: "session.list"}, &ok)
	return ok.Sessions, err
}

// CloseSession ends a Session in the Daemon.
func (c *Client) CloseSession(id string) error {
	return c.call(&Msg{T: "session.close", Session: id}, nil)
}

// Signal sends int, term, kill or hup to a Session's foreground group.
func (c *Client) Signal(id, sig string) error {
	return c.call(&Msg{T: "session.signal", Session: id, Signal: sig}, nil)
}

// Open returns the Conn of a Session for a terminal view (terminal.Options.Conn).
// It attaches when the view first tells its size, or after 300 ms at the
// Session's size.
func (c *Client) Open(id string) *SessionConn {
	sc := &SessionConn{c: c, id: id}
	sc.cond = sync.NewCond(&sc.mu)
	c.mu.Lock()
	c.conns[id] = sc
	c.mu.Unlock()
	sc.timer = time.AfterFunc(300*time.Millisecond, func() { sc.Resize(0, 0) })
	return sc
}

// SessionConn is a Session as an io.ReadWriteCloser with Resize, for the
// terminal plugin's Conn.
type SessionConn struct {
	c        *Client
	id       string
	mu       sync.Mutex
	cond     *sync.Cond
	buf      [][]byte
	eof      bool
	attached bool
	timer    *time.Timer
	st       State

	// OnState hears this Session's state; it runs on the Client's goroutine.
	OnState func(State)
}

func (sc *SessionConn) push(b []byte) {
	sc.mu.Lock()
	sc.buf = append(sc.buf, b)
	sc.cond.Broadcast()
	sc.mu.Unlock()
}

func (sc *SessionConn) setState(st State) {
	sc.mu.Lock()
	sc.st = st
	fn := sc.OnState
	sc.mu.Unlock()
	if fn != nil {
		fn(st)
	}
}

// State is the last state the Daemon sent.
func (sc *SessionConn) State() State {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.st
}

func (sc *SessionConn) end() {
	sc.mu.Lock()
	sc.eof = true
	sc.cond.Broadcast()
	sc.mu.Unlock()
}

// lost marks a Conn whose Daemon connection ended; a new attach is needed.
func (sc *SessionConn) lost() {
	sc.mu.Lock()
	sc.attached = false
	sc.mu.Unlock()
}

// Read returns the Session's output: the Snapshot first, then live bytes.
func (sc *SessionConn) Read(p []byte) (int, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for len(sc.buf) == 0 && !sc.eof {
		sc.cond.Wait()
	}
	if len(sc.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, sc.buf[0])
	if n == len(sc.buf[0]) {
		sc.buf = sc.buf[1:]
	} else {
		sc.buf[0] = sc.buf[0][n:]
	}
	return n, nil
}

// Write sends input, in writes of 64 KiB at most (R-TERM-026).
func (sc *SessionConn) Write(p []byte) (int, error) {
	for off := 0; off < len(p); off += maxWrite {
		end := min(off+maxWrite, len(p))
		if err := sc.c.call(&Msg{T: "session.write", Session: sc.id, B: append([]byte(nil), p[off:end]...)}, nil); err != nil {
			return off, err
		}
	}
	return len(p), nil
}

// Resize attaches on the first call, then resizes the Session.
func (sc *SessionConn) Resize(cols, rows int) error {
	sc.mu.Lock()
	first := !sc.attached
	sc.attached = true
	if sc.timer != nil && cols > 0 {
		sc.timer.Stop()
	}
	sc.mu.Unlock()
	if first {
		return sc.c.call(&Msg{T: "session.attach", Session: sc.id, Cols: cols, Rows: rows}, nil)
	}
	if cols <= 0 {
		return nil
	}
	return sc.c.call(&Msg{T: "session.resize", Session: sc.id, Cols: cols, Rows: rows}, nil)
}

// Reattach attaches again after the Client changed (a Daemon reconnect).
func (sc *SessionConn) Reattach(c *Client) error {
	sc.mu.Lock()
	sc.c, sc.attached, sc.buf = c, true, nil
	sc.mu.Unlock()
	c.mu.Lock()
	c.conns[sc.id] = sc
	c.mu.Unlock()
	st := sc.State()
	return c.call(&Msg{T: "session.attach", Session: sc.id, Cols: st.Cols, Rows: st.Rows}, nil)
}

// Close detaches the view; the Session goes on in the Daemon.
func (sc *SessionConn) Close() error {
	sc.mu.Lock()
	if sc.timer != nil {
		sc.timer.Stop()
	}
	sc.eof = true
	sc.cond.Broadcast()
	c := sc.c
	sc.mu.Unlock()
	c.mu.Lock()
	if c.conns[sc.id] == sc {
		delete(c.conns, sc.id)
	}
	c.mu.Unlock()
	select {
	case <-c.closed:
		return nil
	default:
	}
	return c.call(&Msg{T: "session.detach", Session: sc.id}, nil)
}

func dialRaw(home string) (net.Conn, error) { return net.Dial("unix", SocketPath(home)) }
