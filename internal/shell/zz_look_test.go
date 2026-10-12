package shell

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
)

// TestLook writes the Shell as it looks, light and dark, when
// NOTCH_LAB_LOOK names a folder: a design check, not a test.
func TestLook(t *testing.T) {
	out := os.Getenv("NOTCH_LAB_LOOK")
	if out == "" {
		t.Skip("set NOTCH_LAB_LOOK to a folder")
	}
	s, term, _ := fixture(t)
	s.St.Split(term.Tabs[1], model.Row)
	s.DaemonOK = func() bool { return true }
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(s.View, 1200, 760)
		tt.SetDark(dark)
		tt.Frame()
		name := "light"
		if dark {
			name = "dark"
		}
		shot := func(file string, x *ui.Tester) {
			f, _ := os.Create(out + "/" + file + "-" + name + ".png")
			png.Encode(f, x.Image())
			f.Close()
		}
		shot("shell", tt)
		s.OpenSwitcher()
		tt.Frame()
		shot("switcher", tt)
		s.sw.open = false
		s.OpenNewBench()
		tt.Frame()
		shot("dialog", tt)
		s.dlg.open = false
		v := &SettingsView{S: s}
		st := ui.NewTester(v.View, 720, 520)
		st.SetDark(dark)
		st.Frame()
		shot("settings", st)
	}
}
