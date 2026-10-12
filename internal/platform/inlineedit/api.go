// Package inlineedit edits a label in place, as a file's name in Finder.
// ui.EditableText starts only from a double click or Enter on itself; this
// one starts from the app, as a context menu's Rename does.
package inlineedit

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// inputChrome is the input's padding and border, plus room for the caret
// past the last character.
const inputChrome, minDraftWidth = 2*6 + 2*1 + 4, 24

// Field is one label being edited at a time, by key. Main thread only.
type Field struct {
	key, draft, original string
	fresh                bool
}

// Start edits the label of key, from value.
func (f *Field) Start(key, value string) {
	f.key, f.draft, f.original, f.fresh = key, value, value, true
}

// Editing reports whether a label is being edited, or the label of key
// for a non-empty key.
func (f *Field) Editing(key string) bool {
	return f.key != "" && (key == "" || key == f.key)
}

// Build adds the input for key's label. It reports the new label once,
// as Enter or a click elsewhere commits it; Escape or an empty label
// leaves the old one.
func (f *Field) Build(c *ui.Context, key string) (label string, committed bool) {
	// As wide as the draft, as the label it replaces is, never past the
	// space the label had.
	w, _ := c.MeasureText(0, ui.Span{Text: f.draft})
	in := ui.TextInput(c.Key("inlineedit"), &f.draft).Width(max(w, minDraftWidth)+inputChrome).Shrink(1).MinWidth(0).
		Height(22).Padding(0, 6).
		Margin(0, -7).Radius(theme.RadiusSm).Background(theme.Background).Border(1, theme.Accent).TextColor(theme.Text)
	switch {
	case f.fresh:
		f.fresh = false
		in.Focus()
		in.SetTextSelection(0, len([]rune(f.draft)))
	case in.Shortcut(0, ui.KeyEscape):
		f.key = ""
	case in.Submitted() || !in.Focused():
		f.key = ""
		if f.draft != "" && f.draft != f.original {
			return f.draft, true
		}
	}
	return "", false
}
