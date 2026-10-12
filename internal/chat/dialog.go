package chat

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// dialogTail is how many non-blank lines may follow a dialog's options
	// on the screen, as Claude's "Esc to cancel · Tab to amend" footer.
	dialogTail = 3
	// dialogHead caps the lines read above the options for the dialog's
	// heading and description.
	dialogHead = 10
)

// optionLine is one numbered choice of a Claude select: "❯ 1. Yes" when
// selected, "  2. No" otherwise.
var optionLine = regexp.MustCompile(`^(\s*(❯\s*)?)(\d{1,2})\.\s+(\S.*)$`)

// option is a numbered choice of a dialog on the agent's screen.
type option struct {
	n        int
	text     string
	selected bool
}

// dialog is a permission prompt as the screen shows it: its lines above
// the options, and the options. The parse gives the options no meaning.
type dialog struct {
	lines   []string
	options []option
}

// has reports whether the dialog still offers o: its number with its text.
func (d dialog) has(o option) bool {
	for _, x := range d.options {
		if x.n == o.n && x.text == o.text {
			return true
		}
	}
	return false
}

// parseDialog finds the dialog at the bottom of a screen: the last run of
// options numbered from 1 with one of them selected, followed by at most
// dialogTail lines. Anything else, a numbered list in an answer above
// among them, yields no options.
func parseDialog(screen string) dialog {
	lines := strings.Split(screen, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	start, end, opts := -1, -1, []option(nil)
	for i := 0; i < len(lines); i++ {
		if m := optionLine.FindStringSubmatch(lines[i]); m == nil || m[3] != "1" {
			continue
		}
		o, j := readOptions(lines, i)
		start, end, opts = i, j, o
		i = j - 1
	}
	if opts == nil {
		return dialog{}
	}
	selected, tail := 0, 0
	for _, o := range opts {
		if o.selected {
			selected++
		}
	}
	for _, l := range lines[end:] {
		if strings.TrimSpace(l) != "" {
			tail++
		}
	}
	if selected != 1 || tail > dialogTail {
		return dialog{}
	}
	return dialog{lines: headLines(lines[:start]), options: opts}
}

// readOptions reads the options numbered on from lines[i], each joined
// with the lines its text wraps onto, which Claude indents to its column.
func readOptions(lines []string, i int) ([]option, int) {
	var opts []option
	col := 0
	for ; i < len(lines); i++ {
		l := lines[i]
		if m := optionLine.FindStringSubmatch(l); m != nil {
			if m[3] != strconv.Itoa(len(opts)+1) {
				break
			}
			opts = append(opts, option{n: len(opts) + 1, text: m[4], selected: m[2] != ""})
			col = utf8.RuneCountInString(m[1]) + len(m[3]) + 1
			continue
		}
		text := strings.TrimLeft(l, " ")
		if len(opts) == 0 || text == "" || utf8.RuneCountInString(l)-utf8.RuneCountInString(text) < col {
			break
		}
		opts[len(opts)-1].text += " " + text
	}
	return opts, i
}

// headLines are the dialog's lines above its options, up to the rule
// Claude draws over it; without that rule in reach, only the question
// right above the options.
func headLines(above []string) []string {
	var out []string
	seen := 0
	for i := len(above) - 1; i >= 0 && seen < dialogHead; i-- {
		l := strings.TrimSpace(above[i])
		switch {
		case isRule(l):
			return out
		case l == "":
			continue
		}
		out = append([]string{l}, out...)
		seen++
	}
	var question []string
	for i := len(above) - 1; i >= 0; i-- {
		l := strings.TrimSpace(above[i])
		if l == "" || isRule(l) {
			break
		}
		question = append([]string{l}, question...)
	}
	return question
}

// isRule reports whether a line is only a horizontal box-drawing rule.
func isRule(l string) bool {
	if utf8.RuneCountInString(l) < 3 {
		return false
	}
	return strings.Trim(l, "─━═╌┄") == ""
}
