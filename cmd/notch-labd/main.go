// notch-labd is the terminal daemon: it owns every Session so that they
// outlive the app (docs/spec/03-terminal.md, ADR 0003).
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/daemon"
	"github.com/sviniabanditka/notch-lab/internal/home"
)

var version = "0.1.0"

func main() {
	dir, err := home.Dir()
	if err != nil {
		log.Fatal(err)
	}
	f, err := home.OpenLog("daemon.log")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.SetPrefix("")
	logf := func(format string, args ...any) {
		log.Printf(time.Now().Format(time.RFC3339)+" info daemon: "+format, args...)
	}
	// In the bundle the daemon is Contents/Resources/bin/notch-labd, and
	// libghostty-vt.dylib is in Contents/Resources.
	if exe, err := os.Executable(); err == nil && os.Getenv("MYGO_GHOSTTY_VT") == "" {
		lib := filepath.Join(filepath.Dir(filepath.Dir(exe)), "libghostty-vt.dylib")
		if _, err := os.Stat(lib); err == nil {
			os.Setenv("MYGO_GHOSTTY_VT", lib)
		}
	}
	d, err := daemon.Listen(dir, version)
	if errors.Is(err, daemon.ErrRunning) {
		logf("%v", err)
		os.Exit(3)
	}
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	logf("notch-labd %s listening on %s (pid %d)", version, daemon.SocketPath(dir), os.Getpid())
	if err := d.Serve(ctx); err != nil {
		log.Fatal(err)
	}
	logf("stopped")
}
