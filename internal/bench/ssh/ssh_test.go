package ssh

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/bench/terminal"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
	"github.com/sviniabanditka/notch-lab/internal/sshhosts"
)

func TestHostsAndConnect(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config"), []byte("Host lab\n  HostName lab.local\n  User me\n"), 0o600)
	st := model.New()
	st.EnsureLab()
	bench := st.NewBench(st.Labs[0], model.SSH, "Servers", "")
	set, _, _ := settings.Load(filepath.Join(dir, "settings.json"))
	b := &Bench{Bench: terminal.New(st, set, dir), Hosts: sshhosts.Load(filepath.Join(dir, "hosts.json"), filepath.Join(dir, "config"))}
	b.Hosts.List = []*sshhosts.Host{
		{ID: "b", Name: "bastion", Hostname: "b.example.com", User: "ops", Tags: []string{"prod"}},
		{ID: "a", Name: "api", Hostname: "10.0.0.7", Port: 2222, Jump: "b", Tags: []string{"prod", "eu"}},
	}
	tab := b.Connect(bench, b.Hosts.Get("a"))
	if tab.Title != "api" || !strings.Contains(tab.Data, `"host":"a"`) || !strings.Contains(tab.Data, `"-J","ops@b.example.com"`) || !strings.Contains(tab.Data, `"/usr/bin/ssh"`) {
		t.Fatalf("tab %s %s", tab.Title, tab.Data)
	}
	if spec, _, ok := b.Tunnel("bastion"); !ok || spec != "ops@b.example.com" {
		t.Fatalf("tunnel %q", spec)
	}
	nt := &model.Tab{ID: "n", Bench: bench, Data: "{}"}
	b.sel = "config:lab"
	b.NewTab(bench, nt)
	if nt.Title != "lab" || !strings.Contains(nt.Data, `"lab"]`) {
		t.Fatalf("Cmd+T %s %s", nt.Title, nt.Data)
	}
	b.EditHost(nil)
	b.sheet.h.Hostname = ""
	b.saveSheet(b.sheet)
	if b.sheet == nil || b.sheet.err != "Hostname is required" {
		t.Fatal("hostname required")
	}
	b.sheet.h.Hostname, b.sheet.port, b.sheet.h.Jump, b.sheet.tags = "web.local", "2200", "bastion", "web, eu"
	b.saveSheet(b.sheet)
	if h := b.Hosts.List[2]; h.Name != "web.local" || h.Port != 2200 || h.Jump != "b" || len(h.Tags) != 2 {
		t.Fatalf("saved %+v", h)
	}
	pt := ui.NewTester(func(c *ui.Context) { b.Panel(c, bench) }, 280, 400)
	for _, want := range []string{"prod", "eu", "web", "api", "ssh config", "lab"} {
		if !pt.HasText(want) {
			t.Fatalf("panel lacks %q: %q", want, pt.Texts())
		}
	}
	if out := os.Getenv("NOTCH_LAB_SHOT"); out != "" {
		f, _ := os.Create(out)
		png.Encode(f, pt.Image())
		f.Close()
	}
}
