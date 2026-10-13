package browser

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/webkit"
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
	case "runtime.getContexts":
		return []any{}, nil
	case "background.loadedBefore":
		return true, nil
	case "background.wake", "background.revive", "worker.alive":
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

// manifest is what Ext reads of an Extension's manifest.json.
type manifest struct {
	Permissions, Optional []string
}

// readManifest reads the names of a manifest's permissions, required and
// optional; a name that is not a string is left out.
func readManifest(folder string) manifest {
	var raw struct {
		Permissions []any `json:"permissions"`
		Optional    []any `json:"optional_permissions"`
	}
	if b, err := os.ReadFile(filepath.Join(folder, "manifest.json")); err == nil {
		_ = json.Unmarshal(b, &raw)
	}
	names := func(list []any) []string {
		var out []string
		for _, p := range list {
			if s, ok := p.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return manifest{names(raw.Permissions), names(raw.Optional)}
}

// granted is what permissions.request granted an Extension, of the
// permissions the app answers itself.
func (e *Ext) granted(id string) []string {
	if en := e.M.Get(id); en != nil {
		return en.Granted
	}
	return nil
}

func (e *Ext) setGranted(id string, list []string) {
	en := e.M.Get(id)
	if en == nil {
		return
	}
	list = slices.Clone(list)
	sort.Strings(list)
	en.Granted = slices.Compact(list)
	e.M.Save()
}

// silent are the permissions Chrome grants without a question.
var silent = map[string]bool{"tabGroups": true, "sidePanel": true, "offscreen": true, "idle": true, "power": true,
	"fontSettings": true, "search": true, "system.cpu": true, "system.memory": true, "system.display": true, "favicon": true}

// located is the Tab at a place the Shim gives: an index in the window's
// row, alone or as {"i": index, "w": frame}. There is one window.
func (e *Ext) located(place json.RawMessage) *tab {
	var i int
	if json.Unmarshal(place, &i) != nil {
		var spec struct {
			I *int `json:"i"`
		}
		if json.Unmarshal(place, &spec) != nil || spec.I == nil {
			return nil
		}
		i = *spec.I
	}
	row := e.b.ordered()
	if i < 0 || i >= len(row) {
		return nil
	}
	return row[i]
}

// extensionHost is the Extension id of an Extension page's address, or "".
func extensionHost(u string) string {
	p, err := url.Parse(u)
	if err != nil || (strings.ToLower(p.Scheme) != "chrome-extension" && strings.ToLower(p.Scheme) != "webkit-extension") {
		return ""
	}
	return strings.ToLower(p.Host)
}

// popupURL is the page of the button's popup now: the one the Extension
// set for the active Tab or for all, else its manifest's; "" for none.
func (e *Ext) popupURL(cx *webkit.Context) string {
	set := e.popups[cx.ID]
	path, ok := "", false
	if f := e.b.focus; f != nil {
		path, ok = set[f.t.ID]
	}
	if !ok {
		path, ok = set["*"]
	}
	if !ok {
		return cx.PopupURL()
	}
	if path == "" {
		return ""
	}
	base, _ := url.Parse("chrome-extension://" + cx.ID + "/")
	ref, err := url.Parse(path)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// own answers the calls that need the Extension's context, its Tabs or a
// question to the user, as Search's ExtensionShims.answer; false leaves
// the call to Bench.answer.
func (e *Ext) own(cx *webkit.Context, mf manifest, perms map[string]bool, api string, args []json.RawMessage, reply func(any)) bool {
	ok := func(v any) { reply(map[string]any{"value": v}) }
	fail := func(s string) { reply(map[string]any{"error": s}) }
	family, _, _ := strings.Cut(api, ".")
	if need, gated := gates[family]; gated && !perms[need] {
		return false // Bench.answer refuses it
	}
	places, _ := arg[[]json.RawMessage](args, 0)
	switch api {
	// No screen capture here: the picker is cancelled, as in Chrome.
	case "capture.grant":
		ok("")
	// No offscreen documents here: nothing handles the relay.
	case "offscreen.sendMessage":
		ok(map[string]any{"handled": false})

	case "permissions.granted":
		ok(append([]string{}, e.granted(cx.ID)...))
	case "permissions.remove":
		gone, _ := arg[[]string](args, 0)
		var keep []string
		for _, p := range e.granted(cx.ID) {
			if !slices.Contains(gone, p) {
				keep = append(keep, p)
			}
		}
		e.setGranted(cx.ID, keep)
		ok(true)
	case "permissions.request":
		// Only what the manifest named, as a permission or an optional one.
		wanted, _ := arg[[]string](args, 0)
		named := append(append([]string{}, mf.Permissions...), mf.Optional...)
		quiet := true
		for _, w := range wanted {
			if !slices.Contains(named, w) {
				fail("Only permissions specified in the manifest may be requested.")
				return true
			}
			quiet = quiet && silent[w]
		}
		grant := func(yes bool) {
			if !yes {
				ok(false)
				return
			}
			e.setGranted(cx.ID, append(e.granted(cx.ID), wanted...))
			ok(true)
		}
		if quiet {
			grant(true)
			return true
		}
		var names []string
		for _, w := range wanted {
			names = append(names, strings.ReplaceAll(w, ".", " "))
		}
		e.ask(cx.Name+" asks for more access\n"+strings.Join(names, ", "), grant)
	case "permissions.afterClick":
		// permissions.request for WebKit's own permissions and sites, from
		// a press of the button whose moment WebKit lost: within 10 s of
		// the press, once, and only what the manifest names.
		if at, pressed := e.clicked[cx.ID]; !pressed || time.Since(at) >= 10*time.Second {
			fail("Invalid call to permissions.request(). Must be called during a user gesture.")
			return true
		}
		delete(e.clicked, cx.ID)
		wanted, _ := arg[[]string](args, 0)
		origins, _ := arg[[]string](args, 1)
		missing, unreached, every, named := cx.Unmet(wanted, origins)
		if !named {
			fail("Only permissions specified in the manifest may be requested.")
			return true
		}
		if len(missing) == 0 && len(unreached) == 0 {
			ok(true)
			return true
		}
		sort.Strings(missing)
		sort.Strings(unreached)
		sites := strings.Join(unreached, ", ")
		if every {
			sites = "every website"
		}
		question := "asks for more access"
		if len(unreached) > 0 {
			question = "wants to read and change " + sites
		}
		detail := "Until you remove the extension."
		if len(missing) > 0 {
			detail = strings.Join(missing, ", ")
		}
		e.ask(cx.Name+" "+question+"\n"+detail, func(yes bool) {
			if yes {
				cx.Grant(missing, unreached)
			}
			ok(yes)
		})

	// Tabs, by their place in the window's row.
	case "tabs.describe":
		if !cx.HasPermission("tabs") {
			fail("The extension never asked for “tabs”")
			return true
		}
		out := []any{}
		for _, place := range places {
			x := e.located(place)
			if x == nil {
				out = append(out, nil)
				continue
			}
			d := readData(x.t)
			// Another Extension's page stays blank; its own are its own.
			if h := extensionHost(d.URL); h != "" && h != strings.ToLower(cx.ID) {
				out = append(out, map[string]any{"url": "", "title": ""})
				continue
			}
			info := map[string]any{"url": d.URL, "title": x.t.Title}
			if d.Favicon != "" {
				info["favIconUrl"] = d.Favicon
			}
			out = append(out, info)
		}
		ok(out)
	case "tabs.activate", "tabs.discard":
		x := e.located(firstArg(args))
		if x == nil {
			fail("No tab there")
			return true
		}
		if api == "tabs.activate" {
			e.b.Show(x.t)
		} else if x != e.b.focus {
			e.b.sleep(x)
		}
		ok(nil)

	// No tab groups here: answered as Search answers with groups off.
	case "tabs.groups":
		out := []int{}
		for range places {
			out = append(out, -1)
		}
		ok(out)
	case "tabGroups.query":
		ok([]any{})
	case "tabGroups.get", "tabGroups.update":
		n := "none"
		if i, isInt := arg[int](args, 0); isInt {
			n = fmt.Sprint(i)
		}
		fail("No group with id: " + n + ".")
	case "tabGroups.move":
		fail("notch-lab can't move a tab group for an extension")
	case "tabs.group":
		fail("notch-lab has no tab groups")
	case "tabs.ungroup":
		ok(nil)

	// Its own pages, for clients.matchAll(): the popup and its Tabs.
	case "clients.pages":
		pages := []any{}
		if e.popupTab != nil {
			if h, is := e.popupTab.Host.(popupHost); is && h.id == cx.ID && !h.wv.IsDestroyed() {
				up := e.popup == h.fly
				pages = append(pages, map[string]any{"id": "popup", "url": h.URL(), "visible": up, "focused": up && h.fly.IsFocused()})
			}
		}
		for _, x := range e.b.ordered() {
			if x.wv == nil {
				continue
			}
			u := liveURL(x)
			if extensionHost(u) != strings.ToLower(cx.ID) {
				continue
			}
			w := x.wv.Window()
			shown := x == e.b.focus && w != nil && w.IsVisible()
			pages = append(pages, map[string]any{"id": x.t.ID, "url": u, "visible": shown, "focused": shown && w.IsFocused()})
		}
		ok(pages)

	// The page the button's popup is now (action.setPopup).
	case "action.popup":
		path, _ := arg[string](args, 0)
		i, isInt := arg[int](args, 1)
		row := e.b.ordered()
		if isInt && i >= 0 && i < len(row) {
			if e.popups[cx.ID] == nil {
				e.popups[cx.ID] = map[string]string{}
			}
			e.popups[cx.ID][row[i].t.ID] = path
		} else {
			e.popups[cx.ID] = map[string]string{"*": path}
		}
		ok(nil)
	default:
		return false
	}
	return true
}

func firstArg(args []json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}
