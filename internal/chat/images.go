package chat

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/transcript"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	// thumbHeight is a prompt image's thumbnail height in pixels, twice
	// the DIPs it shows at for Retina displays.
	thumbHeight = 2 * 96
	// maxImagePixels refuses images too large to decode for a thumbnail.
	maxImagePixels = 50_000_000
)

type thumbs []*ui.Bitmap

// decodeThumbs scales a prompt's images down to thumbnails, leaving out
// those that do not decode.
func decodeThumbs(imgs []transcript.Image) thumbs {
	var out thumbs
	for _, im := range imgs {
		data := im.Data
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width*cfg.Height > maxImagePixels || cfg.Height == 0 {
			continue
		}
		src, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			continue
		}
		h := min(cfg.Height, thumbHeight)
		w := max(cfg.Width*h/cfg.Height, 1)
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
		out = append(out, ui.NewBitmap(dst))
	}
	return out
}
