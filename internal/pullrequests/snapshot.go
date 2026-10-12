package pullrequests

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const (
	snapshotPath    = "cache/pullrequests.json"
	snapshotVersion = 1
	// maxSnapshotInboxes keeps the inboxes of the repos used last.
	maxSnapshotInboxes = 8
)

// snapshot is the lists as last fetched, kept on disk so that the view
// shows them before the network answers. It holds rows only: no token,
// no errors, no detail.
type snapshot struct {
	Version int
	// Account is the fingerprint of the token the lists were fetched
	// with; another login drops them.
	Account string
	// Inboxes are by filter: "" for every repo, else owner/name.
	Inboxes map[string]*inbox
}

func loadSnapshot(dir string) (snapshot, error) {
	if dir == "" {
		return snapshot{}, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, snapshotPath))
	if errors.Is(err, fs.ErrNotExist) {
		return snapshot{}, nil
	}
	if err != nil {
		return snapshot{}, err
	}
	var s snapshot
	if err := json.Unmarshal(data, &s); err != nil || s.Version != snapshotVersion {
		return snapshot{}, err
	}
	return s, nil
}

// saveSnapshot writes s, readable by the user only, keeping its newest
// inboxes.
func saveSnapshot(dir string, s snapshot) error {
	if dir == "" {
		return nil
	}
	s.Version = snapshotVersion
	if len(s.Inboxes) > maxSnapshotInboxes {
		keys := make([]string, 0, len(s.Inboxes))
		for k := range s.Inboxes {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return s.Inboxes[keys[i]].Fetched.After(s.Inboxes[keys[j]].Fetched) })
		kept := map[string]*inbox{}
		for _, k := range keys[:maxSnapshotInboxes] {
			kept[k] = s.Inboxes[k]
		}
		s.Inboxes = kept
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, snapshotPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
