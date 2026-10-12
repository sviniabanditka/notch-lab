package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// trayIcon draws the menu bar icon as a template image at 2x: the outline
// of a notch, with a dot under it when a Tab is Unread (R-SHELL-032).
func trayIcon(unread bool) []byte {
	const w, h = 36, 36
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	ink := color.NRGBA{0, 0, 0, 255}
	// A rounded bar hanging from the top: the notch.
	for y := 6; y < 18; y++ {
		for x := 4; x < 32; x++ {
			inner := y < 15 && x >= 7 && x < 29
			corner := y >= 15 && (x < 7 || x >= 29) && (x-7)*(x-7)+(y-14)*(y-14) > 9 && (x-28)*(x-28)+(y-14)*(y-14) > 9
			if !inner && !corner {
				img.Set(x, y, ink)
			}
		}
	}
	if unread {
		for y := 22; y < 30; y++ {
			for x := 14; x < 22; x++ {
				if (x-18)*(x-18)+(y-26)*(y-26) <= 14 {
					img.Set(x, y, ink)
				}
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
