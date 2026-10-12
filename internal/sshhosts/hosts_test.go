package sshhosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "conf.d"), 0o700)
	os.WriteFile(filepath.Join(dir, "config"), []byte(`
Include conf.d/*
Host *
  ServerAliveInterval 60
Host prod web-1
  HostName 10.0.0.5
  User deploy
  Port 2222
  IdentityFile ~/.ssh/prod
Host bastion
  HostName=bastion.example.com
Host db
  HostName db.internal
  ProxyJump bastion
`), 0o600)
	os.WriteFile(filepath.Join(dir, "conf.d", "extra"), []byte("Host lab\n  HostName lab.local\n  User me\n"), 0o600)
	s := Load(filepath.Join(dir, "hosts.json"), filepath.Join(dir, "config"))
	names := []string{}
	for _, h := range s.Imported {
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "prod,web-1,bastion,db,lab" {
		t.Fatalf("hosts %v", names)
	}
	p := s.Get("prod")
	if p.Hostname != "10.0.0.5" || p.User != "deploy" || p.Port != 2222 || !p.Imported {
		t.Fatalf("prod %+v", p)
	}
	if s.Get("bastion").Hostname != "bastion.example.com" || s.Get("db").Jump != "bastion" {
		t.Fatal("= syntax, ProxyJump")
	}
	if got := strings.Join(Args(p, s.Get), " "); got != "-o ServerAliveInterval=30 -o ServerAliveCountMax=3 prod" {
		t.Fatalf("imported args %q", got)
	}
	jump := &Host{ID: "j", Name: "jump", Hostname: "j.example.com", User: "ops", Port: 2200}
	own := &Host{ID: "h", Name: "api", Hostname: "api.example.com", User: "root", Port: 2022, Identity: "/k/id", Jump: "j", Options: []string{"-o ForwardAgent=yes", "Compression=yes"}}
	s.List = []*Host{jump, own}
	got := strings.Join(Args(own, s.Get), " ")
	want := "-o ServerAliveInterval=30 -o ServerAliveCountMax=3 -p 2022 -l root -i /k/id -J ops@j.example.com:2200 -o ForwardAgent=yes -o Compression=yes api.example.com"
	if got != want {
		t.Fatalf("own args\n%s\n%s", got, want)
	}
	if s.Reload() {
		t.Fatal("no change, no reload")
	}
	os.WriteFile(filepath.Join(dir, "conf.d", "extra"), []byte("Host lab2\n  HostName x\n"), 0o600)
	os.Chtimes(filepath.Join(dir, "conf.d", "extra"), timeNow(), timeNow())
	if !s.Reload() || s.Get("lab2") == nil {
		t.Fatal("an included file's change reloads")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if l := Load(s.Path, filepath.Join(dir, "config")); len(l.List) != 2 {
		t.Fatal("hosts.json")
	}
	if !own.Match("API") || own.Match("zzz") {
		t.Fatal("match")
	}
}

func timeNow() time.Time { return time.Now().Add(2 * time.Second) }
