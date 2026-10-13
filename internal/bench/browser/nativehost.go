package browser

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"syscall"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/extensions"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

// Chrome's native messaging, as Search's ExtensionNative: the apps on
// this Mac that an Extension talks to (a password manager's desktop app).
// An app registers with Chrome by a JSON file in a NativeMessagingHosts
// folder: a name, the program to run, and the Extensions that may run it.
// The app reads the same files, runs the same program with the same
// argument, and speaks the same protocol: each message is a 4-byte
// little-endian length and JSON, over the program's stdin and stdout.

var (
	errNoNative   = errors.New("Access to native messaging requires the nativeMessaging permission.")
	errNoHost     = errors.New("Specified native messaging host not found.")
	errForbidden  = errors.New("Access to the specified native messaging host is forbidden.")
	errHostName   = errors.New("Invalid native messaging host name")
	errHostExited = errors.New("Native host has exited.")
	errTooLong    = errors.New("Message too long for a native host")
)

// hostDirs are where Chromium browsers look, per user and for the whole
// Mac, in Search's order.
func hostDirs(home string) []string {
	support := filepath.Join(home, "Library/Application Support")
	return []string{
		filepath.Join(support, "Google/Chrome/NativeMessagingHosts"),
		filepath.Join(support, "Chromium/NativeMessagingHosts"),
		filepath.Join(support, "Microsoft Edge/NativeMessagingHosts"),
		filepath.Join(support, "BraveSoftware/Brave-Browser/NativeMessagingHosts"),
		filepath.Join(support, "Arc/User Data/NativeMessagingHosts"),
		"/Library/Google/Chrome/NativeMessagingHosts",
		"/Library/Application Support/Chromium/NativeMessagingHosts",
		"/Library/Microsoft/Edge/NativeMessagingHosts",
		// Read last: a host of the same name that Chrome or the system
		// knows comes first.
		filepath.Join(support, "Vivaldi/NativeMessagingHosts"),
		filepath.Join(support, "com.operasoftware.Opera/NativeMessagingHosts"),
	}
}

var hostName = regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)*$`)

// findHost is the program for name: the first manifest found in dirs
// decides. It must list the Extension's origin.
func findHost(dirs []string, name, extID string) (string, error) {
	if !hostName.MatchString(name) {
		return "", errHostName
	}
	origin := "chrome-extension://" + extID + "/"
	for _, dir := range dirs {
		raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			continue
		}
		var m struct {
			Path    *string `json:"path"`
			Allowed []any   `json:"allowed_origins"`
		}
		if json.Unmarshal(raw, &m) != nil || m.Path == nil {
			continue
		}
		if !slices.Contains(m.Allowed, any(origin)) {
			return "", errForbidden
		}
		program := *m.Path
		if !filepath.IsAbs(program) {
			program = filepath.Join(dir, program)
		}
		if st, err := os.Stat(program); err != nil || !st.Mode().IsRegular() || st.Mode()&0o111 == 0 {
			return "", errNoHost
		}
		return program, nil
	}
	return "", errNoHost
}

// frame is a message as a host reads it.
func frame(msg []byte) ([]byte, error) {
	if len(msg) > 1<<20 {
		return nil, errTooLong
	}
	out := binary.LittleEndian.AppendUint32(make([]byte, 0, 4+len(msg)), uint32(len(msg)))
	return append(out, msg...), nil
}

// readFrame reads one message a host wrote.
func readFrame(r io.Reader) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(n[:])
	if size > 64<<20 { // ponytail: no host sends this much; a broken stream ends the host
		return nil, errHostExited
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// hostPipe is one running host program.
type hostPipe struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out io.Reader
}

func startHost(program, origin string) (*hostPipe, error) {
	cmd := exec.Command(program, origin)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &hostPipe{cmd: cmd, in: in, out: out}, nil
}

// write sends one message. A host that is gone fails the write; Go does
// not raise SIGPIPE for a pipe that is not stdout.
func (h *hostPipe) write(msg []byte) error {
	f, err := frame(msg)
	if err != nil {
		return err
	}
	_, err = h.in.Write(f)
	return err
}

// read is the next message that is JSON; errHostExited once the host ends.
func (h *hostPipe) read() ([]byte, error) {
	for {
		msg, err := readFrame(h.out)
		if err != nil {
			return nil, errHostExited
		}
		if json.Valid(msg) {
			return msg, nil
		}
	}
}

func (h *hostPipe) stop() {
	_ = h.in.Close()
	_ = h.cmd.Process.Signal(syscall.SIGTERM)
}

// sendOnce is runtime.sendNativeMessage: run, send one, read one, stop.
func sendOnce(dirs []string, name, extID string, msg []byte) ([]byte, error) {
	program, err := findHost(dirs, name, extID)
	if err != nil {
		return nil, err
	}
	h, err := startHost(program, "chrome-extension://"+extID+"/")
	if err != nil {
		return nil, err
	}
	defer func() {
		h.stop()
		go h.cmd.Wait()
	}()
	if err := h.write(msg); err != nil {
		return nil, err
	}
	return h.read()
}

// asksForNative reports a manifest that asks for "nativeMessaging",
// required or optional. Prepare writes it into every manifest for the
// Shim's line to the app, so what it added there does not count.
func (e *Ext) asksForNative(cx *webkit.Context) bool {
	en := e.M.Get(cx.ID)
	if en == nil {
		return false
	}
	folder := e.M.Folder(en)
	mf := readManifest(folder)
	required := slices.Contains(mf.Permissions, "nativeMessaging") && !slices.Contains(extensions.Added(folder), "nativeMessaging")
	return required || slices.Contains(mf.Optional, "nativeMessaging")
}

func nativeDirs() []string {
	home, _ := os.UserHomeDir()
	return hostDirs(home)
}

// SendNative sends one message to a host on this Mac.
func (e *Ext) SendNative(cx *webkit.Context, app string, msg []byte, done func([]byte, error)) {
	if !e.asksForNative(cx) {
		done(nil, errNoNative)
		return
	}
	go func() {
		reply, err := sendOnce(nativeDirs(), app, cx.ID, msg)
		e.b.Post(func() {
			if err == nil {
				done(reply, nil)
				return
			}
			// An Extension asking an app that is not there, over and over,
			// gets slow answers after 12 in one second.
			key, now := cx.ID+"→"+app, time.Now()
			recent := []time.Time{}
			for _, t := range e.failures[key] {
				if now.Sub(t) < time.Second {
					recent = append(recent, t)
				}
			}
			e.failures[key] = append(recent, now)
			if len(e.failures[key]) > 12 {
				time.AfterFunc(time.Second, func() { e.b.Post(func() { done(nil, err) }) })
				return
			}
			done(nil, err)
		})
	}()
}

// ConnectPort takes a port to an app: the socket, or a host on this Mac
// (runtime.connectNative).
func (e *Ext) ConnectPort(cx *webkit.Context, p *webkit.Port) error {
	name := p.ApplicationID()
	if name == socketApp {
		e.connectSocket(cx, p)
		return nil
	}
	if !e.asksForNative(cx) {
		return errNoNative
	}
	if name == "" {
		return errors.New("No host named")
	}
	program, err := findHost(nativeDirs(), name, cx.ID)
	if err != nil {
		return err
	}
	// A new port is often a worker that starts again; the one before may
	// have left its host running.
	e.stopOrphans()
	h, err := startHost(program, "chrome-extension://"+cx.ID+"/")
	if err != nil {
		return err
	}
	e.hosts[h] = p
	go func() {
		for {
			msg, err := h.read()
			if err != nil {
				break
			}
			e.b.Post(func() { p.Send(json.RawMessage(msg)) })
		}
		_ = h.cmd.Wait()
		e.b.Post(func() {
			delete(e.hosts, h)
			p.Disconnect()
		})
	}()
	beating := false
	p.SetMessageHandler(func(msg []byte) {
		if string(msg) == "null" {
			return
		}
		// The worker's Shim asks whether the port arrived (see the Shim,
		// after its WebSocket): answered here, never passed on.
		var asked map[string]any
		if json.Unmarshal(msg, &asked) == nil && asked != nil {
			if word, ok := asked["__searchNative"]; ok {
				// The Shim's answer to "alive" only keeps its worker.
				if word != "here?" {
					return
				}
				p.Send(map[string]any{"__searchNative": "here"})
				// WebKit unloads a worker that did not post on a port for
				// two minutes; Chrome keeps one with a port to an app. A
				// word now and then, heard only by the Shim, makes the
				// worker answer on the port, which WebKit counts.
				if !beating {
					beating = true
					var beat func()
					beat = func() {
						if p.Disconnected() {
							return
						}
						p.Send(map[string]any{"__searchNative": "alive"})
						time.AfterFunc(25*time.Second, func() { e.b.Post(beat) })
					}
					time.AfterFunc(25*time.Second, func() { e.b.Post(beat) })
				}
				return
			}
		}
		_ = h.write(msg)
	})
	p.SetDisconnectHandler(h.stop)
	return nil
}

// stopOrphans stops the hosts of ports that went. WebKit does not always
// call a port's disconnect handler: an Extension unloaded leaves its
// worker's ports disconnected without a word, and each host would run on.
func (e *Ext) stopOrphans() {
	for h, p := range e.hosts {
		if p.Disconnected() {
			h.stop()
			p.Disconnect()
		}
	}
}
