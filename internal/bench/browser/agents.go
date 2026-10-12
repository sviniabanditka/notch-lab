package browser

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo"

	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

// What the MCP tools of the domain `browser` use (04-browser.md, "MCP
// tools"). Main goroutine only, unless a function says otherwise.

// AgentWorld is the content world of the agents' scripts and refs: made at
// launch, never written to a page (R-WEB-236).
var AgentWorld = randName("a")

// The agents' scripts, from Search's AgentPage (R-WEB-200, R-WEB-201).
var (
	//go:embed js/agent/snapshot.js
	SnapshotJS string
	//go:embed js/agent/locate.js
	LocateJS string
	//go:embed js/agent/empty.js
	EmptyJS string
	//go:embed js/agent/choose.js
	ChooseJS string
	//go:embed js/agent/scroll.js
	ScrollJS string
	//go:embed js/agent/found.js
	FoundJS string
)

// TabInfo is a browser Tab for browser.tabs.list.
type TabInfo struct {
	ID       string `json:"id"`
	Bench    string `json:"bench"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Selected bool   `json:"selected"`
	Asleep   bool   `json:"asleep"`
}

// AgentsMayUse reports whether agents see a Bench's Tabs: an isolated
// Profile hides them unless its setting allows (R-WEB-210, R-MCP-033).
func AgentsMayUse(bench *model.Bench) bool {
	if bench == nil || bench.Type != model.Browser {
		return false
	}
	var s struct {
		Isolated bool `json:"isolated"`
		Agents   bool `json:"agentsMayUse"`
	}
	_ = json.Unmarshal([]byte(bench.Settings), &s)
	return !s.Isolated || s.Agents
}

// Describe is a Tab's TabInfo.
func (b *Bench) Describe(t *model.Tab) TabInfo {
	x := b.tabs[t]
	return TabInfo{ID: t.ID, Bench: t.Bench.ID, Title: t.DisplayTitle(), URL: readData(t).URL,
		Selected: t.Bench.FocusedTab() == t, Asleep: x == nil || x.wv == nil}
}

// Open makes a Tab in a Bench with an address, awake, and shows it when asked.
func (b *Bench) Open(bench *model.Bench, u string, owner string) *model.Tab {
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: Host(u), Data: "{}", Owner: owner}
	if u == "" {
		t.Title = "New Tab"
	}
	writeData(t, data{URL: u})
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	if u != "" {
		b.wake(b.live(t), 0)
	}
	return t
}

// Awake wakes a Tab's Web view and returns it, or an error.
func (b *Bench) Awake(t *model.Tab) (*mygo.WebView, error) {
	x := b.live(t)
	if x.wv == nil {
		if readData(t).URL == "" {
			return nil, errors.New("the Tab shows the new tab page; navigate first")
		}
		b.wake(x, 0)
	}
	if x.wv == nil {
		return nil, errors.New("the Web view could not start")
	}
	return x.wv, nil
}

// Navigate loads an address in a Tab.
func (b *Bench) Navigate(t *model.Tab, u string) {
	x := b.live(t)
	b.load(x, u)
	if x.wv != nil && !strings.HasPrefix(u, "http") {
		x.loading = true
	}
}

// Loading reports a Tab that loads now.
func (b *Bench) Loading(t *model.Tab) bool {
	x := b.tabs[t]
	return x != nil && x.loading
}

// Entries are a Tab's Probe entries after a time, of a kind ("console"
// takes console and error, "request" the network).
func (b *Bench) Entries(t *model.Tab, kind string, since time.Time, filter string) []*Entry {
	x := b.live(t)
	var out []*Entry
	for _, e := range x.log.Entries {
		if e.Time.Before(since) {
			continue
		}
		switch kind {
		case "console":
			if e.Kind != "console" && e.Kind != "error" {
				continue
			}
		case "request":
			if e.Kind != "request" || filter != "" && !strings.Contains(e.URL, filter) {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// Downloads is the downloads list.
func (b *Bench) Downloads() []*DownloadItem { return b.Loads.Items }

// KeyFor names a key: Enter, Tab, Escape, ArrowUp…, a letter; it returns
// the characters and the key code AppKit gives it.
func KeyFor(name string) (string, uint16, bool) {
	if k, ok := namedKeys[strings.ToLower(name)]; ok {
		return k.chars, k.code, true
	}
	r := []rune(name)
	if len(r) != 1 {
		return "", 0, false
	}
	return name, KeyCode(r[0]), true
}

type keyDef struct {
	code  uint16
	chars string
}

var namedKeys = map[string]keyDef{
	"enter": {36, "\r"}, "return": {36, "\r"}, "tab": {48, "\t"}, "escape": {53, "\x1b"}, "esc": {53, "\x1b"},
	"backspace": {51, "\x7f"}, "delete": {117, ""}, "space": {49, " "},
	"arrowup": {126, ""}, "arrowdown": {125, ""}, "arrowleft": {123, ""}, "arrowright": {124, ""},
	"up": {126, ""}, "down": {125, ""}, "left": {123, ""}, "right": {124, ""},
	"home": {115, ""}, "end": {119, ""}, "pageup": {116, ""}, "pagedown": {121, ""},
}

// ansi are the key codes of the US layout's letters and digits.
var ansi = map[rune]uint16{
	'a': 0, 's': 1, 'd': 2, 'f': 3, 'h': 4, 'g': 5, 'z': 6, 'x': 7, 'c': 8, 'v': 9, 'b': 11, 'q': 12, 'w': 13, 'e': 14,
	'r': 15, 'y': 16, 't': 17, '1': 18, '2': 19, '3': 20, '4': 21, '6': 22, '5': 23, '=': 24, '9': 25, '7': 26, '-': 27,
	'8': 28, '0': 29, ']': 30, 'o': 31, 'u': 32, '[': 33, 'i': 34, 'p': 35, 'l': 37, 'j': 38, '\'': 39, 'k': 40, ';': 41,
	'\\': 42, ',': 43, '/': 44, 'n': 45, 'm': 46, '.': 47, '`': 50, ' ': 49,
}

// KeyCode is a character's key code on a US layout, 0 for others: the
// characters carry the text.
func KeyCode(r rune) uint16 { return ansi[unicode.ToLower(r)] }

// Modifiers reads "shift", "cmd", "alt", "ctrl" into NSEvent flags.
func Modifiers(names []string) uint {
	var f uint
	for _, n := range names {
		switch strings.ToLower(n) {
		case "shift":
			f |= webkit.Shift
		case "cmd", "command", "meta":
			f |= webkit.Command
		case "alt", "opt", "option":
			f |= webkit.Option
		case "ctrl", "control":
			f |= webkit.Control
		}
	}
	return f
}
