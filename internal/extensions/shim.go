package extensions

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The Shim is Search's search-shim.js (Office Commun, MIT), kept verbatim
// but for the names below (R-WEB-150).
//
//go:embed js/search-shim.js
var searchShim string

const (
	// Application is the native application the Shim's messages go to.
	Application = "notchlab"
	ShimFile    = "notch-shim.js"
	// PasskeysFile stands in for Search's passkey relay, which notch-lab
	// does not carry yet: an empty script, so content scripts that name it
	// still load. ponytail: no passkey bridge; port Search's Passkeys.swift
	// when a password manager needs it.
	PasskeysFile = "notch-passkeys.js"
	Marker       = "/* notch-lab: Chrome APIs WebKit lacks, filled in */"
	Ender        = "/* notch-lab: end of shim */"
	stampFile    = ".notch-shim"
	addedFile    = ".notch-added"
)

var shimText = strings.NewReplacer(
	`sendNativeMessage("search",`, `sendNativeMessage("`+Application+`",`,
	`connectNative("search")`, `connectNative("`+Application+`")`,
	`connectNative("search.socket")`, `connectNative("`+Application+`.socket")`,
	"search-passkeys.js", PasskeysFile,
).Replace(searchShim)

// ShimVersion is the first 8 bytes of the Shim's SHA-256 as hex.
var ShimVersion = func() string {
	h := sha256.Sum256([]byte(shimText))
	return hex.EncodeToString(h[:8])
}()

var eventRef = regexp.MustCompile(`\.([a-zA-Z]+)\.(on[A-Z][A-Za-z]+)\b`)

// shimFor fills the Shim's placeholders for one Extension: the events its
// code mentions and the scripts it ships.
func shimFor(folder string) string {
	found := map[string]bool{}
	var scripts []string
	_ = filepath.Walk(folder, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(p) != ".js" {
			return nil
		}
		data, _ := os.ReadFile(p)
		text := string(data)
		rel := strings.TrimPrefix(p, folder)
		if strings.TrimSpace(text) == "" {
			scripts = append(scripts, "-"+rel)
		} else {
			scripts = append(scripts, rel)
		}
		if filepath.Base(p) == ShimFile {
			return nil
		}
		if strings.HasPrefix(text, Marker) {
			if i := strings.Index(text, Ender); i >= 0 {
				text = text[i+len(Ender):]
			}
		}
		for _, m := range eventRef.FindAllStringSubmatch(text, -1) {
			found[m[1]+"."+m[2]] = true
		}
		return nil
	})
	events := make([]string, 0, len(found))
	for e := range found {
		events = append(events, e)
	}
	sort.Strings(events)
	sort.Strings(scripts)
	ev, _ := json.Marshal(events)
	sc, _ := json.Marshal(scripts)
	return strings.NewReplacer("__SEARCH_EVENTS__", string(ev), "__SEARCH_SCRIPTS__", string(sc),
		"__SEARCH_CHROME__", ChromeVersion, "__SEARCH_VERBOSE__", "false").Replace(shimText)
}

// inside resolves a path a manifest names and keeps it in its folder; a
// link or a way out gives "".
func inside(name, folder string) string {
	p := filepath.Clean(filepath.Join(folder, strings.Trim(name, "/")))
	if !strings.HasPrefix(p, filepath.Clean(folder)+"/") {
		return ""
	}
	if st, err := os.Lstat(p); err != nil || st.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	real, err := filepath.EvalSymlinks(p)
	rf, err2 := filepath.EvalSymlinks(folder)
	if err != nil || err2 != nil || !strings.HasPrefix(real, rf+"/") {
		return ""
	}
	return p
}

var headTag = regexp.MustCompile(`(?i)<head[^>]*>`)

// Prepare puts the Shim in an Extension's folder, as Search's
// ExtensionShims.prepare does (R-WEB-150). fresh marks a package just
// unpacked: names only notch-lab writes are dropped first. A folder
// stamped with this Shim version is left as it is.
func Prepare(folder string, fresh bool) error {
	if fresh {
		os.Remove(filepath.Join(folder, stampFile))
		os.Remove(filepath.Join(folder, addedFile))
	}
	if data, err := os.ReadFile(filepath.Join(folder, stampFile)); err == nil && string(data) == ShimVersion {
		return nil
	}
	mpath := filepath.Join(folder, "manifest.json")
	raw, err := os.ReadFile(mpath)
	if err != nil {
		return ErrUnpack
	}
	var manifest map[string]any
	if err := json.Unmarshal(stripBOM(raw), &manifest); err != nil {
		return ErrUnpack
	}
	script := shimFor(folder)
	if err := os.WriteFile(filepath.Join(folder, ShimFile), []byte(script), 0o600); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(folder, PasskeysFile), []byte("/* notch-lab: passkey relay not carried */\n"), 0o600)

	// nativeMessaging reaches the app; scripting carries userScripts.
	perms, _ := manifest["permissions"].([]any)
	asked := map[string]bool{}
	for _, p := range perms {
		if s, ok := p.(string); ok {
			asked[s] = true
		}
	}
	var added []string
	if data, err := os.ReadFile(filepath.Join(folder, addedFile)); err == nil {
		_ = json.Unmarshal(data, &added)
	}
	need := []string{"nativeMessaging"}
	if asked["userScripts"] {
		need = append(need, "scripting")
	}
	for _, n := range need {
		if !asked[n] {
			perms = append(perms, n)
			added = append(added, n)
		}
	}
	manifest["permissions"] = perms
	seen := map[string]bool{}
	var uniq []string
	for _, a := range added {
		if !seen[a] {
			seen[a] = true
			uniq = append(uniq, a)
		}
	}
	sort.Strings(uniq)
	ad, _ := json.Marshal(uniq)
	_ = os.WriteFile(filepath.Join(folder, addedFile), ad, 0o600)

	if bg, ok := manifest["background"].(map[string]any); ok {
		if worker, ok := bg["service_worker"].(string); ok {
			if p := inside(worker, folder); p != "" {
				if data, err := os.ReadFile(p); err == nil {
					source := string(data)
					if strings.HasPrefix(source, Marker) {
						if i := strings.Index(source, Ender); i >= 0 {
							source = strings.TrimPrefix(source[i+len(Ender):], "\n")
						}
					}
					first := `import "/` + ShimFile + "\";\n"
					for strings.HasPrefix(source, first) {
						source = source[len(first):]
					}
					out := Marker + "\n" + script + "\n" + Ender + "\n" + source
					if t, _ := bg["type"].(string); t == "module" {
						out = first + source
					}
					if err := os.WriteFile(p, []byte(out), 0o600); err != nil {
						return err
					}
				}
			}
		}
		if scripts, ok := bg["scripts"].([]any); ok {
			if len(scripts) == 0 || scripts[0] != ShimFile {
				bg["scripts"] = append([]any{ShimFile}, scripts...)
			}
		}
		manifest["background"] = bg
	}
	if entries, ok := manifest["content_scripts"].([]any); ok {
		for _, e := range entries {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			js, ok := entry["js"].([]any)
			if !ok {
				continue
			}
			has := func(name string) bool {
				for _, x := range js {
					if x == name {
						return true
					}
				}
				return false
			}
			if !has(ShimFile) {
				js = append([]any{ShimFile}, js...)
			}
			if w, _ := entry["world"].(string); strings.EqualFold(w, "MAIN") && !has(PasskeysFile) {
				js = append([]any{PasskeysFile}, js...)
			}
			entry["js"] = js
		}
	}
	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(mpath, out, 0o600); err != nil {
		return err
	}
	tag := `<script src="/` + ShimFile + `"></script>`
	_ = filepath.Walk(folder, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
			return nil
		}
		if ext := strings.ToLower(filepath.Ext(p)); ext != ".html" && ext != ".htm" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || strings.Contains(string(data), ShimFile) {
			return nil
		}
		html := string(data)
		if loc := headTag.FindStringIndex(html); loc != nil {
			html = html[:loc[1]] + tag + html[loc[1]:]
		} else {
			html = tag + html
		}
		return os.WriteFile(p, []byte(html), 0o600)
	})
	return os.WriteFile(filepath.Join(folder, stampFile), []byte(ShimVersion), 0o600)
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf {
		return b[3:]
	}
	return b
}

// Added is what Prepare added to the manifest's permissions, which an
// update's permission diff leaves out (R-WEB-171).
func Added(folder string) []string {
	var added []string
	if data, err := os.ReadFile(filepath.Join(folder, addedFile)); err == nil {
		_ = json.Unmarshal(data, &added)
	}
	return added
}

var errNoManifest = errors.New("no manifest")
