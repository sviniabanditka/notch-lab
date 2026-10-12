// Package kit holds the controls the Shell and the Benches share: Lucide
// icons and the icon button of toolbars (02-shell.md R-SHELL-158).
package kit

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// Lucide makes an icon from the body of a Lucide shape (ISC license, see
// NOTICE), drawn in the text color.
func Lucide(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

// Icons of the toolbars.
var (
	Play         = Lucide(`<polygon points="6 3 20 12 6 21 6 3"/>`)
	PlayAll      = Lucide(`<polygon points="13 19 22 12 13 5 13 19"/><polygon points="2 19 11 12 2 5 2 19"/>`)
	Stop         = Lucide(`<rect width="14" height="14" x="5" y="5" rx="2"/>`)
	History      = Lucide(`<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l4 2"/>`)
	Save         = Lucide(`<path d="M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z"/><path d="M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7"/><path d="M7 3v4a1 1 0 0 0 1 1h7"/>`)
	Download     = Lucide(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/>`)
	Copy         = Lucide(`<rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>`)
	Format       = Lucide(`<path d="M21 6H3"/><path d="M15 12H3"/><path d="M17 18H3"/>`)
	Plus         = Lucide(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	Close        = Lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	Back         = Lucide(`<path d="m15 18-6-6 6-6"/>`)
	Forward      = Lucide(`<path d="m9 18 6-6-6-6"/>`)
	Up           = Lucide(`<path d="m18 15-6-6-6 6"/>`)
	Down         = Lucide(`<path d="m6 9 6 6 6-6"/>`)
	Refresh      = Lucide(`<path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/>`)
	Edit         = Lucide(`<path d="M12 20h9"/><path d="M16.38 3.62a1 1 0 0 1 3 3L7.37 18.64a2 2 0 0 1-.86.5l-2.87.84a.5.5 0 0 1-.62-.62l.84-2.87a2 2 0 0 1 .5-.86z"/>`)
	Replace      = Lucide(`<path d="M14 4a2 2 0 0 1 2-2"/><path d="M16 10a2 2 0 0 1-2-2"/><path d="M20 2a2 2 0 0 1 2 2"/><path d="M22 8a2 2 0 0 1-2 2"/><path d="m3 7 3 3 3-3"/><path d="M6 10V5a3 3 0 0 1 3-3h1"/><rect x="2" y="14" width="8" height="8" rx="2"/>`)
	ReplaceAll   = Lucide(`<path d="M14 14a2 2 0 0 1 2-2"/><path d="M14 20a2 2 0 0 0 2 2"/><path d="M20 12a2 2 0 0 1 2 2"/><path d="M22 20a2 2 0 0 1-2 2"/><path d="m3 7 3 3 3-3"/><path d="M6 10V5a3 3 0 0 1 3-3h1"/><rect x="2" y="14" width="8" height="8" rx="2"/><rect x="14" y="2" width="8" height="8" rx="2"/>`)
	ExternalLink = Lucide(`<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>`)
	Window       = Lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 9h18"/><path d="M8 3v6"/>`)
	Gear         = Lucide(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`)
	More         = Lucide(`<circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/>`)
	Library      = Lucide(`<path d="m16 6 4 14"/><path d="M12 6v14"/><path d="M8 8v12"/><path d="M4 4v16"/>`)
	Sliders      = Lucide(`<path d="M20 7h-9"/><path d="M14 17H5"/><circle cx="17" cy="17" r="3"/><circle cx="7" cy="7" r="3"/>`)
	Pin          = Lucide(`<line x1="12" x2="12" y1="17" y2="22"/><path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z"/>`)
	Trash        = Lucide(`<path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>`)
	Star         = Lucide(`<polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/>`)
	Lock         = Lucide(`<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>`)
	Warning      = Lucide(`<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>`)
	Folder       = Lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
)

// Size is the side of an icon button.
const Size = 28

var motion = ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true}

// IconButton is a borderless toolbar button that shows an icon. label
// names it for the tooltip and VoiceOver; a shortcut in parentheses at its
// end shows in the tooltip only. active tints it with the accent color.
func IconButton(c *ui.Context, key, label string, icon *ui.SVG, active bool) ui.Element {
	t := c.Theme()
	name := label
	if i := strings.Index(label, " ("); i > 0 {
		name = label[:i]
	}
	b := ui.Box(c.Key(key)).Size(Size, Size).Shrink(0).Center().Radius(6).Label(name).Tooltip(label).
		Cursor(ui.CursorPointer).Transition(motion)
	fg := t.TextMuted
	switch {
	case active:
		if t.Dark {
			b.Background(t.Accent.Alpha(0.30))
		} else {
			b.Background(t.Accent.Alpha(0.16))
		}
		fg = t.Accent
	case b.Pressed():
		b.Background(t.Text.Alpha(0.10))
	case b.Hovered():
		b.Background(t.Text.Alpha(0.06))
		fg = t.Text
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(15).TextColor(fg) })
	return b
}

// PrimaryIcon is the accent button of a toolbar's main action, as Run or
// Send: an icon and a short word.
func PrimaryIcon(c *ui.Context, key, label string, icon *ui.SVG, text string) ui.Element {
	t := c.Theme()
	name := label
	if i := strings.Index(label, " ("); i > 0 {
		name = label[:i]
	}
	white := ui.RGB(255, 255, 255)
	b := ui.Row(c.Key(key)).Height(Size).Shrink(0).Padding(0, 10).Gap(6).AlignItems(ui.Center).Radius(6).
		Label(name).Tooltip(label).Cursor(ui.CursorPointer).Background(t.Accent).Transition(motion)
	if b.Pressed() {
		b.Background(t.Accent.Alpha(0.8))
	} else if b.Hovered() {
		b.Background(t.Accent.Alpha(0.9))
	}
	b.Children(func() {
		ui.Icon(c, icon).FontSize(13).TextColor(white)
		if text != "" {
			ui.Text(c, text).FontWeight(600).TextColor(white)
		}
	})
	return b
}

// Off disables a button and dims it when off is true.
func Off(e ui.Element, off bool) ui.Element {
	if off {
		e.Disabled(true).Opacity(0.35)
	}
	return e
}

// Sheet is a modal of the window, over a dimmed backdrop: a title, a close
// button and fn's content, which fills the rest and scrolls on its own.
// Escape and a click outside close it, as the close button does.
func Sheet(c *ui.Context, key, title string, width, height float32, open *bool, fn func()) {
	if !*open {
		return
	}
	ui.Modal(c.Key(key), open, func() {
		ui.Column(c).Width(width).Height(height).Margin(-6).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c.Key("head")).Height(34).Shrink(0).Padding(0, 0, 4, 6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(15).FontWeight(700).Grow(1)
				if IconButton(c, "close", "Close (esc)", Close, false).Clicked() {
					*open = false
				}
			})
			ui.Column(c.Key("body")).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(fn)
		})
	})
}
