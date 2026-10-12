package tasks

import (
	"os"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// chip is one side of the fork/fresh switch.
func (v *View) chip(c *ui.Context, label string, on, disabled bool, col ui.Color) ui.Element {
	r := ui.Row(c.Key(label)).Height(24).Padding(0, 10).Radius(theme.RadiusSm).AlignItems(ui.Center).
		Role(ui.RoleRadio).Checked(on).Label(label).Disabled(disabled)
	text := theme.TextMuted
	switch {
	case disabled:
		text = theme.TextFaint
	case on:
		r.Background(theme.Raised).Border(1, theme.Stroke)
		text = col
	case r.Hovered():
		text = theme.Text
	}
	r.Children(func() { ui.Text(c, label).FontSize(theme.LabelSize).TextColor(text) })
	return r
}

// check is a toggle as a box and a label, in col while on.
func (v *View) check(c *ui.Context, label string, on, disabled bool, col ui.Color) ui.Element {
	r := ui.Row(c.Key(label)).Height(24).Padding(0, 8).Gap(6).Radius(theme.RadiusSm).AlignItems(ui.Center).
		Role(ui.RoleCheckBox).Checked(on).Label(label).Disabled(disabled)
	text, box := theme.TextMuted, theme.Stroke
	switch {
	case disabled:
		text = theme.TextFaint
	case on:
		text, box = col, col
	}
	if !disabled && r.Hovered() {
		r.Background(theme.HoverSubtle)
	}
	r.Children(func() {
		ui.Box(c).Size(13, 13).Radius(3).Border(1, box).Center().Children(func() {
			if on {
				icon.Check(c, 9, col)
			}
		})
		ui.Text(c, label).FontSize(theme.LabelSize).TextColor(text)
	})
	return r
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rest, ok := strings.CutPrefix(p, home); ok && (rest == "" || rest[0] == '/') {
			return "~" + rest
		}
	}
	return p
}
