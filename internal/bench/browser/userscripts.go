package browser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Files the Shim injects for chrome.scripting and chrome.userScripts, as
// Search's ExtensionShims writes them, under <folder>/_notch.

const ownDir = "_notch"

// inside resolves an Extension's file name in its folder, or "".
func inside(name, folder string) string {
	p := filepath.Join(folder, filepath.FromSlash(strings.TrimPrefix(name, "/")))
	if rel, err := filepath.Rel(folder, p); err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	if st, err := os.Lstat(p); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	return p
}

// written writes text once under a name made from its hash, drops older
// files of the prefix after a minute, and returns the name for WebKit.
func written(folder, prefix, text string) (string, error) {
	sum := sha256.Sum256([]byte(text))
	name := prefix + hex.EncodeToString(sum[:8]) + ".js"
	dir := filepath.Join(folder, ownDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err != nil {
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			return "", err
		}
	}
	return ownDir + "/" + name, nil
}

func sweep(folder, prefix string, keep map[string]bool) {
	dir := filepath.Join(folder, ownDir)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) || keep[e.Name()] {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Minute {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// scriptingFile is executeScript's function with its arguments, as a file.
func scriptingFile(function string, args json.RawMessage, folder string) (string, error) {
	if function == "" || len(function) > 1_000_000 {
		return "", unsupported("Invalid script function or arguments")
	}
	if len(args) == 0 {
		args = json.RawMessage("[]")
	}
	name, err := written(folder, "script-", "("+function+")(..."+string(args)+")\n")
	if err == nil {
		sweep(folder, "script-", map[string]bool{filepath.Base(name): true})
	}
	return name, err
}

const userPrelude = `  const chrome = (() => {
    const runtime = globalThis.chrome.runtime;
    return { runtime: {
      id: runtime.id, getURL: (path) => runtime.getURL(path), get lastError() { return runtime.lastError; },
      sendMessage: (message, ...rest) => runtime.sendMessage({ __searchUserScript: true, message }, ...rest.filter((r) => typeof r === "function" || (r && typeof r === "object"))),
      connect: (info) => runtime.connect({ ...(info || {}), name: "search-us:" + ((info && info.name) || "") }),
    } };
  })();
  const browser = chrome;
`

type userScript struct {
	JS []struct {
		Code *string `json:"code"`
		File string  `json:"file"`
	} `json:"js"`
	World        string          `json:"world"`
	IncludeGlobs json.RawMessage `json:"includeGlobs"`
	ExcludeGlobs json.RawMessage `json:"excludeGlobs"`
}

// userScriptFile is one chrome.userScripts script as a file: its code
// inside a block that leaves on pages its globs rule out.
func userScriptFile(raw json.RawMessage, folder string) (string, error) {
	var s userScript
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	code := ""
	for _, src := range s.JS {
		if src.Code != nil {
			code += *src.Code + "\n;\n"
		} else if p := inside(src.File, folder); p != "" {
			if b, err := os.ReadFile(p); err == nil {
				code += string(b) + "\n;\n"
			}
		}
	}
	globs := func(g json.RawMessage) string {
		if len(g) == 0 || string(g) == "null" {
			return "[]"
		}
		return string(g)
	}
	prelude := ""
	if s.World != "MAIN" {
		prelude = userPrelude
	}
	text := "/* notch-lab: a user script (chrome.userScripts) */\nsearch_user_script: {\n" +
		"  const __searchHref = location.href;\n" +
		"  const __searchGlob = (g) => new RegExp(\"^\" + g.replace(/[.+^${}()|[\\]\\\\]/g, \"\\\\$&\").replace(/\\*/g, \".*\").replace(/\\?/g, \".\") + \"$\");\n" +
		"  const __searchIn = " + globs(s.IncludeGlobs) + ", __searchOut = " + globs(s.ExcludeGlobs) + ";\n" +
		"  if ((__searchIn.length && !__searchIn.some((g) => __searchGlob(g).test(__searchHref))) || __searchOut.some((g) => __searchGlob(g).test(__searchHref))) break search_user_script;\n" +
		prelude + code + "}\n"
	return written(folder, "us-", text)
}

// userScripts answers userScripts.* (list, save, file, world, worlds).
func userScripts(api string, args []json.RawMessage, folder string) (any, error) {
	dir := filepath.Join(folder, ownDir)
	readJSON := func(name string) []json.RawMessage {
		var v []json.RawMessage
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			_ = json.Unmarshal(b, &v)
		}
		if v == nil {
			v = []json.RawMessage{}
		}
		return v
	}
	writeJSON := func(name string, v any) error {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		b, _ := json.Marshal(v)
		return os.WriteFile(filepath.Join(dir, name), b, 0o600)
	}
	first := json.RawMessage("null")
	if len(args) > 0 {
		first = args[0]
	}
	switch api {
	case "userScripts.file":
		return userScriptFile(first, folder)
	case "userScripts.list":
		return readJSON("userscripts.json"), nil
	case "userScripts.save":
		var scripts []json.RawMessage
		_ = json.Unmarshal(first, &scripts)
		if scripts == nil {
			scripts = []json.RawMessage{}
		}
		if err := writeJSON("userscripts.json", scripts); err != nil {
			return nil, err
		}
		keep := map[string]bool{}
		for _, s := range scripts {
			if n, err := userScriptFile(s, folder); err == nil {
				keep[filepath.Base(n)] = true
			}
		}
		sweep(folder, "us-", keep)
		return nil, nil
	case "userScripts.world", "userScripts.worlds":
		worlds := readJSON("worlds.json")
		if api == "userScripts.worlds" {
			return worlds, nil
		}
		var props struct {
			WorldID string `json:"worldId"`
			Reset   bool   `json:"reset"`
		}
		_ = json.Unmarshal(first, &props)
		kept := []json.RawMessage{}
		for _, w := range worlds {
			var o struct {
				WorldID string `json:"worldId"`
			}
			_ = json.Unmarshal(w, &o)
			if o.WorldID != props.WorldID {
				kept = append(kept, w)
			}
		}
		if !props.Reset {
			kept = append(kept, first)
		}
		return nil, writeJSON("worlds.json", kept)
	}
	return nil, unsupported("unsupported: " + api)
}
