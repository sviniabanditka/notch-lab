package transcript

import (
	"encoding/json"
	"strings"
	"time"
)

// Claude Code's transcript is not a public format: every field is optional
// here, and a record of a type not handled is skipped.

type rawRecord struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	UUID             string          `json:"uuid"`
	ParentUUID       string          `json:"parentUuid"`
	Timestamp        string          `json:"timestamp"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	IsAPIError       bool            `json:"isApiErrorMessage"`
	Error            json.RawMessage `json:"error"`
	PromptSource     string          `json:"promptSource"`
	Message          *rawMessage     `json:"message"`
	ToolUseResult    json.RawMessage `json:"toolUseResult"`
	DurationMs       int64           `json:"durationMs"`
	Content          json.RawMessage `json:"content"`
	Attachment       *rawAttachment  `json:"attachment"`
	AITitle          string          `json:"aiTitle"`
	Version          string          `json:"version"`
	Cwd              string          `json:"cwd"`
	GitBranch        string          `json:"gitBranch"`
}

type rawMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type rawAttachment struct {
	Type        string          `json:"type"`
	CommandMode string          `json:"commandMode"`
	Prompt      json.RawMessage `json:"prompt"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Source    *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source"`
}

// rec is one record as the reducer takes it: what it adds to the
// conversation, decoded once. Items carry their IDs; the content of an
// Answer or Thinking item is one block of its message.
type rec struct {
	uuid, parent string
	at           time.Time
	// rewinds is set on a prompt or slash command: a row like that whose
	// parent already led to another prompt starts over from its parent.
	rewinds bool
	items   []Item
	// task is a background-task notice's task id, one notice per task.
	task string
	// results are the tool results the record carries.
	results []*result
	// isOutput is set for a command's output, which belongs to the command
	// its parent names.
	isOutput bool
	output   string
	// interrupt is set on the marker that ends running calls.
	interrupt bool
	meta      header
}

// header is what records say about the session.
type header struct {
	version, model, cwd, branch, title string
}

// result is a tool result: its call, its text and what the call changed.
type result struct {
	callID      string
	text        string
	isError     bool
	interrupted bool
	images      []Image
	diff        []DiffLine
}

// maxResultText caps what a result keeps: the chat shows excerpts, and a
// long session holds thousands of records.
const maxResultText = 16 << 10

// maxDiffLines caps the lines an Edit's or Write's diff keeps.
const maxDiffLines = 2000

// decodeRecord reads one transcript line; ok is false for a line that is
// not a JSON object.
func decodeRecord(line []byte) (*rec, bool) {
	var r rawRecord
	if json.Unmarshal(line, &r) != nil {
		return nil, false
	}
	at, _ := time.Parse(time.RFC3339Nano, r.Timestamp)
	rc := &rec{uuid: r.UUID, parent: r.ParentUUID, at: at}
	switch r.Type {
	case "user":
		decodeUser(&r, rc)
	case "assistant":
		decodeAssistant(&r, rc)
	case "system":
		decodeSystem(&r, rc)
	case "attachment":
		decodeAttachment(&r, rc)
	case "ai-title":
		rc.meta.title = r.AITitle
	}
	rc.meta.version, rc.meta.cwd, rc.meta.branch = r.Version, r.Cwd, r.GitBranch
	return rc, true
}

func decodeAssistant(r *rawRecord, rc *rec) {
	if r.Message == nil {
		return
	}
	msg := r.Message.ID
	if msg == "" {
		msg = rc.uuid
	}
	if r.Message.Model != "<synthetic>" {
		rc.meta.model = r.Message.Model
	}
	if r.IsAPIError || isSet(r.Error) {
		text := contentText(r.Message.Content)
		if text == "" {
			text = strings.Trim(string(r.Error), `"`)
		}
		rc.items = append(rc.items, Item{ID: rc.uuid, Kind: KindError, Text: text})
		return
	}
	if r.Message.Model == "<synthetic>" {
		return
	}
	for _, b := range blocks(r.Message.Content) {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				rc.items = append(rc.items, Item{ID: msg + "/answer", Kind: KindAnswer, Text: b.Text})
			}
		case "thinking":
			rc.items = append(rc.items, Item{ID: msg + "/thinking", Kind: KindThinking, Text: b.Thinking})
		case "tool_use":
			rc.items = append(rc.items, Item{ID: b.ID, Kind: KindTool, Tool: decodeUse(b)})
		}
	}
}

func decodeSystem(r *rawRecord, rc *rec) {
	switch r.Subtype {
	case "compact_boundary":
		rc.items = append(rc.items, Item{ID: rc.uuid, Kind: KindCompacted})
	case "turn_duration":
		rc.items = append(rc.items, Item{ID: rc.uuid, Kind: KindTurnEnd,
			Duration: time.Duration(r.DurationMs) * time.Millisecond})
	case "away_summary":
		rc.items = append(rc.items, Item{ID: rc.uuid, Kind: KindRecap, Text: recapText(stringOf(r.Content))})
	case "local_command":
		rc.isOutput, rc.output = true, stringOf(r.Content)
	}
}

// decodeAttachment reads the attachments that are shown: a queued command
// is a prompt the user sent while the agent worked, or a background-task
// notice.
func decodeAttachment(r *rawRecord, rc *rec) {
	a := r.Attachment
	if a == nil || a.Type != "queued_command" {
		return
	}
	text := stringOf(a.Prompt)
	switch a.CommandMode {
	case "task-notification":
		rc.task = tag(text, "task-id")
		rc.items = append(rc.items, Item{ID: rc.uuid, Kind: KindNotice, Text: noticeLine(text)})
	case "prompt":
		rc.items = append(rc.items, Item{ID: rc.uuid, Kind: KindPrompt, Text: text, Queued: true})
	}
}

// stringOf reads a JSON string, or "" for anything else.
func stringOf(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// isSet reports whether a raw JSON field carries a value.
func isSet(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null" && string(raw) != `""`
}

// blocks reads message content, a string or a list of blocks.
func blocks(raw json.RawMessage) []rawBlock {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []rawBlock{{Type: "text", Text: s}}
	}
	var bs []rawBlock
	_ = json.Unmarshal(raw, &bs)
	return bs
}

// contentText joins the text blocks of content, a string or a list.
func contentText(raw json.RawMessage) string {
	var parts []string
	for _, b := range blocks(raw) {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// tag is the trimmed text inside the first <name>…</name> of s.
func tag(s, name string) string {
	_, rest, ok := strings.Cut(s, "<"+name+">")
	if !ok {
		return ""
	}
	inner, _, _ := strings.Cut(rest, "</"+name+">")
	return strings.TrimSpace(inner)
}

// clip cuts s to at most n bytes, at a rune boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
