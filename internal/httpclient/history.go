package httpclient

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one line of a Request's history (R-HTTP-100).
type Entry struct {
	At      time.Time         `json:"at"`
	Status  int               `json:"status"`
	Ms      int64             `json:"time_ms"`
	Size    int64             `json:"size"`
	Error   string            `json:"error,omitempty"`
	Line    string            `json:"request"`
	Headers map[string]string `json:"headers,omitempty"`
	Resp    map[string]string `json:"response_headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

const (
	keep        = 50
	historyBody = 256 << 10
)

// HistoryFile is the history of a Request: the SHA-256 of its file and name.
func HistoryFile(dir, file, name string) string {
	sum := sha256.Sum256([]byte(file + "\x00" + name))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".jsonl")
}

// Append adds an entry and keeps the last 50 (R-HTTP-101); mask hides the
// secrets in it first (R-HTTP-100).
func Append(path string, e Entry, mask func(string) string) error {
	if len(e.Body) > historyBody {
		e.Body = e.Body[:historyBody]
	}
	raw, _ := json.Marshal(e)
	line := mask(string(raw))
	lines := append(readLines(path), line)
	if len(lines) > keep {
		lines = lines[len(lines)-keep:]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// History reads the entries, newest first.
func History(path string) []Entry {
	lines := readLines(path)
	out := make([]Entry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		var e Entry
		if json.Unmarshal([]byte(lines[i]), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 4<<20)
	for s.Scan() {
		if t := s.Text(); t != "" {
			out = append(out, t)
		}
	}
	return out
}
