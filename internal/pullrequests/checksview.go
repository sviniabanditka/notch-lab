package pullrequests

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// noteChecks keeps what d's checks were at this load, and opens them as
// opens says.
func (v *View) noteChecks(d *detail) {
	if d.Issue {
		return
	}
	k := d.key()
	s := summarize(d.CheckList)
	var prev *checksSummary
	if p, ok := v.checkRuns[k]; ok {
		prev = &p
	}
	if s.opens(prev) {
		v.checksOpen[k] = true
	}
	v.checkRuns[k] = s
}

// checksSection is one summary line of d's checks, which opens to a row
// per check.
func (v *View) checksSection(c *ui.Context, d *detail) {
	section(c, "Checks")
	s := summarize(d.CheckList)
	if len(d.CheckList) == 0 {
		ui.Text(c, s.String()).TextColor(theme.TextFaint)
		return
	}
	k := d.key()
	// A row of its own rather than ui.Collapsible, whose click lands after
	// the frame is laid out and shows only with the next one.
	tr := ui.Row(c.Key("checks")).AlignSelf(ui.Start).Height(24).Gap(8).Padding(0, 8, 0, 2).Radius(theme.RadiusMd).
		AlignItems(ui.Center).Focusable().Role(ui.RoleDisclosure).Label("Checks: " + s.String())
	if tr.Clicked() || tr.Shortcut(0, ui.KeyEnter) || tr.Shortcut(0, ui.KeySpace) {
		v.checksOpen[k] = !v.checksOpen[k]
	}
	if tr.Hovered() {
		tr.Background(theme.HoverSubtle)
	}
	open := v.checksOpen[k]
	turn := float32(0)
	if open {
		turn = 1
	}
	tr.Children(func() {
		icon.Chevron(c, 14, theme.TextMuted, turn)
		checkDot(c, s.state())
		ui.Text(c, s.String()).SingleLine().TextColor(theme.Text)
	})
	if !open {
		return
	}
	ui.Column(c).FillWidth().Padding(2, 0, 0, 24).Children(func() {
		for i, ch := range d.CheckList {
			word, kind := describe(ch)
			ui.Row(c.Key(i)).FillWidth().Height(24).Gap(8).AlignItems(ui.Center).Children(func() {
				checkDot(c, kindStates[kind])
				ui.Text(c, ch.Name).SingleLine().Grow(1).Shrink(1).MinWidth(0).TextColor(theme.Text)
				ui.Text(c, word).SingleLine().TextColor(theme.TextFaint)
			})
		}
	})
}

// kindStates colour a check's dot by what it counts as; a skipped one has
// none.
var kindStates = map[checkKind]checkState{kindPassed: checkPassed, kindFailed: checkFailed, kindPending: checkPending}
