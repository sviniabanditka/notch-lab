package chat

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// modelName says a model id in words, as claude-haiku-5-5 is Haiku 5.5
// and claude-3-5-sonnet-20241022 Sonnet 3.5; an id of another shape shows
// as it is.
func modelName(id string) string {
	base, _, _ := strings.Cut(id, "[")
	var family string
	var version []string
	for _, part := range strings.Split(strings.TrimPrefix(base, "claude-"), "-") {
		switch {
		case part == "":
		case isDigits(part) && len(part) <= 2:
			version = append(version, part)
		case isDigits(part):
			// A release date.
		case family == "":
			family = strings.ToUpper(part[:1]) + part[1:]
		default:
			return id
		}
	}
	if family == "" {
		return id
	}
	if len(version) == 0 {
		return family
	}
	return family + " " + strings.Join(version, ".")
}

func isDigits(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return !unicode.IsDigit(r) }) < 0
}

// tildePath shows a path under home from ~.
func tildePath(p, home string) string {
	if home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rel, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + rel
	}
	return p
}

// mascotOrange is the color Claude Code draws its mascot in.
var mascotOrange = ui.Hex("#D77757")

// mascot are the mascot's blocks, as Claude Code's banner draws it in
// quarter-cell block characters: per row of quarter cells, the spans of
// columns filled. A quarter cell is twice as tall as wide.
var mascot = [5][][2]int{
	{{2, 14}},
	{{2, 4}, {5, 11}, {12, 14}},
	{{0, 16}},
	{{2, 14}},
	{{3, 4}, {5, 6}, {10, 11}, {12, 13}},
}

func drawMascot(c *ui.Context, unit float32) ui.Element {
	return ui.Box(c).Size(16*unit, 10*unit).Label("Claude").Draw(func(p *ui.Painter, r ui.Rect) {
		for y, spans := range mascot {
			for _, s := range spans {
				p.Fill(ui.Rect{X: r.X + float32(s[0])*unit, Y: r.Y + float32(y)*2*unit,
					W: float32(s[1]-s[0]) * unit, H: 2 * unit}, mascotOrange, 0)
			}
		}
	})
}

// header is the strip pinned above the conversation: the mascot, Claude Code's
// version, the model and the folder.
func (v *View) header(c *ui.Context, p *paneChat) {
	if p.turns == nil {
		return
	}
	hd := p.turns.meta
	folder := hd.Folder
	if folder == "" {
		folder = p.folder
	}
	ui.Row(c.Key("header")).FillWidth().Padding(10, 16).Gap(12).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Stroke).Children(func() {
		drawMascot(c, 2.5)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Claude Code").FontWeight(600)
				if hd.AgentVersion != "" {
					ui.Text(c, "v"+hd.AgentVersion).FontSize(12).TextColor(theme.TextMuted)
				}
			})
			if hd.Model != "" {
				ui.Text(c, modelName(hd.Model)).FontSize(12).TextColor(theme.TextMuted).SingleLine()
			}
			if folder != "" {
				ui.Row(c).Gap(6).Children(func() {
					ui.Text(c, tildePath(folder, homeDir())).FontSize(12).TextColor(theme.TextMuted).
						SingleLine().Shrink(1).MinWidth(0)
					if hd.Branch != "" {
						ui.Text(c, hd.Branch).FontSize(12).TextColor(theme.TextFaint).SingleLine().Shrink(0)
					}
				})
			}
		})
	})
}
