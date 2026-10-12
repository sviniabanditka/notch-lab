package pullrequests

import (
	"testing"
	"time"
)

func TestFreshnessDecidesShowAndFetch(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	cases := []struct {
		name    string
		f       freshness
		fetched time.Time
		forced  bool
		want    cacheUse
	}{
		{"list, nothing cached", listFreshness, time.Time{}, false, cacheUse{show: false, fetch: true}},
		{"list, fresh", listFreshness, ago(59 * time.Second), false, cacheUse{show: true, fetch: false}},
		{"list, stale at 60s", listFreshness, ago(60 * time.Second), false, cacheUse{show: true, fetch: true}},
		{"list, a day old still shows", listFreshness, ago(24 * time.Hour), false, cacheUse{show: true, fetch: true}},
		{"list, fresh but forced", listFreshness, ago(5 * time.Second), true, cacheUse{show: true, fetch: true}},
		{"detail, fresh", detailFreshness, ago(44 * time.Second), false, cacheUse{show: true, fetch: false}},
		{"detail, stale", detailFreshness, ago(45 * time.Second), false, cacheUse{show: true, fetch: true}},
		{"detail, expired at 10m", detailFreshness, ago(10 * time.Minute), false, cacheUse{show: false, fetch: true}},
		{"detail, expired and forced", detailFreshness, ago(time.Hour), true, cacheUse{show: false, fetch: true}},
	}
	for _, c := range cases {
		if got := c.f.use(c.fetched, now, c.forced); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
