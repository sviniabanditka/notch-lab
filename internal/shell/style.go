package shell

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/kit"
)

// The look of the Shell: a sidebar over the window's material, and the
// work in a rounded card that floats beside it, with rounded panes split
// by gaps, as macOS apps draw them (02-shell.md, "Theme").

// PanelRadius is the radius of the notch panel's bottom corners.
const PanelRadius = 20

const (
	radiusSm  = 6  // buttons and rows
	radiusMd  = 8  // panes
	radiusLg  = 11 // tiles and groups
	cardInset = 8  // around the card, where the material shows
	// windowRadius is the corner radius macOS gives a window without a
	// toolbar. The card's corners are concentric with the window's:
	// their radius is the window's less the inset.
	windowRadius = 16
	paneGap      = 6  // between the panes of a Split
	toolbarH     = 40 // the card's top bar
	iconButton   = 28
)

// hoverMotion eases the colors of hovered and chosen controls.
var hoverMotion = ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true}

// stroke is a hairline: borders and separators, over any background.
func stroke(t *ui.Theme) ui.Color { return t.Text.Alpha(0.09) }

// hover and pressed tint what the pointer is over, from the text color, so
// they read on the material as on the card.
func hover(t *ui.Theme) ui.Color   { return t.Text.Alpha(0.06) }
func pressed(t *ui.Theme) ui.Color { return t.Text.Alpha(0.10) }

// chosen tints a selected row: the accent, light enough for text to stay
// the text color.
func chosen(t *ui.Theme) ui.Color {
	if t.Dark {
		return t.Accent.Alpha(0.30)
	}
	return t.Accent.Alpha(0.16)
}

// faint is text that labels: section titles, counts, shortcuts.
func faint(t *ui.Theme) ui.Color { return t.Text.Alpha(0.45) }

// chrome is the window's own fill where the material would be, for a
// window without vibrancy (Reduce transparency, tests).
func chrome(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGB(0x2a, 0x2a, 0x2d)
	}
	return ui.RGB(0xec, 0xec, 0xef)
}

// card is the content card's fill.
func card(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGB(0x1c, 0x1c, 0x1e)
	}
	return ui.RGB(0xff, 0xff, 0xff)
}

// iconBtn is a borderless square button with an icon, tinted on hover.
func iconBtn(c *ui.Context, key, label string, icon *ui.SVG, active bool) ui.Element {
	return kit.IconButton(c, key, label, icon, active)
}

// cardRadius keeps the card's corners concentric with the window's.
func (s *Shell) cardRadius() float32 {
	if s.Strip != nil {
		return PanelRadius - cardInset
	}
	return windowRadius - cardInset
}
