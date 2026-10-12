package markdown

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

const (
	bodySize = 13
	codeSize = 12
)

var (
	headingSizes = [7]float32{0, 20, 17, 15, 13, 13, 12}
	bullets      = []string{"•", "◦", "▪"}
)

// builder builds one doc in one frame.
type builder struct {
	c    *ui.Context
	d    *Doc
	root ui.Element
	// details counts the disclosures built, which key their open state.
	details int
	depth   int
	// row is the top-level block being built in a list, which keys the
	// open state of its disclosures with details.
	row int
}

func (d *Doc) build(c *ui.Context) ui.Element {
	col := ui.Column(c).FillWidth().Gap(10).FontSize(bodySize).TextColor(theme.Text)
	if d == nil {
		return col
	}
	more := ui.Local(col, "more", func() bool { return false })
	b := &builder{c: c, d: d, root: col}
	col.Children(func() {
		blocks := d.blocks
		folded := d.fold > 0 && more != nil && !*more
		if folded {
			blocks = blocks[:d.fold]
		}
		b.blocks(blocks)
		if folded && ui.Link(c, "Show more", "").Clicked() {
			*more = true
		}
	})
	return col
}

func (d *Doc) buildList(c *ui.Context, maxWidth float32) ui.Element {
	n := 0
	if d != nil {
		n = len(d.blocks)
	}
	list := ui.List(c, nil, n).Gap(10).FontSize(bodySize).TextColor(theme.Text)
	// Rows are built by index, some late as the list measures them, so
	// each row gets a builder of its own.
	return list.Rows(func(r ui.ListRow) {
		rc := r.Context
		row := ui.Column(rc).FillWidth()
		if maxWidth > 0 {
			row.MaxWidth(maxWidth)
		}
		row.Children(func() {
			b := &builder{c: rc, d: d, root: list, row: r.Index}
			b.block(rc, &d.blocks[r.Index])
		})
	})
}

func (b *builder) blocks(blocks []block) {
	for i := range blocks {
		b.block(b.c.Key(i), &blocks[i])
	}
}

func (b *builder) block(c *ui.Context, bl *block) {
	switch bl.kind {
	case kParagraph:
		b.runs(c, bl.runs)
	case kHeading:
		b.heading(c, bl)
	case kList:
		b.list(c, bl)
	case kCode:
		b.code(c, bl)
	case kSuggestion:
		b.suggestion(c, bl)
	case kQuote, kAlert:
		b.quote(c, bl)
	case kTable:
		b.table(c, bl)
	case kRule:
		ui.Divider(c)
	case kDetails:
		b.disclosure(c, bl)
	case kImages:
		b.images(c, bl.images)
	case kVideo:
		b.video(c, bl.video)
	}
}

func (b *builder) runs(c *ui.Context, runs []run) ui.Element {
	return ui.RichText(c).FillWidth().Children(func() {
		for i, r := range runs {
			b.runElement(c.Key(i), r)
		}
	})
}

func (b *builder) runElement(c *ui.Context, r run) {
	var e ui.Element
	switch {
	case r.path != "":
		e = ui.Link(c, r.text, "").Tooltip(r.path)
		if e.Clicked() {
			b.d.open(r.path)
		}
	case r.url != "":
		e = ui.Link(c, r.text, r.url).Tooltip(r.url)
	default:
		e = ui.Text(c, r.text)
	}
	if r.style&bold != 0 {
		e.Bold()
	}
	if r.style&italic != 0 {
		e.Italic()
	}
	if r.style&strike != 0 {
		e.Strikethrough()
	}
	if r.style&code != 0 {
		e.Font(theme.MonoFamily).FontSize(codeSize).TextBackground(theme.Text.Alpha(0.10))
	}
	if r.style&small != 0 {
		e.FontSize(bodySize - 2)
	}
}

func (b *builder) heading(c *ui.Context, bl *block) {
	level := min(max(bl.level, 1), 6)
	ui.Column(c).FillWidth().Gap(6).Padding(6, 0, 0, 0).Children(func() {
		t := b.runs(c, bl.runs).FontSize(headingSizes[level]).FontWeight(600)
		if level == 6 {
			t.TextColor(theme.TextMuted)
		}
		if level <= 2 {
			ui.Divider(c)
		}
	})
}

func (b *builder) list(c *ui.Context, bl *block) {
	b.depth++
	defer func() { b.depth-- }()
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		for i := range bl.children {
			it := &bl.children[i]
			ui.Row(c.Key(i)).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
				marker := bullets[(b.depth-1)%len(bullets)]
				color := theme.TextMuted
				switch {
				case it.task && it.checked:
					marker, color = "☑", theme.Success
				case it.task:
					marker = "☐"
				case bl.ordered:
					marker = fmt.Sprintf("%d.", bl.level+i)
				}
				ui.Text(c, marker).Width(22).TextAlign(ui.End).TextColor(color).Unselectable()
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() { b.blocks(it.children) })
			})
		}
	})
}

func (b *builder) code(c *ui.Context, bl *block) {
	ui.Column(c).FillWidth().Radius(theme.RadiusMd).Background(theme.Text.Alpha(0.05)).Border(1, theme.Stroke).Children(func() {
		head := ui.Row(c).FillWidth().Padding(5, 10, 0, 10).Gap(8).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, bl.lang).FontSize(theme.BadgeSize + 1).TextColor(theme.TextFaint).Unselectable()
			ui.Spacer(c)
			copyLink(c, head, bl.code)
		})
		ui.ScrollHorizontal(c).FillWidth().Padding(2, 10, 8, 10).Children(func() {
			ui.RichText(c, bl.spans...).Font(theme.MonoFamily).FontSize(codeSize).NoWrap()
		})
	})
}

func (b *builder) suggestion(c *ui.Context, bl *block) {
	ui.Column(c).FillWidth().Radius(theme.RadiusMd).Border(1, theme.Stroke).Clip().Children(func() {
		ui.Text(c, "Suggested change").FontSize(theme.BadgeSize+1).FontWeight(600).TextColor(theme.TextMuted).
			Padding(5, 10).Unselectable()
		ui.ScrollHorizontal(c).FillWidth().Children(func() {
			ui.Column(c).MinWidthPercent(100).Children(func() {
				line := func(i int, mark, text string, fill ui.Color) {
					ui.Text(c.Key(i), mark+" "+text).Font(theme.MonoFamily).FontSize(codeSize).NoWrap().
						FillWidth().Padding(1, 10).Background(fill)
				}
				for i, l := range bl.removed {
					line(i, "-", l, theme.DiffDel.Alpha(0.14))
				}
				for i, l := range bl.added {
					line(len(bl.removed)+i, "+", l, theme.DiffAdd.Alpha(0.14))
				}
			})
		})
	})
}

func (b *builder) quote(c *ui.Context, bl *block) {
	border, title := theme.Text.Alpha(0.18), ""
	if bl.kind == kAlert {
		border = alertColor(bl.alert)
		title = bl.alert[:1] + strings.ToLower(bl.alert[1:])
	}
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Stretch).Children(func() {
		ui.Box(c).Width(3).Radius(1.5).Background(border)
		col := ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Padding(2, 0)
		if bl.kind == kQuote {
			col.TextColor(theme.TextMuted)
		}
		col.Children(func() {
			if title != "" {
				ui.Text(c, title).FontWeight(600).TextColor(border)
			}
			b.blocks(bl.children)
		})
	})
}

func (b *builder) table(c *ui.Context, bl *block) {
	cols := 0
	for _, r := range bl.rows {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return
	}
	tracks := make([]ui.Track, cols)
	for i := range tracks {
		tracks[i] = ui.FitContent()
	}
	ui.ScrollHorizontal(c).FillWidth().Children(func() {
		ui.Grid(c).ColumnTracks(tracks...).Border(0.5, theme.Stroke).Children(func() {
			for ri, row := range bl.rows {
				for ci := range cols {
					cell := ui.Column(c.Key(ri*cols+ci)).Padding(5, 10).MaxWidth(420).Border(0.5, theme.Stroke)
					if ri == 0 {
						cell.Background(theme.HoverSubtle).FontWeight(600)
					}
					cell.Children(func() {
						if ci >= len(row) {
							return
						}
						t := b.runs(c, row[ci])
						if ci < len(bl.align) {
							t.TextAlign(bl.align[ci])
						}
					})
				}
			}
		})
	})
}

func (b *builder) disclosure(c *ui.Context, bl *block) {
	b.details++
	open := ui.Local(b.root, [2]int{b.row, b.details}, func() bool { return false })
	if open == nil {
		return
	}
	var label strings.Builder
	for _, r := range bl.runs {
		label.WriteString(r.text)
	}
	if label.Len() == 0 {
		label.WriteString("Details")
	}
	ui.Collapsible(c, label.String(), open, func() {
		ui.Column(c).FillWidth().Gap(10).Padding(4, 0, 0, 18).Children(func() { b.blocks(bl.children) })
	})
}

func (b *builder) images(c *ui.Context, ims []picture) {
	row := ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start)
	if len(ims) > 1 {
		row.Wrap()
	}
	row.Children(func() {
		for i, im := range ims {
			b.image(c.Key(i), im)
		}
	})
}

func (b *builder) image(c *ui.Context, im picture) {
	alt := im.alt
	if alt == "" {
		alt = "image"
	}
	var e *entry
	var key string
	if b.d != nil && b.d.images != nil {
		if k, ok := b.d.images.l.key(im.src); ok {
			key, e = k, b.d.images.l.get(k)
		}
	}
	switch {
	case e == nil || e.state == imgFailed:
		if safeURL(im.src) {
			ui.Link(c, "🖼 "+alt, im.src).Tooltip(im.src)
		} else {
			ui.Text(c, "🖼 "+alt).TextColor(theme.TextMuted)
		}
	case e.state == imgLoading:
		ui.Row(c).Gap(6).Padding(6, 10).AlignItems(ui.Center).Radius(theme.RadiusSm).Background(theme.Text.Alpha(0.05)).Children(func() {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, alt).TextColor(theme.TextFaint)
		})
	default:
		b.picture(c, im, key, e, alt)
	}
}

// picture shows a loaded image. A click zooms it into the lightbox, unless
// it links to another page, which a click opens; an animation shows its
// first frame, marked, and plays in the lightbox.
func (b *builder) picture(c *ui.Context, im picture, key string, e *entry, alt string) {
	w := e.w
	if im.width > 0 {
		w = im.width
	}
	target := im.href
	if target == "" {
		target = im.src
	}
	zoom := b.d.zoom != nil && (im.href == "" || im.href == im.src)
	var hit ui.Element
	if e.animated {
		hit = ui.Box(c).Width(w).Shrink(1).MinWidth(0).Children(func() {
			ui.Image(c, e.bmp).FillWidth().AspectRatio(e.w / e.h).Radius(theme.RadiusSm).Label(alt)
			ui.Row(c).Absolute().Left(6).Top(6).Gap(4).Padding(2, 6).Radius(theme.RadiusSm).AlignItems(ui.Center).
				Background(theme.Scrim.Alpha(0.6)).PassThrough().Children(func() {
				icon.Play(c, 8, ui.RGBA(255, 255, 255, 0.9))
				ui.Text(c, "GIF").FontSize(theme.BadgeSize).FontWeight(600).TextColor(ui.RGBA(255, 255, 255, 0.9)).Unselectable()
			})
		})
	} else {
		hit = ui.Image(c, e.bmp).Width(w).Shrink(1).MinWidth(0).AspectRatio(e.w / e.h).Radius(theme.RadiusSm).Label(alt)
	}
	switch {
	case zoom:
		hit.Cursor(ui.CursorPointer).Tooltip("Click to zoom")
	case safeURL(target):
		hit.Cursor(ui.CursorPointer).Tooltip(target)
	}
	if !hit.Clicked() {
		return
	}
	if !zoom {
		if safeURL(target) {
			c.OpenURL(target)
		}
		return
	}
	title := im.alt
	if title == "" {
		title = path.Base(strings.TrimSuffix(redact(im.src), "/"))
	}
	web := ""
	if safeURL(im.src) {
		web = im.src
	}
	b.d.zoom(lightbox.Picture{Title: title, Preview: e.bmp, W: int(e.w), H: int(e.h), URL: web, From: hit.Bounds(),
		Load: b.d.images.l.original(key)})
}

// alertColor is the color of a GitHub alert's kind, from the theme.
func alertColor(kind string) ui.Color {
	switch kind {
	case "NOTE":
		return theme.Term.ANSI[12]
	case "TIP":
		return theme.Success
	case "IMPORTANT":
		return theme.Term.ANSI[13]
	case "WARNING":
		return theme.Attention
	}
	return theme.Danger
}

// video is a card for a video, which only the browser plays.
func (b *builder) video(c *ui.Context, u string) {
	card := ui.Row(c).Gap(10).Padding(8, 14, 8, 8).Radius(theme.RadiusMd).Border(1, theme.Stroke).AlignItems(ui.Center).
		Background(theme.Text.Alpha(0.04)).Cursor(ui.CursorPointer).MaxWidthPercent(100).Tooltip(u).Label("Video, opens in the browser")
	if card.Hovered() {
		card.Background(theme.Text.Alpha(0.08))
	}
	card.Children(func() {
		ui.Row(c).Size(32, 32).Radius(16).Center().Background(theme.Text.Alpha(0.12)).Children(func() {
			icon.Play(c, 12, theme.Text)
		})
		ui.Column(c).Gap(2).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, "Video").FontWeight(600).Unselectable()
			ui.Text(c, path.Base(redact(u))+" · opens in the browser").SingleLine().FontSize(theme.BadgeSize + 1).
				TextColor(theme.TextFaint).Unselectable()
		})
	})
	if card.Clicked() {
		c.OpenURL(u)
	}
}
