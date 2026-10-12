package transcript

import (
	"os"
	"path/filepath"
	"strings"
)

// configDir is Claude Code's config folder: CLAUDE_CONFIG_DIR as the app
// was started with, else ~/.claude.
func configDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// projectName is the folder Claude Code keeps a working directory's
// transcripts in: every byte but an ASCII letter or digit becomes '-'.
func projectName(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9') {
			b[i] = '-'
		}
	}
	return string(b)
}

// locate finds a session's transcript under dir: in its folder's project,
// else in any project, as Claude may have started in another folder
// (or one a symlink names, as /tmp).
func locate(dir, folder, sessionID string) (string, bool) {
	if sessionID == "" || strings.ContainsAny(sessionID, `/\`) || strings.Contains(sessionID, "..") {
		return "", false
	}
	name := sessionID + ".jsonl"
	if folder != "" {
		if p := filepath.Join(dir, "projects", projectName(folder), name); isFile(p) {
			return p, true
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "projects", "*", name))
	for _, p := range matches {
		if isFile(p) {
			return p, true
		}
	}
	return "", false
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}
