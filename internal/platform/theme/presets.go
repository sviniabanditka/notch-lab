package theme

import "github.com/egoist/mygo/ui"

// darkANSI is the terminal's 16 named colors in every dark preset: T3
// Code ships no ANSI palette under its license.
var darkANSI = hexes(
	"#3b4252", "#f87171", "#4ade80", "#fbbf24", "#60a5fa", "#c084fc", "#22d3ee", "#e8eef2",
	"#64748b", "#fca5a5", "#86efac", "#fde68a", "#93c5fd", "#d8b4fe", "#67e8f9", "#f8fafc",
)

// herdr is the app's own look, kept as the default.
var herdr = Palette{
	ID: "herdr", Name: "herdr", Dark: true,
	Background: ui.Hex("#171717"), Sidebar: ui.Hex("#171717"), Panel: ui.Hex("#1c1c1c"), Raised: ui.Hex("#212121"),
	Text: ui.Hex("#ebebeb"), TextMuted: ui.Hex("#ebebeb").Alpha(0.6),
	Accent: ui.Hex("#459bf7"), AccentText: ui.Hex("#ffffff"),
	Attention: ui.Hex("#fbbf24"), Danger: ui.Hex("#f43f5e"),
	Terminal: Terminal{Background: ui.Hex("#171717"), Foreground: ui.Hex("#ebebeb"), Cursor: ui.Hex("#ebebeb"),
		Selection: ui.Hex("#ebebeb").Alpha(0.18)},
}

var presets = func() []Palette {
	out := []Palette{darkDefaults(herdr)}
	for _, t := range t3Presets {
		out = append(out, darkDefaults(t.palette()))
	}
	return out
}()

// darkDefaults fills the roles no t3 palette has.
func darkDefaults(p Palette) Palette {
	p.Dark = true
	p.Success = ui.Hex("#2dd4bf")
	p.DiffAdd, p.DiffDel = ui.Hex("#10b981"), ui.Hex("#f43f5e")
	p.CodeStyle = "github-dark"
	p.Terminal.ANSI = darkANSI
	return p
}

// t3Colors is the part of a T3 Code palette the app takes; the generator
// writes them, named after t3's roles.
type t3Colors struct {
	ID, Name                                  string
	Canvas, Sidebar, SurfaceOverlay           ui.Color
	Text, TextMuted, Accent, AccentForeground ui.Color
	Warning, Error                            ui.Color
	TerminalBackground, TerminalForeground    ui.Color
	TerminalCursor, TerminalSelection         ui.Color
}

func (t t3Colors) palette() Palette {
	return Palette{
		ID: t.ID, Name: t.Name,
		Background: t.Canvas, Sidebar: t.Sidebar, Panel: t.Sidebar, Raised: t.SurfaceOverlay,
		Text: t.Text, TextMuted: t.TextMuted, Accent: t.Accent, AccentText: t.AccentForeground,
		Attention: t.Warning, Danger: t.Error,
		Terminal: Terminal{Background: t.TerminalBackground, Foreground: t.TerminalForeground,
			Cursor: t.TerminalCursor, Selection: t.TerminalSelection},
	}
}

func preset(id string) Palette {
	for _, p := range presets {
		if p.ID == id {
			return p
		}
	}
	return presets[0]
}

var (
	current  Palette
	revision uint64
)

func use(p Palette, accent ui.Color, canvasClear bool) {
	if accent != (ui.Color{}) {
		p.Accent = accent
	}
	current = p
	revision++
	Background, Text, TextMuted, TextFaint = p.Background, p.Text, p.TextMuted, p.Text.Alpha(0.4)
	Accent, AccentText = p.Accent, p.AccentText
	Attention, Success, Danger = p.Attention, p.Success, p.Danger
	DiffAdd, DiffDel, CodeStyle, Term = p.DiffAdd, p.DiffDel, p.CodeStyle, p.Terminal
	Stroke, HoverSubtle, Hover, HoverStrong, Selection =
		p.Text.Alpha(0.07), p.Text.Alpha(0.05), p.Text.Alpha(0.10), p.Text.Alpha(0.15), p.Text.Alpha(0.18)
	Sidebar, Panel, Raised = p.Sidebar, p.Panel, p.Raised
	PanelOverMaterial = p.Sidebar.Alpha(0.85)
	Canvas = p.Background
	if canvasClear {
		Canvas = ui.Transparent
	}
	Gap = mix(p.Background, ui.RGB(0, 0, 0), 0.45)
}

func init() { use(presets[0], ui.Color{}, false) }

func mix(a, b ui.Color, t float32) ui.Color {
	ch := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return ui.RGB(ch(a.R, b.R), ch(a.G, b.G), ch(a.B, b.B))
}

func hexes(s ...string) (out [16]ui.Color) {
	for i, h := range s {
		out[i] = ui.Hex(h)
	}
	return out
}
