package filepreview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

type kind uint8

const (
	kindLoading kind = iota
	kindText
	kindMarkdown
	kindImage
	kindBinary
	kindTooLarge
	kindFailed
)

const (
	// maxTextBytes is the largest file shown as text.
	maxTextBytes   = 5 << 20
	maxImageBytes  = 50 << 20
	maxImagePixels = 64 << 20
	// sniffBytes is how much of a file is searched for a NUL byte, which
	// marks it binary, as git and grep decide.
	sniffBytes = 8 << 10
)

// content is what reading a file found. Any goroutine may make one; the
// main thread owns it once it is handed over.
type content struct {
	kind kind
	size int64
	err  string
	text *textDoc
	md   *markdown.Doc
	img  *ui.Bitmap
	// imgW and imgH are the image's size in pixels; animated marks a GIF
	// of more than one frame, img its first.
	imgW, imgH int
	animated   bool
	took       time.Duration
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), sniffBytes)], 0) >= 0
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true}

func isMarkdown(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// read reads the file at path, only ever opening it for reading; md
// renders it when it is markdown.
func read(path string, md markdown.Options) (out content) {
	start := time.Now()
	defer func() { out.took = time.Since(start) }()
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return content{kind: kindFailed, err: err.Error()}
	case info.IsDir():
		return content{kind: kindFailed, err: "This is a folder."}
	case !info.Mode().IsRegular():
		return content{kind: kindBinary, size: info.Size()}
	}
	size := info.Size()
	if imageExts[strings.ToLower(filepath.Ext(path))] && size <= maxImageBytes {
		if c, err := readImage(path, size); err == nil {
			return c
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return content{kind: kindFailed, size: size, err: err.Error()}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxTextBytes+1))
	if err != nil {
		return content{kind: kindFailed, size: size, err: err.Error()}
	}
	switch {
	case isBinary(data):
		return content{kind: kindBinary, size: size}
	case len(data) > maxTextBytes:
		return content{kind: kindTooLarge, size: size}
	}
	src := string(data)
	c := content{kind: kindText, size: size, text: newTextDoc(src)}
	if isMarkdown(path) {
		c.kind, c.md = kindMarkdown, markdown.Parse(src, md)
	}
	return c
}

func readImage(path string, size int64) (content, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return content{}, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return content{}, err
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return content{}, errors.New("too many pixels")
	}
	bmp, err := ui.DecodeBitmap(data)
	if err != nil {
		return content{}, err
	}
	return content{kind: kindImage, size: size, img: bmp, imgW: cfg.Width, imgH: cfg.Height, animated: lightbox.Animated(data)}, nil
}

// textDoc is a text split into lines, without their line ends.
type textDoc struct {
	lines []string
	// maxCols is the widest line's cells, up to maxCols.
	maxCols int
}

func newTextDoc(src string) *textDoc {
	src = strings.TrimSuffix(src, "\n")
	d := &textDoc{lines: strings.Split(src, "\n")}
	for i, l := range d.lines {
		l = strings.TrimSuffix(l, "\r")
		d.lines[i] = l
		d.maxCols = max(d.maxCols, displayCols(l))
	}
	return d
}

const tabWidth = 4

// displayCols is how many cells a line takes once tabs are expanded,
// counting every rune as one cell, up to maxCols.
func displayCols(s string) int {
	col := 0
	for _, r := range s {
		if col >= maxCols {
			return maxCols
		}
		if r == '\t' {
			col += tabWidth - col%tabWidth
		} else {
			col++
		}
	}
	return col
}

// sizeWords is n bytes as people read it.
func sizeWords(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d bytes", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f KB", float64(n)/1000)
	case n < 1000*1000*1000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/1e9)
}

// original reads the image at path again, for the lightbox to play its
// frames.
func original(path string) func(ctx context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
		if err == nil && len(data) > maxImageBytes {
			err = errors.New("too large")
		}
		return data, err
	}
}
