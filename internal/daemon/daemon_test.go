package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
)

func start(t *testing.T) (string, *Client) {
	t.Helper()
	home, err := os.MkdirTemp("", "nld") // short: a Unix socket path is at most 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	d, err := Listen(home, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go d.Serve(ctx)
	if _, err := Listen(home, "test"); err != ErrRunning {
		t.Fatalf("second daemon: %v", err)
	}
	c, err := Dial(home, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return home, c
}

func waitText(t *testing.T, term *terminal.Terminal, want string) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if strings.Contains(term.Text(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %q in %q", want, term.Text())
}

func TestSessionAttachSnapshotReattach(t *testing.T) {
	home, c := start(t)
	id, err := c.Create("", "/tmp", []string{"/bin/sh"}, map[string]string{"PS1": "$ "}, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	sc := c.Open(id)
	term, err := terminal.New(terminal.Options{Conn: sc})
	if err != nil {
		t.Fatal(err)
	}
	sc.Resize(80, 24)
	if _, err := sc.Write([]byte("for i in 1 2 3; do echo line$i; done; echo $NOTCH_LAB_SESSION\n")); err != nil {
		t.Fatal(err)
	}
	waitText(t, term, "line3")
	waitText(t, term, id)
	term.Close() // detaches; the Session goes on

	// A second app attaches and sees the same Screen from the Snapshot.
	c2, err := Dial(home, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	sc2 := c2.Open(id)
	term2, _ := terminal.New(terminal.Options{Conn: sc2})
	defer term2.Close()
	sc2.Resize(80, 24)
	waitText(t, term2, "line2")
	list, _ := c2.List()
	if len(list) != 1 || list[0].ID != id || list[0].Cwd == "" {
		t.Fatalf("list %+v", list)
	}
	if err := c2.CloseSession(id); err != nil {
		t.Fatal(err)
	}
	if list, _ := c2.List(); len(list) != 0 {
		t.Fatal("closed session still listed")
	}
}

func TestExitAndState(t *testing.T) {
	_, c := start(t)
	states := make(chan State, 16)
	c.OnState = func(s State) { states <- s }
	id, err := c.Create("", "/tmp", []string{"/bin/sh", "-c", "exit 7"}, nil, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	sc := c.Open(id)
	sc.Resize(80, 24)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case s := <-states:
			if s.Exited {
				if s.Code != 7 {
					t.Fatalf("code %d", s.Code)
				}
				return
			}
		case <-deadline:
			t.Fatal("no exit state")
		}
	}
}

func TestProtocolMismatch(t *testing.T) {
	home, _ := start(t)
	nc, err := dialRaw(home)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	writeMsg(nc, &Msg{T: "hello", Protocol: 99})
	m, err := readMsg(nc)
	if err != nil || m.Err == nil || m.Err.Code != "protocol" {
		t.Fatalf("got %+v %v", m, err)
	}
}

func TestScreenStatus(t *testing.T) {
	for text, want := range map[string]string{
		"✻ Thinking… (esc to interrupt)":             "working",
		"Do you want to proceed?\n❯ 1. Yes\n  2. No": "blocked",
		"> \n  ? for shortcuts":                      "idle",
		"• Working (12s • Esc to interrupt)":         "working",
	} {
		if got := screenStatus(text); got != want {
			t.Errorf("%q: %s, want %s", text, got, want)
		}
	}
	if got := lastLines("", 40); got != "" {
		t.Errorf("empty screen %q", got)
	}
	if got := lastLines("one", 40); got != "one" {
		t.Errorf("one line %q", got)
	}
	if got := lastLines("a\nb\nc\nd\n", 2); got != "c\nd" {
		t.Errorf("lastLines %q", got)
	}
}

func TestEnvironmentDropsDyld(t *testing.T) {
	d := &Daemon{Version: "t"}
	env := strings.Join(d.environment("x", map[string]string{"DYLD_INSERT_LIBRARIES": "evil", "FOO": "bar"}), "\n")
	if strings.Contains(env, "DYLD_") || !strings.Contains(env, "FOO=bar") || !strings.Contains(env, "NOTCH_LAB_SESSION=x") {
		t.Fatalf("env:\n%s", env)
	}
	_ = filepath.Join
}

func TestAgentDetectionAndStatus(t *testing.T) {
	_, c := start(t)
	dir := t.TempDir()
	src := `package main
import ("fmt"; "time")
func main() {
	fmt.Print("✻ Working (esc to interrupt)\n"); time.Sleep(2500 * time.Millisecond)
	fmt.Print("\033[2J\033[HDo you want to proceed?\n❯ 1. Yes\n"); time.Sleep(2500 * time.Millisecond)
	fmt.Print("\033[2J\033[H> \n"); time.Sleep(3 * time.Second)
}`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644)
	bin := filepath.Join(dir, "claude")
	build := exec.Command("go", "build", "-o", bin, filepath.Join(dir, "main.go"))
	build.Env = append(os.Environ(), "GO111MODULE=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	states := make(chan State, 64)
	c.OnState = func(s State) { states <- s }
	id, err := c.Create("", "/tmp", []string{bin}, nil, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	c.Open(id).Resize(80, 24)
	seen := map[string]bool{}
	deadline := time.After(9 * time.Second)
	for !seen["done"] {
		select {
		case s := <-states:
			if s.Agent != nil {
				if s.Agent.Kind != "claude" {
					t.Fatalf("kind %q", s.Agent.Kind)
				}
				seen[s.Agent.Status] = true
			}
		case <-deadline:
			t.Fatalf("statuses seen: %v", seen)
		}
	}
	if !seen["working"] || !seen["blocked"] {
		t.Fatalf("statuses seen: %v", seen)
	}
}
