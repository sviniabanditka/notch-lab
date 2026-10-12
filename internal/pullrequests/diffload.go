package pullrequests

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/diffview"
)

func (v *View) dropDiff() {
	if v.stopDiff != nil {
		v.stopDiff()
		v.stopDiff = nil
	}
	if v.diff != nil {
		v.diff.Close()
		v.diff = nil
	}
	v.diffKey, v.diffErr, v.diffLoading = "", "", false
}

// loadDiff fetches and parses the diff of d's head off the main thread,
// once per head commit.
func (v *View) loadDiff(d *detail) {
	key := d.URL + "@" + d.HeadSHA
	if v.diffKey == key {
		return
	}
	v.dropDiff()
	v.diffKey, v.diffLoading = key, true
	ctx, stop := context.WithCancel(context.Background())
	v.stopDiff = stop
	r, number := d.Repo, d.Number
	go func() {
		started := time.Now()
		files, err := v.gh.files(ctx, r, number)
		var parsed *diffview.Diff
		if err == nil {
			text, missing := unifiedDiff(files)
			notes := make([]diffview.Note, len(missing))
			for i, m := range missing {
				notes[i] = diffview.Note{Path: m, Text: unavailable}
			}
			parsed, err = diffview.Parse(strings.NewReader(text), notes...)
		}
		if ctx.Err() != nil {
			return
		}
		if parsed != nil {
			n, rows := parsed.Size()
			log.Printf("pullrequests: %s#%d diff of %d files, %d rows in %v", r, number, n, rows, time.Since(started).Round(time.Millisecond))
		}
		v.post(ctx, func() {
			v.diffLoading, v.diffErr = false, errText(err)
			if parsed != nil {
				v.diff = diffview.New(parsed, v.host.Update)
			}
		})
	}()
}
