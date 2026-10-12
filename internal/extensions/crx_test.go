package extensions

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func pb(n int, val []byte) []byte {
	var out []byte
	key := uint64(n<<3 | 2)
	out = binary.AppendUvarint(out, key)
	out = binary.AppendUvarint(out, uint64(len(val)))
	return append(out, val...)
}

// makeCRX builds a CRX3 signed with key, for an id made from key.
func makeCRX(t *testing.T, key *rsa.PrivateKey, zipData []byte) ([]byte, string) {
	t.Helper()
	pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	h := sha256.Sum256(pub)
	crxID := h[:16]
	signed := pb(1, crxID)
	var msg bytes.Buffer
	msg.WriteString("CRX3 SignedData\x00")
	var l [4]byte
	binary.LittleEndian.PutUint32(l[:], uint32(len(signed)))
	msg.Write(l[:])
	msg.Write(signed)
	msg.Write(zipData)
	d := sha256.Sum256(msg.Bytes())
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, d[:])
	if err != nil {
		t.Fatal(err)
	}
	header := append(pb(2, append(pb(1, pub), pb(2, sig)...)), pb(10000, signed)...)
	var crx bytes.Buffer
	crx.WriteString("Cr24")
	binary.Write(&crx, binary.LittleEndian, uint32(3))
	binary.Write(&crx, binary.LittleEndian, uint32(len(header)))
	crx.Write(header)
	crx.Write(zipData)
	return crx.Bytes(), Letters(crxID)
}

func zipOf(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, text := range files {
		f, _ := w.Create(name)
		f.Write([]byte(text))
	}
	w.Close()
	return buf.Bytes()
}

func TestCRXCheck(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	z := zipOf(t, map[string]string{"manifest.json": `{"name":"x","version":"1","manifest_version":3}`})
	crx, id := makeCRX(t, key, z)
	got, gotID, err := VerifiedZip(crx, id)
	if err != nil || !bytes.Equal(got, z) || gotID != id {
		t.Fatalf("a good CRX: %v", err)
	}
	bad := append([]byte(nil), crx...)
	bad[len(bad)-5] ^= 1
	if _, _, err := VerifiedZip(bad, id); err != ErrSignature {
		t.Fatalf("a changed zip: %v", err)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, otherID := makeCRX(t, other, z)
	if _, _, err := VerifiedZip(crx, otherID); err != ErrSignature {
		t.Fatalf("another id: %v", err)
	}
	if _, _, err := VerifiedZip([]byte("PK\x03\x04..............."), id); err != ErrNotCrx {
		t.Fatalf("a zip: %v", err)
	}
	dir := filepath.Join(t.TempDir(), "ext", id)
	if err := Unpack(z, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatal("unpacked")
	}
	if err := Unpack(zipOf(t, map[string]string{"a.txt": "x"}), dir); err != ErrUnpack {
		t.Fatalf("no manifest: %v", err)
	}
}

func TestIDs(t *testing.T) {
	const id = "ddkjiahejlhfcafbddmgiahcphecmpfh" // uBlock Origin Lite
	if StoreID("https://chromewebstore.google.com/detail/ublock-origin-lite/"+id) != id {
		t.Fatal("store page")
	}
	if StoreID("https://chrome.google.com/webstore/detail/x/"+id+"?hl=en") != id {
		t.Fatal("legacy page")
	}
	if StoreID("http://chromewebstore.google.com/detail/"+id) != "" || StoreID("https://evil.com/detail/"+id) != "" {
		t.Fatal("only https and the store")
	}
	if IDIn("see "+id+".") != id || IDIn("nothing here") != "" {
		t.Fatal("id in text")
	}
}
