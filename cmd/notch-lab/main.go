// notch-lab is a native macOS development environment under the notch.
//
// M0: the main window with the Shell's frame and nothing in it yet.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/home"
)

// version is the content of VERSION; tools/dev sets it with -ldflags.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("notch-lab %s (%s)\n", version, commit())
		return
	}
	logFile, err := home.OpenLog("app.log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	logf("notch-lab %s (%s) starting", version, commit())

	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:         "notch-lab",
			Width:         1200,
			Height:        800,
			MinWidth:      720,
			MinHeight:     480,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHidden,
			Vibrancy:      mygo.VibrancySidebar,
			Content:       ui.View(shell),
		})
		logf("frame: main window open")
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// shell draws the frame of the Shell: the title strip, the rail, the
// sidebar and the Layout area. Each is empty in M0 (02-shell.md).
func shell(c *ui.Context) {
	t := c.Theme()
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		rail := ui.Column(c).Width(48)
		if !c.Vibrancy() {
			rail.Background(t.Surface)
		}
		rail.Children(func() {
			bar := c.TitleBar()
			ui.Row(c).Height(max(bar.Height, 38)).DragWindow()
		})
		sidebar := ui.Column(c).Width(240)
		if !c.Vibrancy() {
			sidebar.Background(t.Surface)
		}
		sidebar.Children(func() {
			ui.Row(c).Height(38).DragWindow()
		})
		ui.Column(c).Grow(1).Background(t.Background).Children(func() {
			ui.Row(c).Height(38).Center().DragWindow().Children(func() {
				ui.Text(c, "notch-lab").FontSize(13).TextColor(t.TextMuted)
			})
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "No Bench yet").TextColor(t.TextMuted)
			})
		})
	})
}

func commit() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 8 {
				return s.Value[:8]
			}
		}
	}
	return "unknown"
}

func logf(format string, args ...any) {
	log.Printf(time.Now().Format(time.RFC3339)+" info main: "+format, args...)
}

var _ = os.Exit
