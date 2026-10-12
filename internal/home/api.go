// Package home locates the app's home folder and opens its log files.
//
// The home folder is $NOTCH_LAB_HOME when set, else
// ~/Library/Application Support/notch-lab. tools/dev and every test set
// NOTCH_LAB_HOME so that a run never touches the user's own folder.
package home

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	once sync.Once
	dir  string
	err  error
)

// Dir returns the home folder and creates it when it is absent.
func Dir() (string, error) {
	once.Do(func() {
		dir = os.Getenv("NOTCH_LAB_HOME")
		if dir == "" {
			var base string
			base, err = os.UserConfigDir() // ~/Library/Application Support on macOS
			if err != nil {
				return
			}
			dir = filepath.Join(base, "notch-lab")
		}
		err = os.MkdirAll(dir, 0o700)
	})
	return dir, err
}

// OpenLog makes the standard logger write to <home>/<name> as well as
// stderr, with RFC3339 times, and returns the file to close at exit.
// Rotation at 10 MiB: [later].
func OpenLog(name string) (*os.File, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(d, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	log.SetFlags(0)
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	return f, nil
}
