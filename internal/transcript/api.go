// Package transcript reads an agent's transcript file as a Conversation:
// the items of its active branch, in order, with stable IDs. It owns the
// files (locating, following, earlier reads) and knows nothing of panes,
// turns or drawing.
package transcript

import (
	"sync"
	"time"
)

// Agent names the kind of agent that wrote a transcript.
type Agent string

const Claude Agent = "claude" // Codex joins later behind the same types

// Source names one conversation: herdr's agent kind and session id, and the
// agent's folder, which locates the file.
type Source struct {
	Agent     Agent
	SessionID string
	Folder    string
	// ConfigDir is the Agent's config folder; "" is CLAUDE_CONFIG_DIR or
	// ~/.claude (notch-lab: found by the Agent's sessions/<pid>.json).
	ConfigDir string
}

// Follower reads the tail of a source's transcript and keeps it current
// until Close; subagent files join in the subagents spec. Updates delivers
// a new Conversation after every change; a slow reader gets the latest one,
// never a backlog. LoadEarlier reads the part before the loaded tail and is
// a no-op while an earlier read runs.
type Follower struct {
	updates chan Conversation
	more    chan struct{}
	stop    chan struct{}
	once    sync.Once
}

// Follow starts following src. It is safe to call before the file exists:
// the Conversation is Missing until it does.
func Follow(src Source) *Follower {
	f := &Follower{
		updates: make(chan Conversation, 1),
		more:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
	go f.run(src)
	return f
}

// Updates delivers each new Conversation; it is closed by Close.
func (f *Follower) Updates() <-chan Conversation { return f.updates }

// LoadEarlier asks for the part of the transcript before the loaded tail.
func (f *Follower) LoadEarlier() {
	select {
	case f.more <- struct{}{}:
	default:
	}
}

// Close stops the follower and closes Updates.
func (f *Follower) Close() { f.once.Do(func() { close(f.stop) }) }

// FileState tells whether a Conversation's transcript file is there.
type FileState int

const (
	FileReading FileState = iota
	FileReady
	FileMissing
)

// Conversation is immutable: one state of the active branch. Items
// unchanged since the previous one keep their ID and Rev.
type Conversation struct {
	Items   []Item
	Meta    Meta
	State   FileState
	AtStart bool // Items begin at the start of the conversation
	Earlier bool // LoadEarlier is reading; the chat's only loading flag
}

// Meta is what the transcript says about its session; each part comes from
// the latest record carrying it.
type Meta struct{ AgentVersion, Model, Folder, Branch, Title string }

// Item is one shown piece of the conversation.
type Item struct {
	// ID is stable across updates and earlier reads: message.id and kind
	// for Answer and Thinking, CallID for Tool, the record's uuid otherwise.
	ID string
	// Rev grows when the item changes: a tool's result, a merged block.
	Rev  int
	Kind Kind
	At   time.Time

	Text     string  // Prompt, Answer, Thinking, Notice, Error, Recap
	Images   []Image // Prompt
	Queued   bool    // Prompt typed while the agent worked
	Command  *SlashCommand
	Tool     *Tool
	Duration time.Duration // TurnEnd
}

// Kind is what an Item is.
type Kind int

const (
	KindPrompt Kind = iota
	KindAnswer
	KindThinking
	KindTool
	KindSlashCommand
	KindNotice
	KindInterrupted
	KindCompacted
	KindTurnEnd
	KindError
	KindRecap
)

// SlashCommand is a command the user ran: /skill args, or ! cmd when
// IsShell, whose Name then holds the whole shell command.
type SlashCommand struct {
	Name, Args, Output string
	IsShell            bool
}

// Tool is one tool call with its result. A Tool is never changed in place:
// a new result replaces the pointer.
type Tool struct {
	CallID string
	Name   string // as the agent named it
	Family Family // agent-neutral
	Target string // command, path, pattern, url or description
	State  ToolState

	Result    string // clipped
	Images    []Image
	Diff      []DiffLine
	Todos     []Todo
	Questions []Question // AskUserQuestion's questions; answers are not read yet
	Plan      string     // ExitPlanMode
	Subagent  *Subagent  // Family Agent
}

// Family groups tools by what they do, whatever the agent calls them.
type Family int

const (
	FamilyShell Family = iota
	FamilyRead
	FamilyEdit
	FamilyWrite
	FamilySearch
	FamilyWeb
	FamilyAgent
	FamilyTodo
	FamilyPlan
	FamilyQuestion
	FamilySkill
	FamilyOther
)

// ToolState is how a call stands. A denied call and an interrupted one are
// one state: the transcript does not tell them apart.
type ToolState int

const (
	ToolRunning ToolState = iota
	ToolDone
	ToolFailed
	ToolInterrupted
)

// Subagent is an Agent call's own run. Steps are the subagent's items, last
// 50; nested subagents sit on their Tool step.
type Subagent struct {
	Type, Description string
	Steps             []Item
}

// Image is an attached picture as the transcript carries it.
type Image struct {
	MediaType string
	Data      []byte
}

// Todo is one entry of a TodoWrite list.
type Todo struct{ Text, Status string }

// Question is one AskUserQuestion question.
type Question struct {
	Text    string
	Options []string
	Answer  string
}

// DiffOp marks a diff line: a hunk header, added, removed or unchanged.
type DiffOp uint8

const (
	DiffHunk DiffOp = iota
	DiffAdd
	DiffDel
	DiffSame
)

// DiffLine is one line of an Edit's or Write's change.
type DiffLine struct {
	Op   DiffOp
	Text string
}
