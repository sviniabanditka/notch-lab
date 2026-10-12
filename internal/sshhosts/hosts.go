// Package sshhosts keeps the ssh Bench's Hosts, imports ~/.ssh/config and
// writes the ssh command of a Host (docs/spec/10-ssh.md).
package sshhosts

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Host is one machine (R-SSH-001). Imported ones come from ~/.ssh/config
// and keep their alias as Name.
type Host struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Hostname string   `json:"hostname"`
	Port     int      `json:"port,omitempty"`
	User     string   `json:"user,omitempty"`
	Identity string   `json:"identity,omitempty"`
	Jump     string   `json:"jump,omitempty"`    // a Host id or an ssh alias
	Options  []string `json:"options,omitempty"` // Key=Value, each sent as -o
	Color    string   `json:"color,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Order    int      `json:"order"`
	Imported bool     `json:"-"`
}

// Store is hosts.json (R-SSH-002) and the hosts of ~/.ssh/config.
type Store struct {
	Path     string
	List     []*Host
	Imported []*Host
	config   string
	mod      map[string]int64
}

// Load reads hosts.json and ~/.ssh/config (or config, for tests).
func Load(path, config string) *Store {
	s := &Store{Path: path, config: config, mod: map[string]int64{}}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s.List)
	}
	s.Reload()
	return s
}

// Save writes hosts.json.
func (s *Store) Save() error {
	sort.SliceStable(s.List, func(i, j int) bool { return s.List[i].Order < s.List[j].Order })
	data, _ := json.MarshalIndent(s.List, "", "  ")
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// Reload reads ~/.ssh/config again when it or an included file changed
// (R-SSH-005); it reports a change.
func (s *Store) Reload() bool {
	files := append([]string{s.config}, includes(s.config)...)
	changed := false
	for _, f := range files {
		var m int64
		if st, err := os.Stat(f); err == nil {
			m = st.ModTime().UnixNano()
		}
		if s.mod[f] != m {
			s.mod[f], changed = m, true
		}
	}
	if !changed && s.Imported != nil {
		return false
	}
	s.Imported = ParseConfig(s.config)
	return true
}

// Get finds a Host by id, or an imported one by alias.
func (s *Store) Get(id string) *Host {
	for _, h := range s.List {
		if h.ID == id {
			return h
		}
	}
	for _, h := range s.Imported {
		if h.ID == id || h.Name == id {
			return h
		}
	}
	return nil
}

// Find finds a Host by id, name or alias, own ones first.
func (s *Store) Find(name string) *Host {
	if h := s.Get(name); h != nil {
		return h
	}
	for _, h := range s.List {
		if h.Name == name {
			return h
		}
	}
	return nil
}

// Remove takes a Host out (R-SSH-008).
func (s *Store) Remove(id string) {
	for i, h := range s.List {
		if h.ID == id {
			s.List = append(s.List[:i], s.List[i+1:]...)
			_ = s.Save()
			return
		}
	}
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// includes lists the files `Include` names, one level deep.
func includes(path string) []string {
	var out []string
	for _, kv := range lines(path) {
		if strings.EqualFold(kv[0], "include") {
			for _, pat := range strings.Fields(kv[1]) {
				pat = expand(pat)
				if !filepath.IsAbs(pat) {
					pat = filepath.Join(filepath.Dir(path), pat)
				}
				m, _ := filepath.Glob(pat)
				out = append(out, m...)
			}
		}
	}
	return out
}

// lines reads a config file as key, value pairs.
func lines(path string) [][2]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out [][2]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		key, val, ok := strings.Cut(l, " ")
		if k, v, ok2 := strings.Cut(l, "="); ok2 && (!ok || len(k) < len(key)) {
			key, val, ok = k, v, true
		}
		if !ok {
			continue
		}
		out = append(out, [2]string{strings.TrimSpace(key), strings.Trim(strings.TrimSpace(val), `"`)})
	}
	return out
}

// ParseConfig reads Host blocks of an ssh config, and of the files it
// includes one level deep; patterns with * or ? are skipped (R-SSH-004).
func ParseConfig(path string) []*Host {
	var out []*Host
	var cur []*Host
	read := func(file string) {
		for _, kv := range lines(file) {
			key, val := strings.ToLower(kv[0]), kv[1]
			switch key {
			case "include":
				// The included files are read after this one, below.
			case "host":
				cur = nil
				for _, alias := range strings.Fields(val) {
					if strings.ContainsAny(alias, "*?!") {
						continue
					}
					h := &Host{ID: "config:" + alias, Name: alias, Hostname: alias, Imported: true}
					cur = append(cur, h)
					out = append(out, h)
				}
			case "match":
				cur = nil
			default:
				for _, h := range cur {
					switch key {
					case "hostname":
						h.Hostname = val
					case "port":
						h.Port, _ = strconv.Atoi(val)
					case "user":
						h.User = val
					case "identityfile":
						if h.Identity == "" {
							h.Identity = val
						}
					case "proxyjump":
						h.Jump = val
					}
				}
			}
		}
	}
	read(path)
	for _, f := range includes(path) {
		cur = nil
		read(f)
	}
	return out
}

// Args is the ssh command of a Host after `/usr/bin/ssh` (R-SSH-021);
// lookup expands a jump host that is a Host id.
func Args(h *Host, lookup func(id string) *Host) []string {
	args := []string{"-o", "ServerAliveInterval=30", "-o", "ServerAliveCountMax=3"}
	if h.Imported {
		return append(args, h.Name)
	}
	if h.Port != 0 && h.Port != 22 {
		args = append(args, "-p", strconv.Itoa(h.Port))
	}
	if h.User != "" {
		args = append(args, "-l", h.User)
	}
	if h.Identity != "" {
		args = append(args, "-i", expand(h.Identity))
	}
	if h.Jump != "" {
		jump := h.Jump
		if lookup != nil {
			if j := lookup(h.Jump); j != nil && !j.Imported {
				jump = j.Hostname
				if j.User != "" {
					jump = j.User + "@" + jump
				}
				if j.Port != 0 {
					jump += ":" + strconv.Itoa(j.Port)
				}
			} else if j != nil {
				jump = j.Name
			}
		}
		args = append(args, "-J", jump)
	}
	for _, o := range h.Options {
		if o = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(o), "-o")); o != "" {
			args = append(args, "-o", strings.TrimSpace(o))
		}
	}
	return append(args, h.Hostname)
}

// Command is the shell line of a Host's command (Copy ssh command).
func Command(h *Host, lookup func(id string) *Host) string {
	parts := []string{"ssh"}
	for _, a := range Args(h, lookup) {
		if strings.ContainsAny(a, " '\"$") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// Match reports whether a Host matches a search (R-SSH-011).
func (h *Host) Match(q string) bool {
	q = strings.ToLower(q)
	if q == "" {
		return true
	}
	hay := strings.ToLower(h.Name + " " + h.Hostname + " " + h.User + " " + strings.Join(h.Tags, " "))
	return strings.Contains(hay, q)
}
