package filepreview

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

func (v *View) build(c *ui.Context, inset float32) {
	d := v.top()
	if d == nil {
		return
	}
	col := ui.Column(c.Key("preview")).Grow(1).FillHeight().Background(theme.Canvas)
	if col.Shortcut(0, ui.KeyEscape) {
		v.host.LeavePreview()
	}
	col.Children(func() {
		v.topBar(c, d, inset)
		v.body(c, d)
	})
	if !d.shown && d.kind != kindLoading {
		d.shown = true
		log.Printf("filepreview: %s built %v after open", filepath.Base(d.path), time.Since(d.openedAt).Round(time.Microsecond))
	}
}

func (v *View) topBar(c *ui.Context, d *doc, inset float32) {
	ui.Row(c).Height(theme.TopBarHeight).FillWidth().Padding(0, 14, 0, inset+6).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Stroke).DragWindow().Children(func() {
		ui.Text(c, shownPath(d.path, d.root)).FontWeight(600).SingleLine().Shrink(1).MinWidth(0).TextColor(theme.Text).
			Tooltip(d.path)
		if info := d.info(); info != "" {
			ui.Text(c, info).SingleLine().Shrink(2).MinWidth(0).FontSize(theme.BadgeSize + 1).TextColor(theme.TextFaint)
		}
		ui.Spacer(c)
		if d.kind == kindMarkdown {
			mode := 0
			if d.showSource {
				mode = 1
			}
			if ui.Segmented(c, &mode, "Preview", "Source").Changed() {
				d.showSource = mode == 1
			}
		}
		if ui.Button(c, "Open in Editor").Tooltip("Open in $EDITOR in a pane to the right").Clicked() {
			v.host.OpenInEditor(d.path)
		}
	})
}

// shownPath is path relative to root when it lies there.
func shownPath(path, root string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

// info sums the file up for the top bar.
func (d *doc) info() string {
	switch d.kind {
	case kindText, kindMarkdown:
		n := len(d.text.lines)
		unit := "lines"
		if n == 1 {
			unit = "line"
		}
		return fmt.Sprintf("%s · %d %s", sizeWords(d.size), n, unit)
	case kindImage:
		if d.animated {
			return fmt.Sprintf("%s · %d × %d · animated, click to play", sizeWords(d.size), d.imgW, d.imgH)
		}
		return fmt.Sprintf("%s · %d × %d", sizeWords(d.size), d.imgW, d.imgH)
	case kindBinary, kindTooLarge:
		return sizeWords(d.size)
	}
	return ""
}

func (v *View) body(c *ui.Context, d *doc) {
	if d.kind == kindMarkdown && d.jump {
		d.showSource = true
	}
	switch d.kind {
	case kindLoading:
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() { ui.Spinner(c) })
	case kindFailed:
		v.placeholder(c, d, "Cannot preview this file", d.err, false)
	case kindBinary:
		v.placeholder(c, d, "Binary file", sizeWords(d.size)+" of binary data is not shown.", true)
	case kindTooLarge:
		v.placeholder(c, d, "Too large to preview",
			fmt.Sprintf("%s; the preview shows files up to %s.", sizeWords(d.size), sizeWords(maxTextBytes)), true)
	case kindImage:
		focusable(c, d).Padding(16).Center().Background(theme.Canvas).Children(func() {
			name := filepath.Base(d.path)
			img := ui.Image(c, d.img).Fill().Fit(ui.ScaleDown).Label(name).Cursor(ui.CursorPointer).Tooltip("Click to zoom")
			if img.Clicked() {
				v.host.Zoom(lightbox.Picture{Title: name, Preview: d.img, W: d.imgW, H: d.imgH,
					From: scaledDown(img.Bounds(), d.imgW, d.imgH), Load: original(d.path)})
			}
		})
	case kindMarkdown:
		if d.showSource {
			v.code(c, d)
			return
		}
		focusable(c, d).Children(func() {
			d.md.BuildList(c.Key(d.path+"\x00md"), 900).Grow(1).FillWidth().Padding(20, 28, 28, 28)
		})
	case kindText:
		v.code(c, d)
	}
}

// focusable is a column filling the body that takes the keyboard as the
// file shows, so that Esc leaves the view.
func focusable(c *ui.Context, d *doc) ui.Element {
	return ui.Column(c.Key(d.path)).Grow(1).FillWidth().Focusable().AutoFocus().FocusRing(false)
}

func (v *View) code(c *ui.Context, d *doc) {
	if d.code == nil {
		d.code = newCodeView(filepath.Base(d.path), d.text, v.host.Update)
		d.code.scrollY = d.scrollY
		d.code.mark = d.line
		if d.jump {
			d.code.jumpTo, d.jump = d.line, false
		}
	}
	d.code.build(c.Key(d.path))
}

func (v *View) placeholder(c *ui.Context, d *doc, title, detail string, editor bool) {
	focusable(c, d).Center().Gap(8).Padding(24).Children(func() {
		ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(theme.TextMuted)
		if detail != "" {
			ui.Text(c, detail).TextColor(theme.TextFaint).Wrap()
		}
		if editor && ui.Button(c, "Open in Editor").Clicked() {
			v.host.OpenInEditor(d.path)
		}
	})
}

// scaledDown is where a w×h image shows in box as ui.ScaleDown places it.
func scaledDown(box ui.Rect, w, h int) ui.Rect {
	if w <= 0 || h <= 0 {
		return ui.Rect{}
	}
	s := min(box.W/float32(w), box.H/float32(h), 1)
	sw, sh := float32(w)*s, float32(h)*s
	return ui.Rect{X: box.X + (box.W-sw)/2, Y: box.Y + (box.H-sh)/2, W: sw, H: sh}
}
