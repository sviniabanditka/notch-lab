package pullrequests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSnapshotRoundTripHoldsRowsButNotTheToken(t *testing.T) {
	const token = "gho_SecretToken123"
	dir := t.TempDir()
	fetched := time.Date(2026, 10, 9, 11, 58, 0, 0, time.UTC)
	in := &inbox{Fetched: fetched}
	in.Lists[reviewRequested] = []item{{Repo: repo{"DiVotek", "tbtop"}, Number: 412, Title: "Cache tables",
		Author: "octo", URL: "https://github.com/DiVotek/tbtop/pull/412", Draft: true, Checks: checkFailed, Updated: fetched}}
	in.Lists[issues] = []item{{Repo: repo{"maxboiko", "herdr-go"}, Issue: true, Number: 7, Title: "Crash on resize",
		URL: "https://github.com/maxboiko/herdr-go/issues/7", Updated: fetched}}
	saved := snapshot{Account: fingerprint(token), Inboxes: map[string]*inbox{"": in}}

	if err := saveSnapshot(dir, saved); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "cache", "pullrequests.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) || strings.Contains(string(data), "SecretToken") {
		t.Fatalf("snapshot holds the token: %s", data)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("snapshot mode %v, want 0600", st.Mode().Perm())
	}

	got, err := loadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Account != fingerprint(token) || got.Account == fingerprint("gho_other") {
		t.Errorf("account %q does not tell this login from another", got.Account)
	}
	if !reflect.DeepEqual(got.Inboxes[""].Lists, in.Lists) || !got.Inboxes[""].Fetched.Equal(fetched) {
		t.Errorf("rows came back as %+v", got.Inboxes[""])
	}
}
