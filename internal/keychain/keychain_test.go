package keychain

import (
	"os"
	"testing"
)

// The round trip writes the login Keychain under a test service; it runs
// only with NOTCH_LAB_KEYCHAIN_TEST=1 (R-BUILD-040: tests touch no user data).
func TestRoundTrip(t *testing.T) {
	if os.Getenv("NOTCH_LAB_KEYCHAIN_TEST") != "1" {
		t.Skip("set NOTCH_LAB_KEYCHAIN_TEST=1")
	}
	Service = "dev.sviniabanditka.notch-lab.test"
	const acct = "http/env/dev/secret.token"
	if err := Set(acct, `p"a\ss w0rd`); err != nil {
		t.Fatal(err)
	}
	if v, err := Get(acct); err != nil || v != `p"a\ss w0rd` {
		t.Fatalf("%q %v", v, err)
	}
	if err := Delete(acct); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(acct); err != ErrNotFound {
		t.Fatalf("after delete: %v", err)
	}
}
