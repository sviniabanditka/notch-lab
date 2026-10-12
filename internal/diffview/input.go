package diffview

import "github.com/egoist/mygo/ui"

func (v *View) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputScroll:
		dx, dy := ev.DX, ev.DY
		if !ev.Precise {
			// A wheel notch arrives as 40; it moves three rows.
			dx, dy = dx/40*3*v.m.cw(), dy/40*3*v.m.rh()
			if ev.Mods&ui.Shift != 0 {
				dx, dy = dy, 0
			}
		}
		v.scrollX += dx
		v.scrollY += dy
		v.clampScroll()
		return true
	case ui.InputPointerDown:
		if ev.Button == 0 {
			v.click(ev.Y)
		}
		return true
	case ui.InputKeyDown:
		return v.key(ev.Key)
	}
	return false
}

func (v *View) key(k ui.Key) bool {
	rh := v.m.rh()
	switch k {
	case ui.KeyDown:
		v.scrollY += rh
	case ui.KeyUp:
		v.scrollY -= rh
	case ui.KeyPageDown, ui.KeySpace:
		v.scrollY += v.h - rh
	case ui.KeyPageUp:
		v.scrollY -= v.h - rh
	case ui.KeyHome:
		v.scrollY = 0
	case ui.KeyEnd:
		v.scrollY = float32(len(v.d.vis)) * rh
	case ui.KeyRight:
		v.scrollX += 4 * v.m.cw()
	case ui.KeyLeft:
		v.scrollX -= 4 * v.m.cw()
	default:
		return false
	}
	v.clampScroll()
	return true
}

// click toggles the file whose header is under y, the pinned one too;
// collapsing a pinned file scrolls its header to the top.
func (v *View) click(y float32) {
	rh := v.m.rh()
	if rh == 0 {
		return
	}
	first := int(v.scrollY / rh)
	if f, ok := v.stickyFile(first, v.scrollY); ok && y < rh {
		v.toggle(f)
		v.scrollY = float32(v.visIndex(v.d.files[f].first)) * rh
		v.clampScroll()
		return
	}
	vi := int((y + v.scrollY) / rh)
	if vi < 0 || vi >= len(v.d.vis) {
		return
	}
	if r := &v.d.rows[v.d.vis[vi]]; r.kind == rowFile {
		v.toggle(r.file)
		v.clampScroll()
	}
}

func (v *View) toggle(f int32) {
	v.d.toggle(f)
	delete(v.cache, v.d.files[f].first)
}

func (v *View) clampScroll() {
	total := float32(len(v.d.vis)) * v.m.rh()
	v.scrollY = min(max(v.scrollY, 0), max(total-v.h, 0))
	codeW := v.w - v.gutter().code
	width := float32(min(v.d.maxCols, maxCols)+2) * v.m.cw()
	v.scrollX = min(max(v.scrollX, 0), max(width-codeW, 0))
}
