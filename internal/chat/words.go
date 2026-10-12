package chat

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

// action is what a tool call does, for the sentence a run of calls folds
// to; t3code's work-log table (docs/research/t3code-chat-rendering.md §3).
type action uint8

const (
	actCommand action = iota
	actRead
	actEdit
	actCodeSearch
	actWeb
	actOther
	actions
)

func actionOf(f transcript.Family) action {
	switch f {
	case transcript.FamilyShell:
		return actCommand
	case transcript.FamilyRead:
		return actRead
	case transcript.FamilyEdit, transcript.FamilyWrite:
		return actEdit
	case transcript.FamilySearch:
		return actCodeSearch
	case transcript.FamilyWeb:
		return actWeb
	}
	return actOther
}

// phrases word a count of each action, one and many.
var phrases = [actions][2]string{
	actCommand:    {"ran %d command", "ran %d commands"},
	actRead:       {"read %d file", "read %d files"},
	actEdit:       {"edited %d file", "edited %d files"},
	actCodeSearch: {"searched code %d time", "searched code %d times"},
	actWeb:        {"searched the web %d time", "searched the web %d times"},
	actOther:      {"used %d tool", "used %d tools"},
}

// maxParts is how many actions a sentence names before the rest go as
// "and N other actions".
const maxParts = 3

// label words a run of calls: a lone call by its own label, a run as a
// sentence, "Ran 3 commands, read 2 files, edited 1 file". verb is
// plain, target code; a call still running is "Running".
func label(calls []*transcript.Tool, folder string) (verb, target string) {
	for i := len(calls) - 1; i >= 0; i-- {
		if c := calls[i]; c.State == transcript.ToolRunning {
			return "Running", c.Name
		}
	}
	if len(calls) == 1 {
		return callLabel(calls[0], folder)
	}
	var counts [actions]int
	for _, c := range calls {
		counts[actionOf(c.Family)]++
	}
	var parts []string
	rest := 0
	for a, n := range counts {
		switch {
		case n == 0:
		case len(parts) < maxParts:
			parts = append(parts, fmt.Sprintf(phrases[a][min(n-1, 1)], n))
		default:
			rest += n
		}
	}
	s := strings.Join(parts, ", ")
	if rest == 1 {
		s += " and 1 other action"
	} else if rest > 1 {
		s += fmt.Sprintf(" and %d other actions", rest)
	}
	return strings.ToUpper(s[:1]) + s[1:], ""
}

// callLabel words one call: its verb and what it acted on.
func callLabel(c *transcript.Tool, folder string) (verb, target string) {
	t := firstLine(c.Target)
	switch c.Family {
	case transcript.FamilyShell:
		return "Ran", t
	case transcript.FamilyRead:
		return "Read", shortPath(t, folder)
	case transcript.FamilyEdit, transcript.FamilyWrite:
		if c.Family == transcript.FamilyWrite {
			return "Wrote", shortPath(t, folder)
		}
		return "Edited", shortPath(t, folder)
	case transcript.FamilySearch:
		return "Searched for", t
	case transcript.FamilyWeb:
		if c.Name == "WebFetch" {
			return "Fetched", t
		}
		return "Searched the web for", t
	case transcript.FamilyAgent:
		return "Ran agent", t
	case transcript.FamilySkill:
		return "Used skill", t
	}
	return "Used " + c.Name, t
}

// failures counts the calls that failed.
func failures(calls []*transcript.Tool) int {
	n := 0
	for _, c := range calls {
		if c.State == transcript.ToolFailed {
			n++
		}
	}
	return n
}

func shortPath(p, folder string) string {
	if folder != "" {
		if rel, err := filepath.Rel(folder, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return p
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// took words a turn's length as "2m 14s".
func took(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	switch {
	case s < 1:
		return "1s"
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm %ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh %dm", s/3600, s%3600/60)
}
