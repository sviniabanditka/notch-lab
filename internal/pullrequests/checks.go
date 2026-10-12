package pullrequests

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// checksEvery is how often the checks of the pull request shown are
// asked after while some still run.
const checksEvery = 20 * time.Second

// checksChanged asks GitHub whether the checks of r's commit sha changed
// since the answers whose ETags etags holds, which it updates. A 304
// answer costs no quota. The first ask of a path only learns its ETag.
func (c *client) checksChanged(ctx context.Context, r repo, sha string, etags map[string]string) (bool, error) {
	changed := false
	for _, path := range []string{
		fmt.Sprintf("/repos/%s/commits/%s/check-runs?per_page=100", r, sha),
		fmt.Sprintf("/repos/%s/commits/%s/status", r, sha),
	} {
		h := http.Header{}
		prev, seen := etags[path]
		if prev != "" {
			h.Set("If-None-Match", prev)
		}
		resp, err := c.send(ctx, http.MethodGet, path, nil, h)
		if err != nil {
			return false, err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotModified {
			continue
		}
		etags[path] = resp.Header.Get("ETag")
		changed = changed || seen
	}
	return changed, nil
}

// pending reports whether some of d's checks still run.
func (d *detail) pending() bool {
	if d.Issue || d.State != "OPEN" {
		return false
	}
	for _, ch := range d.CheckList {
		if ch.State == checkPending {
			return true
		}
	}
	return d.Checks == checkPending
}

// watchChecks asks after the checks of d, the detail shown, every
// checksEvery while some run, the window has the focus and the quota
// allows; once they changed, d loads again, which ends or renews the
// watch.
func (v *View) watchChecks(d *detail) {
	key := d.key().String() + "@" + d.HeadSHA
	if !d.pending() {
		v.stopWatch()
		return
	}
	if v.watchKey == key {
		return
	}
	v.stopWatch()
	ctx, stop := context.WithCancel(context.Background())
	v.watchKey, v.stopChecks = key, stop
	r, sha, k := d.Repo, d.HeadSHA, d.key()
	go func() {
		etags := map[string]string{}
		tick := time.NewTicker(checksEvery)
		defer tick.Stop()
		for first := true; ; first = false {
			if !first {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
			}
			if _, ok := v.gh.guard.background("core", time.Now()); !ok || !v.focused.Load() {
				continue
			}
			changed, err := v.gh.checksChanged(ctx, r, sha, etags)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Printf("pullrequests: %s checks: %v", k, err)
				continue
			}
			if changed {
				log.Printf("pullrequests: %s checks changed", k)
				v.post(ctx, func() {
					if v.shown == k {
						v.loadDetail(v.shownItem, true)
					}
				})
			}
		}
	}()
}

func (v *View) stopWatch() {
	if v.stopChecks != nil {
		v.stopChecks()
		v.stopChecks = nil
	}
	v.watchKey = ""
}
