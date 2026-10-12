// Package daemon is notch-labd, which owns every terminal Session, and the
// app's client of it (docs/spec/03-terminal.md, ADR 0003).
//
// A Session is a pseudo-terminal and a headless Ghostty terminal fed with
// its output. Attaching sends the headless terminal's Snapshot, then the
// live output, so a client shows exactly the Screen and Scrollback.
package daemon

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Protocol is the version both sides must share (R-TERM-010).
const Protocol = 1

// maxFrame is the largest frame (R-TERM-020).
const maxFrame = 16 << 20

// maxWrite is the largest session.write (R-TERM-026).
const maxWrite = 64 << 10

// Msg is one frame: a request (ID > 0), its answer (same ID, OK or Err),
// or an event (ID 0).
type Msg struct {
	T   string          `json:"t"`
	ID  int64           `json:"id,omitempty"`
	OK  json.RawMessage `json:"ok,omitempty"`
	Err *Error          `json:"error,omitempty"`

	// hello
	Protocol int    `json:"protocol,omitempty"`
	Client   string `json:"client,omitempty"`
	Daemon   string `json:"daemon,omitempty"`
	PID      int    `json:"pid,omitempty"`

	// session fields, in requests and events
	Session string            `json:"session,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Argv    []string          `json:"argv,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cols    int               `json:"cols,omitempty"`
	Rows    int               `json:"rows,omitempty"`
	B       []byte            `json:"b,omitempty"` // base64 in JSON
	Signal  string            `json:"signal,omitempty"`
	State   *State            `json:"state,omitempty"`
	Code    *int              `json:"code,omitempty"`
}

// Error is a request's failure.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errors a request can return.
var (
	ErrUnknown = &Error{Code: "unknown", Message: "no such session"}
	ErrLimit   = &Error{Code: "limit", Message: "too many sessions"}
	ErrTooBig  = &Error{Code: "too_large", Message: "write over 64 KiB"}
)

// State is what the Daemon knows of a Session, sent in session.state
// events and in session.list.
type State struct {
	ID     string   `json:"id"`
	Cwd    string   `json:"cwd"`
	Argv   []string `json:"argv,omitempty"`
	Cols   int      `json:"cols"`
	Rows   int      `json:"rows"`
	PID    int      `json:"pid"`
	Title  string   `json:"title,omitempty"`
	Fg     string   `json:"fg,omitempty"` // the foreground process name
	Shell  bool     `json:"shell"`        // the foreground process is the login shell
	Exited bool     `json:"exited"`
	Code   int      `json:"code"`
	Agent  *Agent   `json:"agent,omitempty"`
}

// Agent is a coding agent in a Session (12-agents.md).
type Agent struct {
	Kind   string `json:"kind"` // claude, codex
	PID    int    `json:"pid"`
	Status string `json:"status"` // idle, working, blocked, done
}

func writeMsg(w io.Writer, m *Msg) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(data) > maxFrame {
		return fmt.Errorf("frame of %d bytes", len(data))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func readMsg(r io.Reader) (*Msg, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrame {
		return nil, errors.New("frame too large")
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	var m Msg
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
