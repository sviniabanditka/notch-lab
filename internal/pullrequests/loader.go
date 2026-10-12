package pullrequests

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
)

const (
	// tickEvery is how often an open view checks whether its lists or
	// detail went stale; the freshness decides whether that fetches.
	tickEvery = 10 * time.Second
	// foldLines is about how much of a long body shows before "Show more".
	foldLines = 45
)

// cwdOf is the folder the view runs in.
func cwdOf(st *State) string {
	if st == nil {
		return ""
	}
	return st.Cwd
}

func (v *View) openFor(st *State) {
	if v.open {
		return
	}
	v.open, v.focusList, v.entering, v.openedAt, v.firstList = true, true, true, time.Now(), false
	v.focused.Store(v.host.WindowFocused())
	ctx, stop := context.WithCancel(context.Background())
	v.stop = stop
	moved := false
	if cwd := cwdOf(st); cwd != v.cwd || !v.repoDone {
		moved = cwd != v.cwd || !v.repoDone
		v.cwd, v.repo, v.repoDone = cwd, repo{}, false
	}
	cwd := v.cwd
	if v.repoDone {
		v.refreshList(false)
		v.refreshShown(false)
	} else {
		go func() {
			r, ok := repoAt(ctx, cwd)
			v.post(ctx, func() {
				v.repo, v.repoDone = r, true
				if moved {
					// "This repo" is on by default where there is one.
					v.thisRepo = ok
					v.clampSelection()
				}
				v.refreshList(false)
				v.refreshShown(false)
				if p := v.pending; p != nil {
					v.pending = nil
					p()
				}
			})
		}()
	}
	go func() {
		tick := time.NewTicker(tickEvery)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				v.post(ctx, v.tick)
			}
		}
	}()
}

// tick refreshes what went stale, in the background: only while the
// window has the focus and the quota is above its reserve. It also
// redraws "updated … ago".
func (v *View) tick() {
	focused := v.host.WindowFocused()
	v.focused.Store(focused)
	if !focused || !v.repoDone {
		return
	}
	if _, ok := v.gh.guard.background("graphql", time.Now()); !ok {
		return
	}
	v.refreshList(false)
	v.refreshShown(false)
}

func (v *View) close() {
	if !v.open {
		return
	}
	v.open, v.pending = false, nil
	v.stop()
	v.cancelDetail()
	v.stopWatch()
	// The diff is the view's bulk; it loads again on the next opening.
	v.dropDiff()
}

// post hands fn to the main thread unless ctx ended first.
func (v *View) post(ctx context.Context, fn func()) {
	v.host.Update(func() {
		if ctx.Err() == nil {
			fn()
		}
	})
}

// filter is the key of the lists shown: owner/name with "This repo" on,
// "" across repos.
func (v *View) filter() (string, repo) {
	if v.thisRepo && v.repo.owner != "" {
		return v.repo.String(), v.repo
	}
	return "", repo{}
}

// refresh is the user's Refresh: the lists, the search shown and the
// detail shown fetch anew; what shows stays until they answer.
func (v *View) refresh() {
	v.refreshList(true)
	if v.scope == int(searchScope) && v.found.text != "" {
		v.runSearch()
	}
	v.refreshShown(true)
}

// refreshList fetches the lists of the filter unless they are fresh or
// forced is false and their fetch would be background, which the rate
// guard may hold back.
func (v *View) refreshList(forced bool) {
	f, only := v.filter()
	in := v.inboxes[f]
	var fetched time.Time
	if in != nil {
		fetched = in.Fetched
	}
	if !listFreshness.use(fetched, time.Now(), forced).fetch || v.fetching[f] || !v.open {
		return
	}
	if in != nil && !forced {
		if _, ok := v.gh.guard.background("graphql", time.Now()); !ok {
			return
		}
	}
	v.fetching[f] = true
	ctx := context.Background()
	go func() {
		started := time.Now()
		got, err := v.gh.fetchInbox(ctx, only)
		account := ""
		if err == nil {
			account, err = v.gh.account(ctx)
		}
		n := 0
		if got != nil {
			for _, l := range got.Lists {
				n += len(l)
			}
		}
		log.Printf("pullrequests: lists %q: %d rows in %v, err %v", f, n, time.Since(started).Round(time.Millisecond), err)
		v.host.Update(func() {
			v.fetching[f] = false
			v.noAuth = errors.Is(err, errNoToken)
			if err != nil {
				v.listErr[f] = errText(err)
				return
			}
			delete(v.listErr, f)
			if account != v.account {
				// Another login: the lists held are not this user's.
				v.inboxes, v.account = map[string]*inbox{}, account
			}
			v.inboxes[f] = got
			v.clampSelection()
			v.saveSnapshot()
		})
	}()
}

// saveSnapshot writes the lists to disk off the main thread.
func (v *View) saveSnapshot() {
	s := snapshot{Account: v.account, Inboxes: make(map[string]*inbox, len(v.inboxes))}
	for k, in := range v.inboxes {
		s.Inboxes[k] = in
	}
	go func() {
		v.saveMu.Lock()
		defer v.saveMu.Unlock()
		if err := saveSnapshot(v.dir, s); err != nil {
			log.Printf("pullrequests: %s: %v", snapshotPath, err)
		}
	}()
}

// invalidate marks every list stale after the user changed it on GitHub,
// and fetches the lists shown.
func (v *View) invalidate() {
	for k, in := range v.inboxes {
		cp := *in
		cp.Fetched = in.Fetched.Add(-listFreshness.fresh)
		v.inboxes[k] = &cp
	}
	v.refreshList(true)
}

// runSearch runs the search field's text on GitHub and shows what it
// finds in the Search scope.
func (v *View) runSearch() {
	_, only := v.filter()
	q := searchQuery(v.query, only)
	if q == "" {
		return
	}
	text := v.query
	v.found = searchState{text: text, rows: v.found.rows, loading: true}
	v.scope = int(searchScope)
	v.clampSelection()
	go func() {
		started := time.Now()
		rows, err := v.gh.search(context.Background(), q)
		log.Printf("pullrequests: search %q: %d rows in %v, err %v", q, len(rows), time.Since(started).Round(time.Millisecond), err)
		v.host.Update(func() {
			if v.found.text != text {
				return
			}
			v.found = searchState{text: text, rows: rows, err: errText(err)}
			if err != nil {
				v.found.rows = nil
			}
			v.clampSelection()
		})
	}()
}

// clearSearch drops the search results once the field is empty.
func (v *View) clearSearch() {
	v.found = searchState{}
	if v.scope == int(searchScope) {
		v.scope = int(reviewRequested)
	}
	v.clampSelection()
}

func (v *View) cancelDetail() {
	if v.stopDetail != nil {
		v.stopDetail()
		v.stopDetail = nil
	}
	v.detailBusy = false
}

// refreshShown loads the item shown again if its detail went stale, or
// forced.
func (v *View) refreshShown(forced bool) {
	if v.shown != (itemKey{}) {
		v.loadDetail(v.shownItem, forced)
	}
}

// loadDetail shows it from the cache when its detail may show, and
// fetches it when that is stale, missing or forced; a later call drops
// this one's result.
func (v *View) loadDetail(it item, forced bool) {
	k := it.key()
	e := v.details[k]
	use := detailFreshness.use(e.fetched, time.Now(), forced)
	if use.show && v.shown == k {
		v.det, v.detErr = e.d, ""
		v.watchChecks(e.d)
	}
	if !use.fetch {
		return
	}
	v.cancelDetail()
	ctx, stop := context.WithCancel(context.Background())
	v.stopDetail, v.detailBusy = stop, true
	go func() {
		started := time.Now()
		var d *detail
		var err error
		if it.Issue {
			d, err = v.gh.issueDetail(ctx, it.Repo, it.Number)
		} else {
			d, err = v.gh.detail(ctx, it.Repo, it.Number)
		}
		if err == nil {
			v.parseBodies(d)
		}
		if ctx.Err() != nil {
			return
		}
		log.Printf("pullrequests: %s detail in %v, err %v", k, time.Since(started).Round(time.Millisecond), err)
		v.post(ctx, func() {
			v.detailBusy = false
			if err == nil {
				v.details.put(k, d, time.Now())
				v.noteChecks(d)
			}
			if v.shown != k {
				return
			}
			v.detErr = errText(err)
			if err == nil {
				v.det = d
				v.watchChecks(d)
			}
		})
	}()
}

// parseBodies parses d's description and comments as markdown, off the
// main thread, once per load.
func (v *View) parseBodies(d *detail) {
	o := markdown.Options{Refs: githubRefs(d.Repo), Images: v.images, Zoom: v.host.Zoom, NewlineBreaks: true, Fold: foldLines}
	d.BodyDoc = markdown.Parse(d.Body, o)
	for i := range d.Comments {
		d.Comments[i].Doc = markdown.Parse(d.Comments[i].Body, o)
	}
	for i := range d.Threads {
		t := &d.Threads[i]
		to := o
		if t.Replaced != nil {
			to.Suggestion = &markdown.Suggestion{Replaced: t.Replaced}
		}
		for j := range t.Comments {
			t.Comments[j].Doc = markdown.Parse(t.Comments[j].Body, to)
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
