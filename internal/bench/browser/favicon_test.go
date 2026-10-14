package browser

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"

	"golang.org/x/image/bmp"
)

func ico(entry []byte, w byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0, 1, 0, 1, 0})
	e := make([]byte, 16)
	e[0], e[1] = w, w
	binary.LittleEndian.PutUint32(e[8:], uint32(len(entry)))
	binary.LittleEndian.PutUint32(e[12:], 22)
	b.Write(e)
	b.Write(entry)
	return b.Bytes()
}

// R-SHELL-053: a site's ICO icon decodes, with a PNG or a DIB in it.
func TestDecodeIcon(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	var p bytes.Buffer
	_ = png.Encode(&p, img)
	if decodeIcon(ico(p.Bytes(), 16)) == nil {
		t.Error("an ICO with a PNG")
	}
	var m bytes.Buffer
	_ = bmp.Encode(&m, img)
	dib := append([]byte(nil), m.Bytes()[14:]...)
	binary.LittleEndian.PutUint32(dib[8:], 32) // with the mask's height
	dib = append(dib, make([]byte, 16*4)...)  // the AND mask
	if decodeIcon(ico(dib, 16)) == nil {
		t.Error("an ICO with a DIB")
	}
	if decodeIcon([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16"/></svg>`)) == nil {
		t.Error("an SVG")
	}
	if decodeIcon([]byte("<html>")) != nil {
		t.Error("a page is no icon")
	}
}
