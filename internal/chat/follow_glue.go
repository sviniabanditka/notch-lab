package chat

import (
	"time"

	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

// sessionEvery is how often an idle or done chat asks herdr which session its
// agent runs: /clear and /resume change no status, so only asking catches them.
const sessionEvery = time.Second

// follow hands the Conversations of one transcript to the main thread, each
// grouped and rendered off it, until the chat switches session or closes.
func (v *View) follow(p *paneChat, f *transcript.Follower, folder string) {
	open := func(t string) {
		file, line := target(t)
		v.host.OpenFileAt(file, folder, line)
	}
	cache := newRenderCache(fileLinks{folder: folder, home: homeDir()}, open)
	go func() {
		for c := range f.Updates() {
			tl := group(c)
			cache.render(tl)
			v.host.Update(func() {
				if p.follow == f && v.panes[p.id] == p {
					p.take(tl, c)
				}
			})
		}
	}()
}

// watchSession asks herdr for the pane's session every sessionEvery while
// the agent is idle or done, until the chat closes.
func (v *View) watchSession(p *paneChat) {
	go func() {
		t := time.NewTicker(sessionEvery)
		defer t.Stop()
		for {
			select {
			case <-p.closed:
				return
			case <-t.C:
				v.host.Update(func() {
					if v.panes[p.id] == p && p.isAtRest() && p.asking == 0 {
						v.lookup(p)
					}
				})
			}
		}
	}()
}

// renderCache keeps what items render to across Conversations, by item: an
// answer's parsed text, a prompt's thumbnails. Only the render goroutine of
// one follow uses it.
type renderCache struct {
	links  fileLinks
	open   func(target string)
	docs   map[string]cachedDoc
	thumbs map[string]thumbs
}

type cachedDoc struct {
	text string
	doc  *markdown.Doc
}

func newRenderCache(links fileLinks, open func(string)) *renderCache {
	return &renderCache{links: links, open: open, docs: map[string]cachedDoc{}, thumbs: map[string]thumbs{}}
}

func (rc *renderCache) render(tl *turnList) {
	seen := map[string]bool{}
	for _, t := range tl.turns {
		items := t.body
		if t.opener != nil {
			items = append([]*item{t.opener}, items...)
		}
		for _, it := range items {
			seen[it.key] = true
			switch {
			case it.kind == itemAnswer:
				d, ok := rc.docs[it.key]
				if !ok || d.text != it.text {
					d = cachedDoc{it.text, markdown.Parse(it.text, markdown.Options{
						Paths: rc.links.paths, Open: rc.open})}
					rc.docs[it.key] = d
				}
				it.doc = d.doc
			case len(it.images) > 0:
				tb, ok := rc.thumbs[it.key]
				if !ok {
					tb = decodeThumbs(it.images)
					rc.thumbs[it.key] = tb
				}
				it.thumbs = tb
			}
		}
	}
	for k := range rc.docs {
		if !seen[k] {
			delete(rc.docs, k)
		}
	}
	for k := range rc.thumbs {
		if !seen[k] {
			delete(rc.thumbs, k)
		}
	}
}
