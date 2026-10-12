// Package settings reads and writes settings.json (docs/spec/02-shell.md
// R-SHELL-162 … R-SHELL-165). Secrets never go here.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Settings is the schema of R-SHELL-163.
type Settings struct {
	Version    int        `json:"version"`
	General    General    `json:"general"`
	Notch      Notch      `json:"notch"`
	Appearance Appearance `json:"appearance"`
	Sidebar    Sidebar    `json:"sidebar"`
	Window     Window     `json:"window"`
	Terminal   Terminal   `json:"terminal"`
	Browser    Browser    `json:"browser"`
	Editor     Editor     `json:"editor"`
	MCP        MCP        `json:"mcp"`
	Advanced   Advanced   `json:"advanced"`
}

type General struct {
	OpenMainWindowAtStart bool   `json:"openMainWindowAtStart"`
	ConfirmCloseUnsaved   bool   `json:"confirmCloseUnsaved"`
	StartLab              string `json:"startLab"` // "last" or a Lab id
}
type Notch struct {
	Size         string `json:"size"` // S, M, L
	HoverShows   bool   `json:"hoverShows"`
	HoverDelayMs int    `json:"hoverDelayMs"`
}
type Appearance struct {
	Theme         string  `json:"theme"` // system, dark, light
	UIFontSize    float64 `json:"uiFontSize"`
	ShowStatusBar bool    `json:"showStatusBar"`
}
type Sidebar struct {
	Width float64 `json:"width"`
}
type Window struct {
	Main struct {
		Open bool `json:"open"`
	} `json:"main"`
}
type Terminal struct {
	FontFamily      string  `json:"fontFamily"`
	FontSize        float64 `json:"fontSize"`
	LineHeight      float64 `json:"lineHeight"`
	ScrollbackLines int     `json:"scrollbackLines"`
	Cursor          string  `json:"cursor"`
	Shell           string  `json:"shell"`
	Bell            string  `json:"bell"`
	CopyOnSelect    bool    `json:"copyOnSelect"`
	AltIsMeta       bool    `json:"altIsMeta"`
	ConfirmClose    bool    `json:"confirmClose"`
	OSCTitle        bool    `json:"oscTitle"`
	BellUnread      bool    `json:"bellUnread"`
	LinksExternal   bool    `json:"linksExternal"`
}
type Browser struct {
	SearchEngine   string `json:"searchEngine"`
	HomePage       string `json:"homePage"`
	DownloadFolder string `json:"downloadFolder"`
	AskWhereToSave bool   `json:"askWhereToSave"`
	BlockPopups    bool   `json:"blockPopups"`
	DefaultProfile string `json:"defaultProfile"`
	Inspectable    bool   `json:"inspectable"`
	BookmarksBar   bool   `json:"bookmarksBar"`
	// Permissions, Zoom and Schemes are remembered per origin (R-WEB-051,
	// R-WEB-091, R-WEB-037).
	Permissions map[string]map[string]string `json:"permissions"`
	Zoom        map[string]float64           `json:"zoom"`
	Schemes     map[string]map[string]bool   `json:"schemes"`
}
type Editor struct {
	FontFamily        string  `json:"fontFamily"`
	FontSize          float64 `json:"fontSize"`
	TabWidth          int     `json:"tabWidth"`
	InsertSpaces      bool    `json:"insertSpaces"`
	Autosave          string  `json:"autosave"`
	TrimTrailingSpace bool    `json:"trimTrailingSpace"`
	LineNumbers       bool    `json:"lineNumbers"`
	Wrap              bool    `json:"wrap"`
	External          string  `json:"external"`
	HideDots          bool    `json:"hideDots"`
}
type MCP struct {
	Access       string            `json:"access"`
	Domains      map[string]string `json:"domains"`
	ToastOnWrite bool              `json:"toastOnWrite"`
	// CloseOwnTabs closes a Client's own Tabs when it leaves (R-MCP-030).
	CloseOwnTabs bool `json:"closeOwnTabs"`
	// FullAgreed is the answer to the first Full sheet (R-MCP-043).
	FullAgreed bool `json:"fullAgreed"`
}
type Advanced struct {
	LogLevel string `json:"logLevel"`
}

// Defaults returns the defaults of R-SHELL-163.
func Defaults() Settings {
	var s Settings
	s.Version = 1
	s.General = General{OpenMainWindowAtStart: true, ConfirmCloseUnsaved: true, StartLab: "last"}
	s.Notch = Notch{Size: "M", HoverShows: true, HoverDelayMs: 300}
	s.Appearance = Appearance{Theme: "system", UIFontSize: 13, ShowStatusBar: true}
	s.Sidebar = Sidebar{Width: 240}
	s.Window.Main.Open = true
	s.Terminal = Terminal{FontFamily: "JetBrains Mono, SF Mono, Menlo", FontSize: 13, LineHeight: 1.2, ScrollbackLines: 10000, Cursor: "block", Bell: "none",
		AltIsMeta: true, ConfirmClose: true, OSCTitle: true}
	s.Browser = Browser{SearchEngine: "https://duckduckgo.com/?q=%s", HomePage: "about:blank", DownloadFolder: "~/Downloads", BlockPopups: true, DefaultProfile: "shared"}
	s.Editor = Editor{FontFamily: "Menlo", FontSize: 13, TabWidth: 4, InsertSpaces: true, Autosave: "off", LineNumbers: true,
		External: "cursor --goto {path}:{line}:{col}", HideDots: true}
	s.Browser.Inspectable = true
	s.Browser.Permissions, s.Browser.Zoom, s.Browser.Schemes = map[string]map[string]string{}, map[string]float64{}, map[string]map[string]bool{}
	s.MCP = MCP{Access: "read", Domains: map[string]string{"ssh": "off"}, ToastOnWrite: true, CloseOwnTabs: true}
	s.Advanced = Advanced{LogLevel: "info"}
	return s
}

// Store owns the file. Its value belongs to the main goroutine.
type Store struct {
	S    Settings
	path string
	// extra holds top-level keys not in the schema, written back as they
	// are (R-SHELL-164).
	extra   map[string]json.RawMessage
	mu      sync.Mutex
	pending bool
}

// Load reads path. A missing file gives the defaults. A section of the
// wrong shape keeps its defaults and is reported in problems; a file that
// is not JSON gives the defaults and err (R-SHELL-162, R-SHELL-164).
// ponytail: a bad value resets its whole section, not only that key.
func Load(path string) (st *Store, problems []string, err error) {
	st = &Store{S: Defaults(), path: path, extra: map[string]json.RawMessage{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil, nil
	}
	if err != nil {
		return st, nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return st, nil, fmt.Errorf("settings.json is not JSON: %w", err)
	}
	sections := map[string]any{
		"version": &st.S.Version, "general": &st.S.General, "notch": &st.S.Notch,
		"appearance": &st.S.Appearance, "sidebar": &st.S.Sidebar, "window": &st.S.Window,
		"terminal": &st.S.Terminal, "browser": &st.S.Browser, "editor": &st.S.Editor,
		"mcp": &st.S.MCP, "advanced": &st.S.Advanced,
	}
	def := Defaults()
	defaults := map[string]any{
		"version": def.Version, "general": def.General, "notch": def.Notch,
		"appearance": def.Appearance, "sidebar": def.Sidebar, "window": def.Window,
		"terminal": def.Terminal, "browser": def.Browser, "editor": def.Editor,
		"mcp": def.MCP, "advanced": def.Advanced,
	}
	for k, raw := range top {
		dst, ok := sections[k]
		if !ok {
			st.extra[k] = raw
			continue
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			problems = append(problems, fmt.Sprintf("settings.json: %s: %v; defaults used", k, err))
			b, _ := json.Marshal(defaults[k])
			_ = json.Unmarshal(b, dst)
		}
	}
	if st.S.Browser.Permissions == nil {
		st.S.Browser.Permissions = map[string]map[string]string{}
	}
	if st.S.Browser.Zoom == nil {
		st.S.Browser.Zoom = map[string]float64{}
	}
	if st.S.Browser.Schemes == nil {
		st.S.Browser.Schemes = map[string]map[string]bool{}
	}
	if st.S.MCP.Domains == nil {
		st.S.MCP.Domains = map[string]string{}
	}
	st.clamp()
	return st, problems, nil
}

// clamp keeps numbers in the ranges Settings offers.
func (st *Store) clamp() {
	s := &st.S
	s.Appearance.UIFontSize = min(16, max(12, s.Appearance.UIFontSize))
	s.Sidebar.Width = min(480, max(160, s.Sidebar.Width))
	s.Terminal.ScrollbackLines = min(100000, max(1000, s.Terminal.ScrollbackLines))
	s.Terminal.FontSize = min(32, max(8, s.Terminal.FontSize))
}

// Changed writes the file 100 ms after the first of a batch of changes
// (R-SHELL-162). onErr hears a failed write.
func (st *Store) Changed(onErr func(error)) {
	st.clamp()
	if st.pending {
		return
	}
	st.pending = true
	data, err := st.encode()
	time.AfterFunc(100*time.Millisecond, func() {
		st.mu.Lock()
		defer st.mu.Unlock()
		st.pending = false
		if err == nil {
			err = st.writeLocked(data)
		}
		if err != nil && onErr != nil {
			onErr(err)
		}
	})
}

// Flush writes the file now.
func (st *Store) Flush() error {
	data, err := st.encode()
	if err != nil {
		return err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.writeLocked(data)
}

func (st *Store) encode() ([]byte, error) {
	known, err := json.Marshal(st.S)
	if err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	_ = json.Unmarshal(known, &top)
	for k, v := range st.extra {
		top[k] = v
	}
	return json.MarshalIndent(top, "", "  ")
}

func (st *Store) writeLocked(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(st.path), 0o700); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}
