package transcript

import (
	"encoding/json"
	"fmt"
	"strings"
)

// familyOf maps a tool's name to its family; a name it does not know is
// Other.
func familyOf(name string) Family {
	switch name {
	case "Bash", "BashOutput", "KillShell":
		return FamilyShell
	case "Read", "NotebookRead":
		return FamilyRead
	case "Edit", "MultiEdit", "NotebookEdit":
		return FamilyEdit
	case "Write":
		return FamilyWrite
	case "Grep", "Glob", "LS":
		return FamilySearch
	case "WebSearch", "WebFetch":
		return FamilyWeb
	case "Agent", "Task":
		return FamilyAgent
	case "TodoWrite", "TaskCreate", "TaskUpdate":
		return FamilyTodo
	case "ExitPlanMode":
		return FamilyPlan
	case "AskUserQuestion":
		return FamilyQuestion
	case "Skill":
		return FamilySkill
	}
	return FamilyOther
}

// toolInput is the input of the tools the chat words: the fields whose
// first non-empty value is the call's target, and what its families show.
type toolInput struct {
	Command      string `json:"command"`
	Description  string `json:"description"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Pattern      string `json:"pattern"`
	Query        string `json:"query"`
	URL          string `json:"url"`
	Skill        string `json:"skill"`
	Plan         string `json:"plan"`
	Todos        []struct {
		Content string `json:"content"`
		Status  string `json:"status"`
	} `json:"todos"`
	Questions []struct {
		Question string `json:"question"`
		Options  []struct {
			Label string `json:"label"`
		} `json:"options"`
	} `json:"questions"`
}

// decodeUse reads a tool_use block into a running Tool.
func decodeUse(b rawBlock) *Tool {
	var in toolInput
	_ = json.Unmarshal(b.Input, &in)
	t := &Tool{CallID: b.ID, Name: b.Name, Family: familyOf(b.Name), State: ToolRunning}
	switch {
	case in.Command != "":
		t.Target = in.Command
	case in.FilePath != "":
		t.Target = in.FilePath
	case in.NotebookPath != "":
		t.Target = in.NotebookPath
	case in.Pattern != "":
		t.Target = in.Pattern
	case in.Query != "":
		t.Target = in.Query
	case in.URL != "":
		t.Target = in.URL
	case in.Skill != "":
		t.Target = in.Skill
	default:
		t.Target = in.Description
	}
	for _, td := range in.Todos {
		t.Todos = append(t.Todos, Todo{Text: td.Content, Status: td.Status})
	}
	for _, q := range in.Questions {
		qq := Question{Text: q.Question}
		for _, o := range q.Options {
			qq.Options = append(qq.Options, o.Label)
		}
		t.Questions = append(t.Questions, qq)
	}
	t.Plan = in.Plan
	return t
}

// interruptText is the result Claude writes for a call the user declined
// or stopped at a permission dialog.
const interruptText = "The user doesn't want to proceed"

// decodeResult reads a tool_result block. extra is the record's
// toolUseResult, from which an Edit's or Write's patch is read.
func decodeResult(b rawBlock, extra json.RawMessage) *result {
	text := contentText(b.Content)
	res := &result{callID: b.ToolUseID, text: clip(text, maxResultText), isError: b.IsError}
	res.interrupted = b.IsError && strings.HasPrefix(text, interruptText)
	res.images = contentImages(b.Content)
	res.diff = decodePatch(extra)
	return res
}

// contentImages reads the image blocks of a tool result's content.
func contentImages(raw json.RawMessage) []Image {
	var out []Image
	for _, b := range blocks(raw) {
		if b.Type == "image" {
			if img, ok := decodeImage(b); ok {
				out = append(out, img)
			}
		}
	}
	return out
}

type patchResult struct {
	Type            string `json:"type"`
	Content         string `json:"content"`
	StructuredPatch []struct {
		OldStart int      `json:"oldStart"`
		OldLines int      `json:"oldLines"`
		NewStart int      `json:"newStart"`
		NewLines int      `json:"newLines"`
		Lines    []string `json:"lines"`
	} `json:"structuredPatch"`
}

// decodePatch reads a toolUseResult's change as diff lines, at most
// maxDiffLines of them.
func decodePatch(extra json.RawMessage) []DiffLine {
	var p patchResult
	if len(extra) == 0 || extra[0] != '{' || json.Unmarshal(extra, &p) != nil {
		return nil
	}
	var out []DiffLine
	add := func(op DiffOp, text string) {
		if len(out) < maxDiffLines {
			out = append(out, DiffLine{Op: op, Text: text})
		}
	}
	for _, h := range p.StructuredPatch {
		add(DiffHunk, fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines))
		for _, l := range h.Lines {
			op := diffOp(l)
			if op != DiffSame {
				l = l[1:]
			}
			add(op, l)
		}
	}
	if len(p.StructuredPatch) == 0 && p.Type == "create" && p.Content != "" {
		for _, l := range strings.Split(strings.TrimSuffix(p.Content, "\n"), "\n") {
			add(DiffAdd, l)
		}
	}
	return out
}

// diffOp reads the marker a unified diff line starts with.
func diffOp(l string) DiffOp {
	switch {
	case strings.HasPrefix(l, "+"):
		return DiffAdd
	case strings.HasPrefix(l, "-"):
		return DiffDel
	}
	return DiffSame
}
