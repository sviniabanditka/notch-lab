// Package theme holds the app's design tokens: colors, fonts, radii and
// motion. The colors come from the palette in use; presets are dark only
// for now.
package theme

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Palette is a theme's colors. Its roles are opaque; the overlays that
// borders and hovers paint (Stroke, Hover, …) derive from Text, so they
// let a background image through.
type Palette struct {
	ID, Name string
	Dark     bool

	// Background is the main content's canvas.
	Background ui.Color
	// Sidebar fills a side panel over the vibrancy material, at the
	// user's sidebar opacity; Panel fills it without the material.
	Sidebar, Panel ui.Color
	// Raised is the fill of what floats over the window: toasts, popovers.
	Raised          ui.Color
	Text, TextMuted ui.Color
	// AccentText is text on Accent.
	Accent, AccentText ui.Color
	// Attention marks what waits for the user, Success what finished,
	// Danger what failed.
	Attention, Success, Danger ui.Color
	DiffAdd, DiffDel           ui.Color
	// CodeStyle is the chroma style code is highlighted with.
	CodeStyle string
	Terminal  Terminal
}

// Terminal is the palette panes resolve their default and named colors
// with; indexed colors 16–255 stay the standard cube and grays.
type Terminal struct {
	Background, Foreground, Cursor, Selection ui.Color
	ANSI                                      [16]ui.Color
}

// The tokens of the palette in use. Views read them as they build or
// paint; Use and SetAccent change them on the main thread.
var (
	Background ui.Color
	Text       ui.Color
	TextMuted  ui.Color
	TextFaint  ui.Color
	Accent     ui.Color
	AccentText ui.Color
	Attention  ui.Color
	Success    ui.Color
	Danger     ui.Color
	DiffAdd    ui.Color
	DiffDel    ui.Color
	CodeStyle  string
	Term       Terminal

	Stroke      ui.Color
	HoverSubtle ui.Color
	Hover       ui.Color
	HoverStrong ui.Color
	Selection   ui.Color

	// PanelOverMaterial is a side panel's fill over a vibrancy material.
	PanelOverMaterial ui.Color
	Sidebar           ui.Color
	Panel             ui.Color
	Raised            ui.Color
	Shadow            = ui.RGBA(0, 0, 0, 0.45)
	// Scrim darkens what a modal covers.
	Scrim = ui.RGBA(0, 0, 0, 1)
	// Canvas fills the main content's views: Background, or nothing while
	// a background image shows behind them.
	Canvas ui.Color
	// Gap fills the room between rounded panes, a shade under Background.
	Gap ui.Color
)

const (
	LabelSize = 12
	BadgeSize = 10

	MonoFamily = "JetBrainsMono Nerd Font, SF Mono, Menlo, monospace"
	MonoSize   = 13

	RadiusSm = 4
	RadiusMd = 6
	RadiusLg = 8

	TopBarHeight = 40
)

// Motion is a duration along a CSS cubic-bezier curve.
type Motion struct {
	Duration time.Duration
	Ease     ui.Easing
}

var (
	MotionHover       = Motion{120 * time.Millisecond, cubicBezier(0.22, 1, 0.36, 1)}
	MotionTabResize   = Motion{200 * time.Millisecond, cubicBezier(0.3333, 0.6667, 0.6667, 1)}
	MotionDrawerOpen  = Motion{200 * time.Millisecond, cubicBezier(0.22, 1, 0.36, 1)}
	MotionDrawerClose = Motion{160 * time.Millisecond, cubicBezier(0.4, 0, 1, 1)}
	MotionPopover     = Motion{170 * time.Millisecond, cubicBezier(0.16, 1, 0.3, 1)}
	MotionPaneEnter   = Motion{260 * time.Millisecond, cubicBezier(0.22, 1, 0.36, 1)}
	MotionToast       = Motion{180 * time.Millisecond, cubicBezier(0.22, 1, 0.36, 1)}
	MotionBackdrop    = Motion{160 * time.Millisecond, cubicBezier(0.22, 1, 0.36, 1)}
)

// Transition is an element transition along m.
func (m Motion) Transition() ui.ElementTransition {
	return ui.ElementTransition{Duration: m.Duration, Ease: m.Ease}
}

// Presets are the built-in palettes; the first is the default.
func Presets() []Palette { return presets }

// Preset is the preset with id, or the default one.
func Preset(id string) Palette { return preset(id) }

// Use makes p the palette in use, accent overriding its accent unless it
// is zero, and canvasClear leaving the main content's canvas unfilled for
// a background image. Main thread only.
func Use(p Palette, accent ui.Color, canvasClear bool) { use(p, accent, canvasClear) }

// Current is the palette in use, its accent overridden.
func Current() Palette { return current }

// Revision changes whenever Use does, so that a view caching resolved
// colors knows to drop them.
func Revision() uint64 { return revision }

// Apply makes c's widgets and text follow the tokens.
func Apply(c *ui.Context) {
	t := *c.Theme()
	t.Dark = current.Dark
	t.Background, t.Text, t.TextMuted = Background, Text, TextMuted
	t.Surface, t.SurfaceHover, t.SurfacePressed = HoverSubtle, Hover, HoverStrong
	t.Border, t.Selection = Stroke, Selection
	t.Accent, t.Focus, t.AccentText = Accent, Accent, AccentText
	t.Warning, t.Success, t.Danger = Attention, Success, Danger
	t.FontSize, t.Radius = LabelSize, RadiusMd
	c.SetTheme(&t)
}

// Hex writes c as #rrggbb, ignoring its alpha.
func Hex(c ui.Color) string {
	const digits = "0123456789abcdef"
	b := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range [3]uint8{c.R, c.G, c.B} {
		b[1+2*i], b[2+2*i] = digits[v>>4], digits[v&15]
	}
	return string(b)
}
