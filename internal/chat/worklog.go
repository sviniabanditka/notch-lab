package chat

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

// diffFold is how many diff lines show before "Show all".
const diffFold = 20

// work is a run of tool calls as one line, which opens to a row per call.
func (v *View) work(c *ui.Context, p *paneChat, it *item) {
	open := p.toggled[it.key]
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		if disclosure(c, open, func() { workLine(c, it.calls, p.folder) }) {
			p.flip(it.key)
		}
		if !open {
			return
		}
		ui.Column(c).FillWidth().Gap(10).Padding(0, 0, 0, 18).Children(func() {
			for _, cl := range it.calls {
				callRow(c, p, cl)
			}
		})
	})
}

func workLine(c *ui.Context, calls []*transcript.Tool, folder string) {
	verb, target := label(calls, folder)
	running := verb == "Running"
	ui.Text(c, verb).FontSize(12).TextColor(theme.TextMuted)
	if target != "" {
		ui.Text(c, target).Font(theme.MonoFamily).FontSize(11.5).TextColor(theme.TextMuted).SingleLine().Shrink(1).MinWidth(0)
	}
	if running {
		ui.Text(c, "…").FontSize(12).TextColor(theme.TextMuted)
	}
	if n := failures(calls); n > 0 {
		ui.Text(c, fmt.Sprintf("· %d failed", n)).FontSize(12).TextColor(theme.Danger)
	}
}

// callRow is one call: the tool, what it acted on and how it ended, then
// its diff, or an excerpt of its result; an Agent's opens to its report.
func callRow(c *ui.Context, p *paneChat, cl *transcript.Tool) {
	key := "call/" + cl.CallID
	isAgent := cl.Family == transcript.FamilyAgent
	ui.Column(c.Key(key)).FillWidth().Gap(5).Children(func() {
		head := ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center)
		if isAgent {
			head.Cursor(ui.CursorPointer)
		}
		head.Children(func() {
			ui.Text(c, cl.Name).FontSize(12).FontWeight(600).TextColor(theme.TextMuted)
			if cl.Target != "" {
				t := firstLine(cl.Target)
				if fam := cl.Family; fam == transcript.FamilyRead || fam == transcript.FamilyEdit || fam == transcript.FamilyWrite {
					t = shortPath(t, p.folder)
				}
				ui.Text(c, t).Font(theme.MonoFamily).FontSize(11.5).TextColor(theme.Text).SingleLine().Shrink(1).MinWidth(0).
					Tooltip(cl.Target)
			}
			switch cl.State {
			case transcript.ToolRunning:
				ui.Text(c, "Running "+cl.Name+"…").FontSize(12).TextColor(theme.TextFaint)
			case transcript.ToolFailed:
				ui.Text(c, "failed").FontSize(12).TextColor(theme.Danger)
			case transcript.ToolInterrupted:
				ui.Text(c, "Not run").FontSize(12).TextColor(theme.TextFaint)
			}
		})
		if isAgent && head.Clicked() {
			p.flip(key)
		}
		switch {
		case cl.State == transcript.ToolRunning || cl.State == transcript.ToolInterrupted:
		case isAgent:
			if p.toggled[key] {
				excerpt(c, cl.Result, 0)
			}
		case len(cl.Diff) > 0 && cl.State == transcript.ToolDone:
			diff(c, p, key+"/diff", cl.Diff)
		case strings.TrimSpace(cl.Result) != "":
			excerpt(c, strings.TrimSpace(cl.Result), 3)
		}
	})
}

// diff shows an edit's lines, the first diffFold of them until "Show all".
func diff(c *ui.Context, p *paneChat, key string, diffLines []transcript.DiffLine) {
	lines := diffLines
	shown := p.toggled[key]
	if !shown && len(lines) > diffFold {
		lines = lines[:diffFold]
	}
	ui.Column(c).FillWidth().Radius(theme.RadiusMd).Border(1, theme.Stroke).Clip().Children(func() {
		ui.ScrollHorizontal(c).FillWidth().Children(func() {
			ui.Column(c).MinWidthPercent(100).Padding(4, 0).Children(func() {
				for i, l := range lines {
					diffLine(c.Key(i), l)
				}
			})
		})
		if more := len(diffLines) - len(lines); !shown && more > 0 && ui.Link(c, fmt.Sprintf("Show all (%d more lines)", more), "").FontSize(11).
			Padding(4, 10).Clicked() {
			p.flip(key)
		}
	})
}

// diffLine is one line of a diff with its marker: the decoder keeps a
// context line's leading space in its text, and strips the +/- of a change.
func diffLine(c *ui.Context, l transcript.DiffLine) {
	fill, col := ui.Transparent, theme.Text
	text := l.Text
	switch l.Op {
	case transcript.DiffHunk:
		col = theme.TextFaint
	case transcript.DiffAdd:
		fill, text = theme.DiffAdd.Alpha(0.14), "+"+text
	case transcript.DiffDel:
		fill, text = theme.DiffDel.Alpha(0.14), "-"+text
	}
	if text == "" {
		text = " "
	}
	ui.Text(c, text).Font(theme.MonoFamily).FontSize(11.5).TextColor(col).NoWrap().FillWidth().Padding(0, 10).Background(fill)
}
