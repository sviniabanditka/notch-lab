package browser

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// The Shim's calls to the app (R-WEB-151): Chrome APIs WebKit lacks,
// answered as Search's ExtensionShims.answer does, `{"value": …}` or
// `{"error": …}`. A family the manifest never asked for is an error.

// gates are the families whose answers leave the Extension's own origin.
var gates = map[string]string{
	"bookmarks": "bookmarks", "history": "history", "downloads": "downloads",
	"sessions": "sessions", "topSites": "topSites", "idle": "idle",
	"notifications": "notifications", "tabGroups": "tabGroups", "tts": "tts", "userScripts": "userScripts",
}

var told = map[string]bool{}

type unsupported string

func (u unsupported) Error() string { return string(u) }

// answer is one call; perms are the names the manifest asked for.
func (b *Bench) answer(perms map[string]bool, self map[string]any, folder, api string, args []json.RawMessage) map[string]any {
	family, _, _ := strings.Cut(api, ".")
	if need, ok := gates[family]; ok && !perms[need] {
		return map[string]any{"error": fmt.Sprintf("The extension never asked for “%s”", need)}
	}
	v, err := b.run(self, folder, api, args)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"value": v}
}

func arg[T any](args []json.RawMessage, i int) (T, bool) {
	var v T
	if i >= len(args) || json.Unmarshal(args[i], &v) != nil {
		return v, false
	}
	return v, true
}

// bookmark ids for Chrome: "0" the root, "1" the bar.
func chromeID(bm *Bookmark) string {
	switch bm.ID {
	case "root":
		return "0"
	case "bar":
		return "1"
	}
	return bm.ID
}

func (b *Bench) byChromeID(id string) (*Bookmark, *Bookmark) {
	var walk func(f, parent *Bookmark) (*Bookmark, *Bookmark)
	walk = func(f, parent *Bookmark) (*Bookmark, *Bookmark) {
		if chromeID(f) == id {
			return f, parent
		}
		for _, c := range f.Children {
			if x, p := walk(c, f); x != nil {
				return x, p
			}
		}
		return nil, nil
	}
	return walk(b.Marks.Root, nil)
}

func node(bm, parent *Bookmark, deep bool) map[string]any {
	n := map[string]any{"id": chromeID(bm), "title": bm.Title, "dateAdded": bm.Created * 1000}
	if parent != nil {
		n["parentId"] = chromeID(parent)
		for i, c := range parent.Children {
			if c == bm {
				n["index"] = i
			}
		}
	}
	if bm.Folder || bm.Children != nil {
		if deep {
			kids := []any{}
			for _, c := range bm.Children {
				kids = append(kids, node(c, bm, true))
			}
			n["children"] = kids
		}
	} else {
		n["url"] = bm.URL
	}
	return n
}

func (b *Bench) allMarks() [][2]*Bookmark {
	var out [][2]*Bookmark
	var walk func(f *Bookmark)
	walk = func(f *Bookmark) {
		for _, c := range f.Children {
			if c.Folder {
				walk(c)
			} else {
				out = append(out, [2]*Bookmark{c, f})
			}
		}
	}
	walk(b.Marks.Root)
	return out
}

func visitOf(v Visit) map[string]any {
	return map[string]any{"id": v.URL, "url": v.URL, "title": v.Title,
		"lastVisitTime": v.At.UnixMilli(), "visitCount": v.Count, "typedCount": 0}
}

func (b *Bench) run(self map[string]any, folder, api string, args []json.RawMessage) (any, error) {
	if strings.HasPrefix(api, "userScripts.") && folder != "" {
		return userScripts(api, args, folder)
	}
	switch api {
	case "scripting.file":
		fn, _ := arg[string](args, 0)
		var a json.RawMessage
		if len(args) > 1 {
			a = args[1]
		}
		return scriptingFile(fn, a, folder)
	case "bookmarks.getTree":
		return []any{node(b.Marks.Root, nil, true)}, nil
	case "bookmarks.getSubTree", "bookmarks.get":
		id, _ := arg[string](args, 0)
		if bm, p := b.byChromeID(id); bm != nil {
			return []any{node(bm, p, api == "bookmarks.getSubTree")}, nil
		}
		return []any{}, nil
	case "bookmarks.getChildren":
		id, _ := arg[string](args, 0)
		out := []any{}
		if bm, _ := b.byChromeID(id); bm != nil {
			for _, c := range bm.Children {
				out = append(out, node(c, bm, false))
			}
		}
		return out, nil
	case "bookmarks.getRecent":
		n, ok := arg[int](args, 0)
		if !ok {
			n = 10
		}
		all := b.allMarks()
		out := []any{}
		for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
			out = append(out, node(all[i][0], all[i][1], false))
		}
		return out, nil
	case "bookmarks.search":
		q, ok := arg[string](args, 0)
		if !ok {
			spec, _ := arg[map[string]string](args, 0)
			q = spec["query"]
		}
		out := []any{}
		for _, m := range b.Marks.Match(q, 100) {
			_, p := b.Marks.Find(m.URL)
			out = append(out, node(m, p, false))
		}
		return out, nil
	case "bookmarks.create":
		spec, _ := arg[struct {
			ParentID string `json:"parentId"`
			Title    string `json:"title"`
			URL      string `json:"url"`
		}](args, 0)
		folder, _ := b.byChromeID(spec.ParentID)
		if folder == nil || !folder.Folder {
			folder, _ = b.byChromeID("1")
		}
		id := fmt.Sprintf("x%d", time.Now().UnixNano())
		var made *Bookmark
		if spec.URL != "" {
			made = b.Marks.Add(folder, spec.Title, spec.URL, id)
		} else {
			made = &Bookmark{ID: id, Title: spec.Title, Folder: true, Created: time.Now().Unix()}
			folder.Children = append(folder.Children, made)
			b.Marks.Changed()
		}
		return node(made, folder, false), nil
	case "bookmarks.remove", "bookmarks.removeTree":
		id, _ := arg[string](args, 0)
		bm, p := b.byChromeID(id)
		if bm == nil || p == nil {
			return nil, unsupported("Can't find bookmark for id.")
		}
		b.Marks.Remove(bm)
		return nil, nil

	case "history.search":
		spec, _ := arg[struct {
			Text       string  `json:"text"`
			StartTime  float64 `json:"startTime"`
			MaxResults int     `json:"maxResults"`
		}](args, 0)
		if spec.MaxResults == 0 {
			spec.MaxResults = 100
		}
		start := time.Now().Add(-24 * time.Hour)
		if spec.StartTime > 0 {
			start = time.UnixMilli(int64(spec.StartTime))
		}
		out := []any{}
		for _, v := range b.History.Recent(spec.Text, 1000) {
			if !v.At.Before(start) && len(out) < spec.MaxResults {
				out = append(out, visitOf(v))
			}
		}
		return out, nil
	case "history.addUrl":
		spec, _ := arg[map[string]string](args, 0)
		b.History.Add(spec["url"], spec["title"], SharedProfile)
		return nil, nil
	case "history.deleteUrl":
		spec, _ := arg[map[string]string](args, 0)
		b.History.Remove(spec["url"])
		return nil, nil
	case "history.deleteAll":
		b.History.Clear(time.Time{})
		return nil, nil

	case "downloads.search":
		out := []any{}
		for i, d := range b.Loads.Items {
			state := map[string]string{"running": "in_progress", "done": "complete", "failed": "interrupted"}[d.State]
			out = append(out, map[string]any{"id": i + 1, "url": d.URL, "finalUrl": d.URL, "filename": d.Path,
				"state": state, "startTime": time.Unix(d.At, 0).UTC().Format(time.RFC3339), "mime": "", "exists": true})
		}
		return out, nil

	case "idle.queryState":
		return "active", nil // ponytail: no idle reading yet; CGEventSource seconds when an Extension needs it
	case "idle.getAutoLockDelay":
		return 0, nil

	case "management.getSelf", "management.get":
		return self, nil
	case "management.getAll":
		return []any{}, nil
	case "runtime.getContexts", "clients.pages", "permissions.granted":
		return []any{}, nil
	case "permissions.remove":
		return true, nil
	case "background.loadedBefore":
		return true, nil
	case "background.wake", "background.revive", "action.popup", "worker.alive":
		return nil, nil
	// No side panels in WebKit: their settings are taken, not refused, as
	// a refusal stops the start of a worker that sets them (Bitwarden).
	case "sidePanel.setOptions", "sidePanel.setPanelBehavior":
		return nil, nil
	case "sidePanel.getOptions":
		return map[string]any{"enabled": false}, nil
	case "sidePanel.getPanelBehavior":
		return map[string]any{"openPanelOnActionClick": false}, nil
	case "debug.error":
		e, _ := arg[string](args, 0)
		log.Printf("extensions: %s: %s", self["name"], e)
		return nil, nil
	}
	if key := fmt.Sprint(self["id"], api); !told[key] { // R-WEB-152: one line per name per Extension
		told[key] = true
		log.Printf("extensions: %s: unsupported %s", self["name"], api)
	}
	return nil, unsupported("unsupported: " + api)
}
