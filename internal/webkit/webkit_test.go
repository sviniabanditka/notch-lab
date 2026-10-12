//go:build darwin

package webkit

import (
	"strings"
	"testing"
)

func TestDataStoreAndScripts(t *testing.T) {
	cfg := send(send(class("WKWebViewConfiguration"), "alloc"), "init")
	defer release(cfg)
	const u = "4E4F5443-484C-4142-0000-000000000001"
	SetDataStore(uintptr(cfg), u)
	got := goString(send(send(send(cfg, "websiteDataStore"), "identifier"), "UUIDString"))
	if !strings.EqualFold(got, u) {
		t.Fatalf("store identifier %q", got)
	}
	if nsString("héllo") == 0 || goString(nsString("héllo")) != "héllo" {
		t.Fatal("strings")
	}
}
