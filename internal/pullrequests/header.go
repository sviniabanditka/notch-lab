package pullrequests

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

var mergeableWords = map[string]string{"MERGEABLE": "No conflicts", "CONFLICTING": "Has conflicts", "UNKNOWN": "Checking for conflicts"}

// header is a pull request's title, facts and branches, one line each,
// over its actions.
func (v *View) header(c *ui.Context, st *State, d *detail) {
	col := ui.Column(c.Key("header")).FillWidth().Padding(16, 20, 12, 20).Gap(6)
	room := col.Bounds().W - 40
	if room != v.branchRoom {
		// The width is the last frame's: after a resize or a change of panes
		// the cut would stay fitted to the old one until something redraws.
		v.branchRoom = room
		c.AnimationFrame()
	}
	col.Children(func() {
		v.titleRow(c, d.Title)
		state := d.State
		if d.Draft && d.State == "OPEN" {
			state = "Draft"
		}
		parts := []string{fmt.Sprintf("%s#%d", d.Repo, d.Number), d.Author, stateWord(state)}
		if w := mergeableWords[d.Mergeable]; w != "" && d.State == "OPEN" {
			parts = append(parts, w)
		}
		metaLine(c, parts...)
		branches(c, d.HeadRef, d.BaseRef, room)
		v.actions(c, st, d)
	})
}

// titleRow is the title on one line, the whole of it in the tooltip, after
// the back button of a narrow view.
func (v *View) titleRow(c *ui.Context, title string) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		v.backButton(c)
		ui.Text(c, title).FontSize(16).FontWeight(600).TextColor(theme.Text).SingleLine().Shrink(1).MinWidth(0).
			Tooltip(title).Selectable()
	})
}

// metaLine is facts about an item on one line, each cut short as the room
// runs out.
func metaLine(c *ui.Context, parts ...string) {
	ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
		for i, p := range parts {
			if i > 0 {
				ui.Text(c, "·").TextColor(theme.TextFaint)
			}
			ui.Text(c, p).SingleLine().Shrink(1).MinWidth(0).TextColor(theme.TextMuted).Selectable()
		}
	})
}

// stateWord is GitHub's OPEN, CLOSED, … as a header says it.
func stateWord(s string) string {
	if s == "" || s == "Draft" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

const branchArrow = " → "

// branches is "head → base" in mono, each cut in the middle so both fit
// room, the width the header had in the last frame (0 before it has one).
func branches(c *ui.Context, head, base string, room float32) {
	size := float32(theme.LabelSize - 1)
	cw, _ := c.MeasureText(0, ui.Span{Text: "0", Font: theme.MonoFamily, Size: size})
	if room > 0 && cw > 0 {
		head, base = fitBranches(head, base, int(room/cw)-len([]rune(branchArrow)))
	}
	ui.Text(c, head+branchArrow+base).Font(theme.MonoFamily).FontSize(size).SingleLine().FillWidth().
		TextColor(theme.TextMuted).Selectable()
}

// fitBranches cuts head and base to n runes together: base keeps up to
// half of them, more when head is shorter.
func fitBranches(head, base string, n int) (string, string) {
	h, b := len([]rune(head)), len([]rune(base))
	if h+b <= n {
		return head, base
	}
	keep := min(b, max(n/2, n-h))
	return cutMiddle(head, n-keep), cutMiddle(base, keep)
}

// cutMiddle shortens s to n runes with an ellipsis in its middle.
func cutMiddle(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 2 {
		return "…"
	}
	front := n / 2
	back := n - 1 - front
	return string(r[:front]) + "…" + string(r[len(r)-back:])
}
