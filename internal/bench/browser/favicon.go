package browser

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
)

// TabIcon is the site's icon of a Tab, for its pinned tile (R-SHELL-053):
// nil while it loads, or for a site without one that decodes.
func (b *Bench) TabIcon(t *model.Tab) ui.ImageSource {
	u := readData(t).Favicon
	if u == "" {
		return nil
	}
	if b.icons == nil {
		b.icons = map[string]ui.ImageSource{}
	}
	if src, ok := b.icons[u]; ok {
		return src
	}
	b.icons[u] = nil // asked once a run
	go func() {
		src := fetchIcon(u)
		if src != nil {
			b.Post(func() { b.icons[u] = src })
		}
	}()
	return nil
}

var iconClient = &http.Client{Timeout: 8 * time.Second}

// fetchIcon gets and decodes a site's icon: PNG, JPEG, GIF, WebP, BMP,
// SVG or ICO; nil when it fails.
func fetchIcon(u string) ui.ImageSource {
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return nil
	}
	res, err := iconClient.Get(u)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil
	}
	return decodeIcon(body)
}

func decodeIcon(body []byte) ui.ImageSource {
	if bytes.HasPrefix(body, []byte{0, 0, 1, 0}) {
		body = fromICO(body)
	}
	if head := bytes.TrimSpace(body[:min(len(body), 512)]); bytes.HasPrefix(head, []byte("<svg")) || bytes.HasPrefix(head, []byte("<?xml")) {
		if svg, err := ui.ParseSVG(body); err == nil {
			return svg
		}
		return nil
	}
	if bmp, err := ui.DecodeBitmap(body); err == nil {
		return bmp
	}
	return nil
}

// fromICO returns the largest image of an ICO file: its PNG, or its DIB
// as a BMP file (the DIB's height counts its mask too); nil when it is
// damaged.
func fromICO(ico []byte) []byte {
	if len(ico) < 6 {
		return nil
	}
	n := int(binary.LittleEndian.Uint16(ico[4:]))
	var best []byte
	bestW := 0
	for i := 0; i < n; i++ {
		e := 6 + 16*i
		if e+16 > len(ico) {
			return nil
		}
		w := int(ico[e])
		if w == 0 {
			w = 256
		}
		size := int(binary.LittleEndian.Uint32(ico[e+8:]))
		off := int(binary.LittleEndian.Uint32(ico[e+12:]))
		if off < 0 || size <= 0 || off+size > len(ico) || w <= bestW {
			continue
		}
		best, bestW = ico[off:off+size], w
	}
	if len(best) < 40 || bytes.HasPrefix(best, []byte("\x89PNG")) {
		return best
	}
	dib := append([]byte(nil), best...)
	h := int32(binary.LittleEndian.Uint32(dib[8:]))
	binary.LittleEndian.PutUint32(dib[8:], uint32(h/2))
	bpp := binary.LittleEndian.Uint16(dib[14:])
	colors := binary.LittleEndian.Uint32(dib[32:])
	if colors == 0 && bpp <= 8 {
		colors = 1 << bpp
	}
	pixels := 14 + binary.LittleEndian.Uint32(dib[0:]) + 4*colors
	file := make([]byte, 14, 14+len(dib))
	copy(file, "BM")
	binary.LittleEndian.PutUint32(file[2:], uint32(14+len(dib)))
	binary.LittleEndian.PutUint32(file[10:], pixels)
	return append(file, dib...)
}
