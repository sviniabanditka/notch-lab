package pullrequests

import "time"

// freshness says how long cached data is served as it is (fresh) and
// how long it may still show while a fetch replaces it (keep; 0 keeps it
// for good). Past keep it is expired and not shown.
type freshness struct{ fresh, keep time.Duration }

var (
	// Lists are long-lived: whatever is cached shows at once.
	listFreshness = freshness{fresh: 60 * time.Second}
	// A detail is short-lived: a stale one shows only while it reloads.
	detailFreshness = freshness{fresh: 45 * time.Second, keep: 10 * time.Minute}
)

// cacheUse is what to do with a cache entry: show it, fetch anew, or both
// (stale-while-revalidate).
type cacheUse struct{ show, fetch bool }

// use decides for an entry fetched at fetched (zero when there is none);
// forced is the user's Refresh, which fetches but keeps the entry shown.
func (f freshness) use(fetched, now time.Time, forced bool) cacheUse {
	if fetched.IsZero() {
		return cacheUse{fetch: true}
	}
	age := now.Sub(fetched)
	switch {
	case age < f.fresh:
		return cacheUse{show: true, fetch: forced}
	case f.keep > 0 && age >= f.keep:
		return cacheUse{fetch: true}
	}
	return cacheUse{show: true, fetch: true}
}

// maxDetails is how many details the memory cache holds: parsed bodies
// are the bulk of one.
const maxDetails = 24

type detailEntry struct {
	d       *detail
	fetched time.Time
}

// detailCache holds the details shown lately. Main thread only.
type detailCache map[itemKey]detailEntry

func (c detailCache) put(k itemKey, d *detail, now time.Time) {
	c[k] = detailEntry{d, now}
	for len(c) > maxDetails {
		var oldest itemKey
		var at time.Time
		for k, e := range c {
			if at.IsZero() || e.fetched.Before(at) {
				oldest, at = k, e.fetched
			}
		}
		delete(c, oldest)
	}
}
