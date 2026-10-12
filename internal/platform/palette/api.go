// Package palette is the chrome every palette over the window shares: the
// dimmed backdrop, the panel that grows in as it opens, the query input,
// the row look and the hint footer. What a palette lists is its own.
package palette

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

const (
	Width     = 560
	RowHeight = 34
	MaxRows   = 10
	// ListHeight is the most the list grows to before it scrolls.
	ListHeight = MaxRows*RowHeight + 12
	// The panel grows from this scale as it opens; mygo has no transform,
	// so it is the panel's width that grows.
	startScale = 0.94
	startDrop  = 8
	backdrop   = 0.35
)

// Frame adds the palette keyed key over the window; call it in every
// frame, so it animates in and out. A click outside or Esc clears *open.
// content runs only while the panel shows.
func Frame(c *ui.Context, key, label string, open *bool, content func()) {
	ui.Overlay(c, func() {
		back := ui.Column(c.Key(key)).Absolute().Left(0).Top(0)
		pm, bm := theme.MotionPopover, theme.MotionBackdrop
		shown := back.AnimateWith("shown", b2f(*open), pm.Duration, pm.Ease)
		dim := back.AnimateWith("dim", b2f(*open), bm.Duration, bm.Ease)
		if !*open && shown == 0 && dim == 0 {
			back.Size(0, 0)
			return
		}
		back.Right(0).Bottom(0).AlignItems(ui.Center).Padding(theme.TopBarHeight+56, 16, 16, 16).
			Background(theme.Scrim.Alpha(backdrop * dim))
		if *open {
			back.Modal()
		} else {
			back.PassThrough()
		}
		back.Children(func() {
			panel := ui.Column(c).Width(Width*(startScale+(1-startScale)*shown)).MaxWidthPercent(100).
				Margin(startDrop*(1-shown), 0, 0, 0).Opacity(shown).Radius(theme.RadiusLg+2).Clip().
				Background(theme.Raised).Border(1, theme.Stroke).Shadow(0, 16, 48, 0, theme.Shadow).
				Role(ui.RoleDialog).Label(label)
			if *open && (panel.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape)) {
				*open = false
			}
			panel.Children(content)
		})
	})
}

// Input is the query row; it takes the focus when *fresh, then clears it.
func Input(c *ui.Context, query *string, placeholder, label string, fresh *bool) ui.Element {
	var in ui.Element
	ui.Row(c).FillWidth().Height(46).Padding(0, 14).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Stroke).Children(func() {
		icon.Search(c, 15, theme.TextMuted)
		in = ui.TextInputBase(c.Key("query"), query).Grow(1).FontSize(14).TextColor(theme.Text).
			Placeholder(placeholder).Label(label).AutoFocus()
		if *fresh {
			*fresh = false
			in.Focus()
		}
	})
	return in
}

// Row is a list row, highlighted when selected or hovered; it returns
// the row and the text color its content should take.
func Row(c *ui.Context, label string, selected bool) (ui.Element, ui.Color) {
	r := ui.Row(c).FillWidth().Height(RowHeight).Padding(0, 10).Gap(10).Radius(theme.RadiusMd).
		AlignItems(ui.Center).Label(label).Selected(selected)
	switch {
	case selected:
		r.Background(theme.Hover)
	case r.Hovered():
		r.Background(theme.HoverSubtle)
	}
	if selected {
		return r, theme.Text
	}
	return r, theme.TextMuted
}

// Note is a line in place of the list: nothing matches, still loading.
func Note(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).TextColor(theme.TextFaint).Padding(16, 16)
}

// Footer lists keyboard hints; trailing, when set, fills its right end.
func Footer(c *ui.Context, hints []string, trailing func()) {
	ui.Row(c).FillWidth().Height(30).Padding(0, 14).Gap(14).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(theme.Stroke).Children(func() {
		for _, hint := range hints {
			ui.Text(c, hint).FontSize(theme.BadgeSize + 1).TextColor(theme.TextFaint)
		}
		if trailing != nil {
			ui.Spacer(c)
			trailing()
		}
	})
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
