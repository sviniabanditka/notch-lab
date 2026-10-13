package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"github.com/sviniabanditka/notch-lab/internal/extensions"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

// A WebSocket for an Extension's worker, as Search's ExtensionSocket.
// WebKit runs a worker on the main thread of its web process, and a
// worker's own WebSocket waits there for the main thread to set up its
// channel: the worker waits on itself for ever. So the Shim's WebSocket in
// a worker is made here, and its frames go over a native port: text as it
// is, binary as base64. The Origin is the one Chrome sends.

// socketApp is the native application the Shim connects to for a socket.
const socketApp = extensions.Application + ".socket"

// socket is one port and its WebSocket. Its fields without mu belong to
// the main goroutine.
type socket struct {
	b          *Bench
	e          *Ext
	ext        string // the Extension's id
	port       *webkit.Port
	origin     string
	seen, sent int              // dev runs: frames logged
	inbox      []map[string]any // frames read, not yet posted; under mu
	ctx        context.Context
	cancel     context.CancelFunc
	// started is true once "open" came; ended once either end let go.
	started, ended bool

	mu    sync.Mutex
	conn  *websocket.Conn
	queue []wsFrame
	wake  chan struct{}
	// closing is the close the Shim asked for, once it is written.
	closing *wsFrame
}

// wsFrame is one thing to write: text, binary or (close) a close.
type wsFrame struct {
	binary, close bool
	data          []byte
	code          websocket.StatusCode
}

func (e *Ext) connectSocket(cx *webkit.Context, p *webkit.Port) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &socket{b: e.b, e: e, ext: cx.ID, port: p, origin: "chrome-extension://" + cx.ID, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	// A worker that starts again opens its socket anew, and WebKit carries
	// what the app still sends on the old one into the new: the old
	// socket's frames reached the new handshake (SignalR's "Message is
	// incomplete"). The Extension's older sockets end first.
	// Its port is left alone: closing it closed the new one too.
	for _, old := range e.sockets[cx.ID] {
		old.end(false)
	}
	e.sockets[cx.ID] = []*socket{s}
	if os.Getenv("NOTCH_LAB_HOME") != "" {
		log.Printf("extensions: %s: socket port opened", cx.Name)
	}
	p.SetMessageHandler(s.take)
	p.SetDisconnectHandler(func() { s.end(false) })
	// No "ready" yet, unlike Search: the Shim takes any message for the
	// sign that its "open" arrived and stops saying it; WebKit drops what
	// a worker posts on a port in its first moments, so an early "ready"
	// left the socket unopened for good. "ready" answers "open" (take).
}

// take is a message from the Shim.
func (s *socket) take(raw []byte) {
	// WebKit may hand what the worker sends on its new port to the old
	// port's handler: an ended socket gives it to the Extension's current
	// one.
	if s.ended {
		if cur := s.e.sockets[s.ext]; len(cur) > 0 && cur[0] != s {
			cur[0].take(raw)
		}
		return
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return
	}
	// The worker's Shim asks every port to an app whether it arrived
	// before it sends on it (see nativehost.go): this one answers too.
	if w, ok := m["__searchNative"]; ok {
		if w == "here?" {
			s.post(map[string]any{"__searchNative": "here"})
		}
		return
	}
	if addr, ok := m["open"].(string); ok {
		if os.Getenv("NOTCH_LAB_HOME") != "" && !s.started {
			log.Printf("extensions: socket open %s", addr[:min(len(addr), 80)])
		}
		// Said again now that the worker surely listens: the first one is
		// often lost. The worker says "open" until it hears back.
		s.post(map[string]any{"ready": true})
		if s.started {
			return
		}
		var protocols []string
		if list, ok := m["protocols"].([]any); ok {
			for _, p := range list {
				if p, ok := p.(string); ok {
					protocols = append(protocols, p)
				}
			}
		}
		ua, _ := m["userAgent"].(string)
		s.start(addr, protocols, ua)
	} else if text, ok := m["send"].(string); ok {
		if os.Getenv("NOTCH_LAB_HOME") != "" && s.sent < 4 {
			s.sent++
			log.Printf("extensions: socket out %q", text[:min(len(text), 200)])
		}
		s.write(wsFrame{data: []byte(text)})
	} else if enc, ok := m["sendBinary"].(string); ok {
		if data, err := base64.StdEncoding.DecodeString(enc); err == nil {
			s.write(wsFrame{binary: true, data: data})
		}
	} else if c, ok := m["close"]; ok {
		code := websocket.StatusNormalClosure
		if n, ok := c.(float64); ok && (n == 1000 || (n >= 3000 && n <= 4999)) {
			code = websocket.StatusCode(n)
		}
		reason, _ := m["reason"].(string)
		s.write(wsFrame{close: true, code: code, data: []byte(reason)})
	}
}

// start dials the address off the main goroutine, then reads and writes
// until the socket ends.
func (s *socket) start(addr string, protocols []string, ua string) {
	s.started = true
	u, err := url.Parse(addr)
	if err != nil || (strings.ToLower(u.Scheme) != "ws" && strings.ToLower(u.Scheme) != "wss") {
		s.fail()
		return
	}
	h := http.Header{"Origin": {s.origin}}
	if ua != "" {
		h.Set("User-Agent", ua)
	}
	go func() {
		conn, _, err := websocket.Dial(s.ctx, addr, &websocket.DialOptions{HTTPHeader: h, Subprotocols: protocols})
		if err != nil {
			s.b.Post(s.fail)
			return
		}
		conn.SetReadLimit(-1)
		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()
		// "opened" goes in the same queue as the frames, first: a frame
		// before it is lost to the page.
		s.mu.Lock()
		s.inbox = append(s.inbox, map[string]any{"opened": conn.Subprotocol()})
		s.mu.Unlock()
		s.b.Post(s.drain)
		go s.writer(conn)
		s.reader(conn)
	}()
}

// reader posts each frame as it comes, then how the socket ended.
func (s *socket) reader(conn *websocket.Conn) {
	for {
		typ, data, err := conn.Read(s.ctx)
		if err != nil {
			code := websocket.CloseStatus(err)
			var ce websocket.CloseError
			reason := ""
			if errors.As(err, &ce) {
				reason = ce.Reason
			}
			s.mu.Lock()
			if c := s.closing; code == -1 && c != nil {
				code, reason = c.code, string(c.data) // the close this end asked for
			}
			s.mu.Unlock()
			s.b.Post(func() {
				s.drain() // the frames before the end, first
				if s.ended {
					return
				}
				if code == -1 {
					s.fail()
					return
				}
				s.post(map[string]any{"closed": int(code), "reason": reason, "clean": true})
				s.end(true)
			})
			return
		}
		if os.Getenv("NOTCH_LAB_HOME") != "" && s.seen < 4 { // dev runs: the first frames, as they come
			s.seen++
			log.Printf("extensions: socket in %v %q", typ, data[:min(len(data), 200)])
		}
		msg := map[string]any{"text": string(data)}
		if typ == websocket.MessageBinary {
			msg = map[string]any{"binary": base64.StdEncoding.EncodeToString(data)}
		}
		// In order: the app's Post keeps no order, and a frame that
		// passes the one before breaks the protocol (SignalR's handshake).
		s.mu.Lock()
		s.inbox = append(s.inbox, msg)
		s.mu.Unlock()
		s.b.Post(s.drain)
	}
}

// write queues a frame for the writer; frames before the socket opens wait.
func (s *socket) write(f wsFrame) {
	s.mu.Lock()
	s.queue = append(s.queue, f)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// writer writes the queued frames in order.
func (s *socket) writer(conn *websocket.Conn) {
	for {
		s.mu.Lock()
		list := s.queue
		s.queue = nil
		s.mu.Unlock()
		for _, f := range list {
			var err error
			switch {
			case f.close:
				// The reader hears the answer and tells how it ended.
				s.mu.Lock()
				s.closing = &f
				s.mu.Unlock()
				err = conn.Close(f.code, string(f.data))
			case f.binary:
				err = conn.Write(s.ctx, websocket.MessageBinary, f.data)
			default:
				err = conn.Write(s.ctx, websocket.MessageText, f.data)
			}
			if err != nil {
				return
			}
		}
		select {
		case <-s.wake:
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *socket) post(v any) {
	if !s.port.Disconnected() {
		s.port.Send(v)
	}
}

func (s *socket) fail() {
	s.post(map[string]any{"failed": true})
	s.post(map[string]any{"closed": 1006, "reason": "", "clean": false})
	s.end(true)
}

// end stops the socket once; tellPort closes the port too.
func (s *socket) end(tellPort bool) {
	if s.ended {
		return
	}
	s.ended = true
	go func() {
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn != nil {
			_ = conn.Close(websocket.StatusGoingAway, "")
		}
		s.cancel()
	}()
	if tellPort {
		s.port.Disconnect()
	}
}

// drain posts the frames read so far, in the order they came.
func (s *socket) drain() {
	s.mu.Lock()
	in := s.inbox
	s.inbox = nil
	s.mu.Unlock()
	for _, msg := range in {
		if !s.ended {
			s.post(msg)
		}
	}
}
