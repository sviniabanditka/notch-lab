package markdown

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
)

const (
	// maxImageBytes is the most a download may be.
	maxImageBytes = 10 << 20
	// maxImagePixels refuses images whose decoding alone would take
	// gigabytes, whatever their file's size.
	maxImagePixels = 40 << 20
	// maxPixelWidth is the widest a kept bitmap is: twice the reading
	// width, for Retina screens.
	maxPixelWidth = 1600
	// cacheBytes bounds the decoded pixels kept.
	cacheBytes = 96 << 20
	// fetchers is how many images download at once.
	fetchers = 4
)

type imgState uint8

const (
	imgLoading imgState = iota
	imgReady
	imgFailed
)

type entry struct {
	state imgState
	bmp   *ui.Bitmap
	// w and h are the image's size in DIPs: its pixel size before scaling.
	w, h float32
	// animated marks a GIF of more than one frame; bmp is its first.
	animated bool
	bytes    int
	used     uint64
}

// loader fetches images and keeps them by URL. Its map is the main
// thread's; fetches hand results over through post.
type loader struct {
	post      func(func())
	source    func(*url.URL) *url.URL
	authorize func(*url.URL) string
	// local, when set, reads the files it resolves srcs to instead of
	// fetching URLs.
	local func(src string) (string, bool)
	// resolved keeps local's answers: it may touch the disk, and the view
	// asks every frame.
	resolved map[string]resolvedSrc
	http     *http.Client
	sem      chan struct{}

	entries map[string]*entry
	total   int
	tick    uint64
}

func newLoader(post func(func()), source func(*url.URL) *url.URL, authorize func(*url.URL) string) *loader {
	l := &loader{post: post, source: source, authorize: authorize, sem: make(chan struct{}, fetchers), entries: map[string]*entry{}}
	l.http = &http.Client{Timeout: 30 * time.Second, CheckRedirect: l.redirect}
	return l
}

// redirect keeps a credential only to the host that was asked: the
// Authorization header is set afresh for each hop and only when the hop
// stays on the previous one's host.
func (l *loader) redirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	req.Header.Del("Authorization")
	if prev := via[len(via)-1]; prev.URL.Host == req.URL.Host && prev.Header.Get("Authorization") != "" {
		if a := l.auth(req.URL); a != "" {
			req.Header.Set("Authorization", a)
		}
	}
	return nil
}

func (l *loader) auth(u *url.URL) string {
	// A credential never travels in the clear.
	if l.authorize == nil || u.Scheme != "https" {
		return ""
	}
	return l.authorize(u)
}

// key is what the loader loads src by: the URL, or the file local
// resolves it to; false leaves the image unloaded.
func (l *loader) key(src string) (string, bool) {
	if l.local == nil {
		return src, safeURL(src)
	}
	r, ok := l.resolved[src]
	if !ok {
		r.path, r.ok = l.local(src)
		if l.resolved == nil {
			l.resolved = map[string]resolvedSrc{}
		}
		l.resolved[src] = r
	}
	return r.path, r.ok
}

type resolvedSrc struct {
	path string
	ok   bool
}

// get is src's entry, fetched the first time it is asked for.
func (l *loader) get(src string) *entry {
	l.tick++
	if e, ok := l.entries[src]; ok {
		e.used = l.tick
		return e
	}
	e := &entry{used: l.tick}
	l.entries[src] = e
	go func() {
		l.sem <- struct{}{}
		bmp, w, h, animated, err := l.load(context.Background(), src)
		<-l.sem
		if err != nil {
			log.Printf("markdown: image %s: %v", redact(src), err)
		}
		l.post(func() {
			if l.entries[src] != e {
				return
			}
			if err != nil {
				e.state = imgFailed
				return
			}
			e.state, e.bmp, e.w, e.h, e.animated = imgReady, bmp, w, h, animated
			bw, bh := bmp.Size()
			e.bytes = bw * bh * 4
			l.total += e.bytes
			l.evict(e)
		})
	}()
	return e
}

// evict drops the least recently shown bitmaps over the budget, keeping
// keep.
func (l *loader) evict(keep *entry) {
	for l.total > cacheBytes {
		var oldest string
		var o *entry
		for k, e := range l.entries {
			if e != keep && e.state == imgReady && (o == nil || e.used < o.used) {
				oldest, o = k, e
			}
		}
		if o == nil {
			return
		}
		l.total -= o.bytes
		delete(l.entries, oldest)
	}
}

func (l *loader) load(ctx context.Context, src string) (bmp *ui.Bitmap, w, h float32, animated bool, err error) {
	data, err := l.fetch(ctx, src)
	if err != nil {
		return nil, 0, 0, false, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, false, err
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return nil, 0, 0, false, fmt.Errorf("%d×%d pixels is too large", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, false, err
	}
	w, h = float32(cfg.Width), float32(cfg.Height)
	if cfg.Width > maxPixelWidth {
		img = shrink(img, maxPixelWidth, max(1, cfg.Height*maxPixelWidth/cfg.Width))
	}
	return ui.NewBitmap(img), w, h, lightbox.Animated(data), nil
}

// original reads src again for the lightbox: the cache keeps a smaller
// bitmap, and an animation's first frame only.
func (l *loader) original(src string) func(ctx context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) { return l.fetch(ctx, src) }
}

func (l *loader) fetch(ctx context.Context, src string) ([]byte, error) {
	if l.local != nil {
		return readFile(src)
	}
	u, err := url.Parse(src)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("not a web URL")
	}
	if l.source != nil {
		u = l.source(u)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if a := l.auth(u); a != "" {
		req.Header.Set("Authorization", a)
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif,image/webp,image/*;q=0.8")
	resp, err := l.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s", resp.Status)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, fmt.Errorf("%d bytes is too large", resp.ContentLength)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("over %d bytes", maxImageBytes)
	}
	return data, nil
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("over %d bytes", maxImageBytes)
	}
	return data, nil
}

// shrink scales img down to w×h, averaging the source pixels each
// destination pixel covers.
func shrink(img image.Image, w, h int) image.Image {
	b := img.Bounds()
	src := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, bl, a, n uint32
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride:]
				for sx := x0; sx < x1; sx++ {
					p := row[sx*4 : sx*4+4]
					r, g, bl, a = r+uint32(p[0]), g+uint32(p[1]), bl+uint32(p[2]), a+uint32(p[3])
					n++
				}
			}
			o := dst.Pix[y*dst.Stride+x*4:]
			o[0], o[1], o[2], o[3] = uint8(r/n), uint8(g/n), uint8(bl/n), uint8(a/n)
		}
	}
	return dst
}

// redact drops a URL's query, where signed links carry their tokens.
func redact(src string) string {
	if u, err := url.Parse(src); err == nil {
		u.RawQuery = ""
		return u.String()
	}
	return "?"
}
