package extensions

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed js/store.js
var storeScript string

// StoreScript replaces the Web Store's grey "Add to Chrome" with "Add to
// notch-lab" (R-WEB-140); handler is the message handler's name.
func StoreScript(handler string) string { return strings.ReplaceAll(storeScript, "%HANDLER%", handler) }

// Entry is one installed Extension in installed.json (R-WEB-180).
type Entry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
	Pinned  bool   `json:"pinned"`
	// Local is the folder of an unpacked Extension, loaded in place.
	Local   string `json:"local,omitempty"`
	Updated int64  `json:"updated"`
}

// Manager keeps the installed list. Main goroutine only, except the
// Fetch* functions, which return an Entry for Put.
type Manager struct {
	Dir  string // <home>/extensions
	List []*Entry
}

// Load reads installed.json under dir.
func Load(dir string) *Manager {
	m := &Manager{Dir: dir}
	if b, err := os.ReadFile(filepath.Join(dir, "installed.json")); err == nil {
		_ = json.Unmarshal(b, &m.List)
	}
	return m
}

// Save writes installed.json.
func (m *Manager) Save() {
	b, _ := json.MarshalIndent(m.List, "", "  ")
	_ = os.MkdirAll(m.Dir, 0o700)
	tmp := filepath.Join(m.Dir, "installed.json.tmp")
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, filepath.Join(m.Dir, "installed.json"))
	}
}

// Get finds an Entry.
func (m *Manager) Get(id string) *Entry {
	for _, e := range m.List {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// Folder is where an Extension's files are.
func (m *Manager) Folder(e *Entry) string {
	if e.Local != "" {
		return e.Local
	}
	return filepath.Join(m.Dir, e.ID)
}

// Put adds or replaces an Entry, keeping its switches.
func (m *Manager) Put(e *Entry) {
	if old := m.Get(e.ID); old != nil {
		e.Enabled, e.Pinned = old.Enabled, old.Pinned
		*old = *e
	} else {
		m.List = append(m.List, e)
	}
	m.Save()
}

// Remove takes an Extension out and deletes its unpacked files (R-WEB-182).
// An unpacked folder of the user's own stays.
func (m *Manager) Remove(id string) {
	for i, e := range m.List {
		if e.ID == id {
			if e.Local == "" {
				os.RemoveAll(filepath.Join(m.Dir, id))
			}
			m.List = append(m.List[:i], m.List[i+1:]...)
			m.Save()
			return
		}
	}
}

// manifest reads the name and version an Extension says it has.
func manifest(folder string) (name, version string, err error) {
	b, err := os.ReadFile(filepath.Join(folder, "manifest.json"))
	if err != nil {
		return "", "", errNoManifest
	}
	var mf struct{ Name, Version string }
	if err := json.Unmarshal(stripBOM(b), &mf); err != nil {
		return "", "", err
	}
	return mf.Name, mf.Version, nil
}

func (m *Manager) unpacked(zip []byte, id string) (*Entry, error) {
	folder := filepath.Join(m.Dir, id)
	if err := Unpack(zip, folder); err != nil {
		return nil, err
	}
	if err := Prepare(folder, true); err != nil {
		return nil, err
	}
	name, version, err := manifest(folder)
	if err != nil {
		return nil, err
	}
	return &Entry{ID: id, Name: name, Version: version, Enabled: true, Updated: time.Now().Unix()}, nil
}

// FetchStore downloads, checks and unpacks an Extension from the Web Store
// by link or id (R-WEB-141 … R-WEB-145). Off the main goroutine.
func (m *Manager) FetchStore(ctx context.Context, raw string) (*Entry, error) {
	id := StoreID(raw)
	if id == "" {
		id = IDIn(raw)
	}
	if id == "" {
		return nil, ErrNotAnID
	}
	crx, _, err := Fetch(ctx, DownloadURL(id))
	if err != nil {
		return nil, err
	}
	zip, _, err := VerifiedZip(crx, id)
	if err != nil {
		return nil, err
	}
	return m.unpacked(zip, id)
}

// FetchFile installs a .crx file; its id comes from its own key (R-WEB-146).
func (m *Manager) FetchFile(path string) (*Entry, error) {
	crx, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	zip, id, err := VerifiedZip(crx, "")
	if err != nil {
		return nil, err
	}
	return m.unpacked(zip, id)
}

// LocalID is the id of an unpacked folder, as Chrome makes it: the SHA-256
// of the path, written in letters a–p.
func LocalID(folder string) string {
	sum := sha256.Sum256([]byte(folder))
	return Letters(sum[:16])
}

// FetchFolder adds an unpacked folder, loaded in place (R-WEB-146).
func (m *Manager) FetchFolder(folder string) (*Entry, error) {
	folder, err := filepath.Abs(folder)
	if err != nil {
		return nil, err
	}
	if err := Prepare(folder, false); err != nil {
		return nil, err
	}
	name, version, err := manifest(folder)
	if err != nil {
		return nil, errors.New("The folder has no manifest.json")
	}
	return &Entry{ID: LocalID(folder), Name: name, Version: version, Enabled: true, Local: folder, Updated: time.Now().Unix()}, nil
}
