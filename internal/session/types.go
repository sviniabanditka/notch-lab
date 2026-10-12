// Package session holds the agent and pane types the chat and task views
// ported from herdr-go (MIT) speak. notch-lab fills them from its own
// terminal Sessions (docs/spec/12-agents.md); there is no herdr here.
package session

// AgentInfo is what herdr knows of an agent beyond the snapshot.
type AgentInfo struct {
	// Kind is the agent herdr detected, as "claude"; empty for none.
	Kind string
	// SessionID is the agent's own session id, known only when its herdr
	// integration reported it.
	SessionID string
	Folder    string
	Status    AgentStatus
	// ConfigDir is the Agent's config folder, where its transcripts are
	// (notch-lab).
	ConfigDir string
}

type AgentStatus string

const (
	AgentIdle    AgentStatus = "idle"
	AgentWorking AgentStatus = "working"
	AgentBlocked AgentStatus = "blocked"
	AgentDone    AgentStatus = "done"
	AgentUnknown AgentStatus = "unknown"
)

type State struct {
	BootID             string
	FocusedWorkspaceID string
	FocusedTabID       string
	FocusedPaneID      string
	Workspaces         []Workspace
	Tabs               []Tab
	Panes              []Pane
	Agents             []Agent
	Surface            Surface
	LastError          string
	Closed             bool
}

type Workspace struct {
	ID, Label   string
	Number      int
	Focused     bool
	ActiveTabID string
	AgentStatus AgentStatus
}

type Tab struct {
	ID, WorkspaceID, Label string
	Number                 int
	Focused                bool
	// Zoomed is set while one pane of the tab fills it.
	Zoomed      bool
	AgentStatus AgentStatus
}

type Pane struct {
	ID, WorkspaceID, TabID, Label string
	// Cwd is the shell's working directory as herdr last reported it, in a
	// snapshot or asked after Enter; empty when herdr does not know it.
	Cwd string
	// ForegroundCwd is the working directory of the process group in the
	// pane's foreground (the shell at its prompt, or what it runs), read
	// as herdr renders; empty when herdr cannot read it.
	ForegroundCwd string
	Focused       bool
}

// Folder is where the pane is working: its foreground's folder, else
// the shell's.
func (p Pane) Folder() string {
	if p.ForegroundCwd != "" {
		return p.ForegroundCwd
	}
	return p.Cwd
}

type Agent struct {
	PaneID, WorkspaceID, TabID, Name string
	// Kind is the agent herdr detected, as "claude", whatever Name shows.
	Kind   string
	Status AgentStatus
}

// Surface is the focused tab's composed cell grid, sized for this client.
type Surface struct {
	Revision   uint64
	Cols, Rows int
	Lines      [][]Cell
	Cursor     Cursor
	Panes      []SurfacePane
	Splits     []Split
}

type Cell struct {
	Text   string // empty for the tail of a wide character
	FG, BG Color
	Attrs  Attrs
}

type Attrs uint16

const (
	Bold Attrs = 1 << iota
	Dim
	Italic
	Underline
	SlowBlink
	RapidBlink
	Inverse
	Hidden
	Strike
)

type ColorKind uint8

const (
	ColorDefault ColorKind = iota
	ColorIndexed
	ColorRGB
)

// Color is a terminal color; Index is the palette index (0–15 for named).
type Color struct {
	Kind    ColorKind
	Index   uint8
	R, G, B uint8
}

type Cursor struct {
	X, Y    int
	Visible bool
}

type Rect struct{ X, Y, W, H int }

type SurfacePane struct {
	ID             string
	Rect, Inner    Rect
	Focused        bool
	MouseReporting bool
	AltScreen      bool
	Scroll         ScrollMetrics
}

type ScrollMetrics struct {
	OffsetFromBottom, MaxOffsetFromBottom, ViewportRows int
}

type Split struct {
	// SideBySide splits divide along x, so their divider moves horizontally.
	SideBySide bool
	Pos        int
	Area       Rect
	HitRect    Rect
	Path       []bool
}

type Command interface{ command() }

type KeyCode uint8

// Values match the wire's ClientKeyCode tags.
const (
	KeyBackspace KeyCode = iota
	KeyEnter
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyTab
	KeyBackTab
	KeyDelete
	KeyInsert
	KeyEsc
	KeyChar
	KeyF
)

type Mods uint8

const (
	ModShift Mods = 1 << iota
	ModCtrl
	ModAlt
)

// Key sends a non-text key (or a Ctrl/Alt combination) to a pane.
type Key struct {
	PaneID string
	Code   KeyCode
	Char   rune  // for KeyChar
	F      uint8 // for KeyF
	Mods   Mods
}

// Text sends typed or composed text to a pane.
type Text struct{ PaneID, Text string }

type Paste struct{ PaneID, Text string }

// Wheel scrolls a pane at a cell of its inner rect: herdr moves its
// scrollback, or reports the wheel to an app that takes the mouse.
type Wheel struct {
	PaneID   string
	Up       bool
	Lines    int
	Col, Row int
}

// Resize sets this client's surface size, in cells and cell pixels.
type Resize struct{ Cols, Rows, CellWidthPx, CellHeightPx int }

type WindowFocus struct{ Focused bool }

// HostTheme tells herdr the colors the panes show in, which apps in them
// query (OSC 10, 11 and 4, the color-scheme report): the default
// foreground and background, the 16 named colors and the appearance.
type HostTheme struct {
	Foreground, Background RGB
	ANSI                   [16]RGB
	Dark                   bool
}

type RGB struct{ R, G, B uint8 }

type Direction string

const (
	Left  Direction = "left"
	Right Direction = "right"
	Up    Direction = "up"
	Down  Direction = "down"
)

// SplitPane splits a pane to the Right or Down and focuses the new pane,
// whose shell starts in Cwd, or where herdr picks when it is empty.
type SplitPane struct {
	PaneID    string
	Direction Direction
	Cwd       string
}

type FocusDirection struct{ Direction Direction }

type FocusPane struct{ PaneID string }

type FocusTab struct{ TabID string }

// CreateTab opens a tab in a workspace and focuses it.
type CreateTab struct{ WorkspaceID string }

type CloseTab struct{ TabID string }

type ClosePane struct{ PaneID string }

// SetSplitRatio moves a split's divider; herdr accepts 0.1–0.9.
type SetSplitRatio struct {
	TabID string
	Path  []bool
	Ratio float32
}

// ZoomPane toggles whether a pane fills its tab.
type ZoomPane struct{ PaneID string }

// CreateWorkspace opens a workspace in Cwd and focuses it. Without a Cwd
// herdr picks the folder as its own new workspace does: by its
// terminal.new_cwd setting, the focused pane's folder by default.
type CreateWorkspace struct{ Cwd string }

type CloseWorkspace struct{ WorkspaceID string }

type RenameWorkspace struct{ WorkspaceID, Label string }

type RenameTab struct{ TabID, Label string }

// MoveWorkspace reorders a workspace. InsertIndex counts the gaps of the
// current order, the moved one included, as herdr does: moving the
// workspace at i one down is InsertIndex i+2, one up i-1.
type MoveWorkspace struct {
	WorkspaceID string
	InsertIndex int
}

// MoveTab reorders a tab within its workspace, InsertIndex as for
// MoveWorkspace.
type MoveTab struct {
	TabID       string
	InsertIndex int
}

// StartTask opens a tab in a workspace at Cwd labelled Label, or with
// Branch a herdr worktree of it on that new branch from HEAD (Cwd and
// Label then unused), in the background; starts an agent of Kind named AgentName
// there with Args; and sends it Prompt once it is ready. A start that
// blocks (a trust dialog) waits for the user to answer it, two minutes at
// most; Undelivered then gets the agent's pane, off the main thread, and
// the prompt is not sent. Failures surface in State.LastError.
type StartTask struct {
	WorkspaceID, Cwd, Label string
	Branch                  string
	AgentName, Kind         string
	Args                    []string
	Prompt                  string
	Undelivered             func(paneID string)
}

func (Key) command()             {}
func (Text) command()            {}
func (Paste) command()           {}
func (Wheel) command()           {}
func (Resize) command()          {}
func (WindowFocus) command()     {}
func (HostTheme) command()       {}
func (SplitPane) command()       {}
func (FocusDirection) command()  {}
func (FocusPane) command()       {}
func (FocusTab) command()        {}
func (CreateTab) command()       {}
func (CloseTab) command()        {}
func (ClosePane) command()       {}
func (SetSplitRatio) command()   {}
func (ZoomPane) command()        {}
func (CreateWorkspace) command() {}
func (CloseWorkspace) command()  {}
func (RenameWorkspace) command() {}
func (RenameTab) command()       {}
func (MoveWorkspace) command()   {}
func (MoveTab) command()         {}
func (StartTask) command()       {}
