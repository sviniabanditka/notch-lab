package lightbox

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

const (
	// frameBudget bounds the pixels an animation keeps, its frames
	// composited. The GPU keeps a texture of each frame as it plays, so
	// the process holds about twice this.
	frameBudget = 64 << 20
	// maxPixels refuses originals whose decoding alone would take
	// gigabytes.
	maxPixels = 40 << 20
	// slowDelay is how long browsers show a GIF frame asking for 10 ms or
	// less.
	slowDelay = 100 * time.Millisecond
)

type frame struct {
	bmp   *ui.Bitmap
	delay time.Duration
}

// original is what decoding the original found: frames for an animation,
// or a still, and a note on what it could not show.
type original struct {
	still  *ui.Bitmap
	frames []frame
	note   string
}

// decode decodes data: every frame of an animated GIF that fits the frame
// budget, else the first. needStill false skips a still the caller has
// already.
func decode(ctx context.Context, data []byte, needStill bool) (original, error) {
	var out original
	if w, h, n, _ := scanGIF(data); n > 1 {
		if w*h > maxPixels {
			return out, fmt.Errorf("%d×%d pixels is too large", w, h)
		}
		if scale, ok := framePlan(w, h, n, frameBudget); ok {
			g, err := gif.DecodeAll(bytes.NewReader(data))
			if err != nil {
				return out, err
			}
			imgs, err := composite(ctx, g, scale)
			if err != nil {
				return out, err
			}
			out.frames = make([]frame, len(imgs))
			for i, img := range imgs {
				delay := time.Duration(g.Delay[i]) * 10 * time.Millisecond
				if g.Delay[i] <= 1 {
					delay = slowDelay
				}
				out.frames[i] = frame{bmp: ui.NewBitmap(img), delay: delay}
			}
			if scale < 1 {
				out.note = fmt.Sprintf("Animation at %.0f%%", scale*100)
			}
			return out, nil
		}
		out.note = "Too large to animate"
		needStill = true
	}
	if !needStill {
		return out, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return out, err
	}
	if cfg.Width*cfg.Height > maxPixels {
		return out, fmt.Errorf("%d×%d pixels is too large", cfg.Width, cfg.Height)
	}
	out.still, err = ui.DecodeBitmap(data)
	return out, err
}

// framePlan is the scale the frames of an n-frame w×h animation are kept
// at to fit budget bytes, and false when they would have to shrink below
// half: decoding them, a byte a pixel, would pass the budget by itself.
func framePlan(w, h, n, budget int) (float64, bool) {
	full := float64(w) * float64(h) * 4 * float64(n)
	if full <= float64(budget) {
		return 1, true
	}
	s := math.Sqrt(float64(budget) / full)
	if s < 0.5 {
		return 0, false
	}
	return s, true
}

// composite draws the frames of g one over another, disposing of each as
// it says before the next, as browsers do, and returns each frame as it
// shows, scaled by scale.
func composite(ctx context.Context, g *gif.GIF, scale float64) ([]*image.RGBA, error) {
	w, h := g.Config.Width, g.Config.Height
	sw, sh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	var saved *image.RGBA
	out := make([]*image.RGBA, 0, len(g.Image))
	for i, p := range g.Image {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var disposal byte
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			saved = clone(canvas, saved)
		}
		draw.Draw(canvas, p.Bounds(), p, p.Bounds().Min, draw.Over)
		shown := image.NewRGBA(image.Rect(0, 0, sw, sh))
		if sw == w && sh == h {
			copy(shown.Pix, canvas.Pix)
		} else {
			xdraw.ApproxBiLinear.Scale(shown, shown.Bounds(), canvas, canvas.Bounds(), draw.Src, nil)
		}
		out = append(out, shown)
		switch disposal {
		case gif.DisposalBackground:
			// Browsers clear to transparent rather than to the background
			// color.
			draw.Draw(canvas, p.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			copy(canvas.Pix, saved.Pix)
		}
	}
	return out, nil
}

// clone copies src into dst, made when nil.
func clone(src, dst *image.RGBA) *image.RGBA {
	if dst == nil {
		dst = image.NewRGBA(src.Rect)
	}
	copy(dst.Pix, src.Pix)
	return dst
}

// scanGIF reads a GIF's size and counts its frames by walking its blocks,
// without decompressing any; ok is false when data is no well-formed GIF,
// with what it read before.
func scanGIF(data []byte) (w, h, frames int, ok bool) {
	if len(data) < 13 || string(data[:3]) != "GIF" {
		return 0, 0, 0, false
	}
	w, h = int(data[6])|int(data[7])<<8, int(data[8])|int(data[9])<<8
	i := 13
	if data[10]&0x80 != 0 {
		i += 3 << (data[10]&7 + 1)
	}
	for i < len(data) {
		switch data[i] {
		case 0x21: // an extension: its label, then its sub-blocks
			i = skipBlocks(data, i+2)
		case 0x2C: // an image: its descriptor, color table and data
			if i+10 > len(data) {
				return w, h, frames, false
			}
			frames++
			flags := data[i+9]
			i += 10
			if flags&0x80 != 0 {
				i += 3 << (flags&7 + 1)
			}
			i = skipBlocks(data, i+1) // past the LZW code size
		case 0x3B:
			return w, h, frames, true
		default:
			return w, h, frames, false
		}
	}
	return w, h, frames, false
}

// skipBlocks returns the index past the data sub-blocks starting at i.
func skipBlocks(data []byte, i int) int {
	for i < len(data) {
		n := int(data[i])
		i++
		if n == 0 {
			return i
		}
		i += n
	}
	return len(data)
}
