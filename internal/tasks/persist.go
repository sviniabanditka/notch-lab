package tasks

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

const stateFile = "tasks.json"

// choices are what the form remembers across opens and restarts. Skip
// permissions is never among them: it is off on every open.
type choices struct {
	Fresh    bool `json:"fresh"`
	Worktree bool `json:"worktree"`
}

// loadChoices reads the file in dir; a missing or unreadable one gives
// fork in the same checkout.
func loadChoices(dir string) choices {
	var c choices
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("tasks: %v", err)
		}
		return c
	}
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("tasks: %s: %v", stateFile, err)
		return choices{}
	}
	return c
}

// saveChoices replaces the file in dir in one rename.
func saveChoices(dir string, c choices) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, stateFile+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, stateFile))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
