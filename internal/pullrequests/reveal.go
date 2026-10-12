package pullrequests

import (
	"slices"
	"sort"
)

func (v *View) loadedRows() []Row {
	if v.noAuth {
		return nil
	}
	seen := map[itemKey]bool{}
	var all []item
	for _, in := range v.inboxes {
		for _, list := range in.Lists {
			for _, it := range list {
				if !seen[it.key()] {
					seen[it.key()] = true
					all = append(all, it)
				}
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Updated.After(all[j].Updated) })
	out := make([]Row, len(all))
	for i, it := range all {
		out[i] = Row{Key: it.key().String(), Title: it.Title, Repo: it.Repo.String(), Number: it.Number, Issue: it.Issue}
	}
	return out
}

func (v *View) loaded(key string) (item, bool) {
	for _, in := range v.inboxes {
		for _, list := range in.Lists {
			for _, it := range list {
				if it.key().String() == key {
					return it, true
				}
			}
		}
	}
	return item{}, false
}

func (v *View) reveal(key string) {
	it, ok := v.loaded(key)
	if !ok {
		return
	}
	v.query = ""
	v.clearSearch()
	// A narrow view shows the detail rather than start on the list.
	v.entering, v.onList = false, false
	v.show(it)
	v.whenRepo(func() { v.choose(it.key()) })
}

// choose selects k in the first inbox scope that holds it.
func (v *View) choose(k itemKey) {
	if f, _ := v.filter(); f != "" && !holds(v.inboxes[f], k) {
		v.thisRepo = false
		v.refreshList(false)
	}
	f, _ := v.filter()
	for s := range inboxScopes {
		if in := v.inboxes[f]; in != nil && slices.ContainsFunc(in.Lists[s], keyed(k)) {
			v.scope = int(s)
			break
		}
	}
	v.clampSelection()
}

func holds(in *inbox, k itemKey) bool {
	return in != nil && slices.ContainsFunc(in.Lists[:], func(list []item) bool {
		return slices.ContainsFunc(list, keyed(k))
	})
}

func keyed(k itemKey) func(item) bool { return func(it item) bool { return it.key() == k } }

func (v *View) searchFor(query string) {
	v.query, v.onList = query, true
	v.whenRepo(v.runSearch)
}

// whenRepo runs fn once the focused workspace's repo is known, so that an
// opening's own choice of "This repo" does not undo it.
func (v *View) whenRepo(fn func()) {
	if v.repoDone {
		fn()
		return
	}
	v.pending = fn
}
