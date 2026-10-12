package browser

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The Probe (R-WEB-190 … R-WEB-196) is Search's AgentProbe scripts (MIT):
// a watcher in the page's world and a listener in a private content world.

//go:embed js/watcher.js
var watcherJS string

//go:embed js/listener.js
var listenerJS string

//go:embed js/secrets.js
var secretsJS string

// SecretPattern is what counts as a secret (R-WEB-192).
const SecretPattern = `^(authorization|proxy-authorization|cookie|set-cookie|auth|sid|key|code|sig)$|token|secret|passw|pwd|passcode|otp|csrf|xsrf|api[-_]?key|apikey|credential|session[-_]?id|signature|private[-_]?key|security[-_]?token`

// ProbeWorld is the listener's content world; ProbeHandler its message
// handler. Both are made at launch and never written to a page (R-WEB-236).
var ProbeWorld, ProbeHandler, probeEvent = randName("w"), randName("h"), randName("e")

func randName(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

// WatcherScript and ListenerScript are the two user scripts.
func WatcherScript() string {
	se := strings.NewReplacer("%PATTERN%", SecretPattern, "%MASK%", "•••").Replace(secretsJS)
	return strings.NewReplacer("%EVENT%", probeEvent, "%SECRETS%", se, "%WORKERS%", "false").Replace(watcherJS)
}

func ListenerScript() string {
	return strings.NewReplacer("%EVENT%", probeEvent, "%HANDLER%", ProbeHandler).Replace(listenerJS)
}

// Entry is one thing the Probe saw, the fields the app knows (R-WEB-191).
type Entry struct {
	Time     time.Time         `json:"time"`
	Kind     string            `json:"kind"` // console, error, request
	Level    string            `json:"level,omitempty"`
	Text     string            `json:"text,omitempty"`
	URL      string            `json:"url,omitempty"`
	Method   string            `json:"method,omitempty"`
	Status   int               `json:"status,omitempty"`
	Failed   string            `json:"failed,omitempty"`
	Ms       int               `json:"duration,omitempty"`
	Via      string            `json:"via,omitempty"`
	ReqHead  map[string]string `json:"requestHeaders,omitempty"`
	RespHead map[string]string `json:"responseHeaders,omitempty"`
	ReqBody  string            `json:"requestBody,omitempty"`
	Body     string            `json:"body,omitempty"`
	id       string
}

// Log is a Tab's ring buffer of 2 000 entries (R-WEB-193).
type Log struct {
	Entries []*Entry
	byID    map[string]*Entry
}

const maxEntries = 2000

func cut(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func heads(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, x := range m {
		if len(out) == 32 {
			break
		}
		out[k] = cut(fmt.Sprint(x), 1024)
	}
	return out
}

// Add takes one batch the listener posted.
func (l *Log) Add(batch string) {
	var items []map[string]any
	if json.Unmarshal([]byte(batch), &items) != nil {
		return
	}
	if l.byID == nil {
		l.byID = map[string]*Entry{}
	}
	str := func(m map[string]any, k string) string {
		if v, ok := m[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	num := func(m map[string]any, k string) int {
		if f, ok := m[k].(float64); ok {
			return int(f)
		}
		return 0
	}
	for _, m := range items {
		switch str(m, "kind") {
		case "console", "error":
			l.push(&Entry{Time: time.Now(), Kind: str(m, "kind"), Level: str(m, "level"), Text: cut(str(m, "text"), 4096), URL: str(m, "source")})
		case "net":
			id := str(m, "id")
			e := l.byID[id]
			switch str(m, "phase") {
			case "start", "resource":
				e = &Entry{Time: time.Now(), Kind: "request", id: id, URL: str(m, "url"), Method: str(m, "method"), Via: str(m, "via"),
					ReqHead: heads(m["requestHeaders"]), ReqBody: cut(str(m, "requestBody"), 64<<10), Ms: num(m, "ms"), Failed: str(m, "failed")}
				if e.Method == "" {
					e.Method = "GET"
				}
				l.byID[id] = e
				l.push(e)
			case "end":
				if e != nil {
					e.Status, e.Ms, e.Failed = num(m, "status"), num(m, "ms"), str(m, "failed")
					e.RespHead = heads(m["responseHeaders"])
				}
			case "body":
				if e != nil {
					e.Body = cut(str(m, "body"), 64<<10)
				}
			}
		}
	}
}

func (l *Log) push(e *Entry) {
	l.Entries = append(l.Entries, e)
	if len(l.Entries) > maxEntries {
		old := l.Entries[0]
		delete(l.byID, old.id)
		l.Entries = l.Entries[1:]
	}
}

// Clear empties the buffer.
func (l *Log) Clear() { l.Entries, l.byID = nil, nil }
