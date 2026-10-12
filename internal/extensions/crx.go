// Package extensions is the browser's WebExtension layer: Store installs
// with the CRX check, the Shim, and the installed list
// (docs/spec/04-browser.md R-WEB-130 … R-WEB-182). It ports Search by
// Office Commun (MIT): Crx.swift, StoreRelay.swift, ExtensionShims.swift.
package extensions

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Errors of an install, as Search words them (R-WEB-143 … R-WEB-145).
var (
	ErrNotAnID   = errors.New("That isn't a Chrome Web Store link or extension id")
	ErrEmpty     = errors.New("The Chrome Web Store has nothing for that id — it may have been taken down, or only exist for old versions of Chrome")
	ErrNotCrx    = errors.New("What came back isn't a Chrome extension")
	ErrSignature = errors.New("The extension's signature doesn't hold up")
	ErrUnpack    = errors.New("The extension couldn't be unpacked")
)

// ChromeVersion is the Chrome the Store is told it talks to.
const ChromeVersion = "140.0.0.0"

var anyID = regexp.MustCompile(`(?:^|[^a-z])([a-p]{32})(?:[^a-z]|$)`)

// IDIn finds 32 letters a–p anywhere in a text: a bare id or a link.
func IDIn(text string) string {
	m := anyID.FindStringSubmatch(strings.ToLower(text))
	if m == nil {
		return ""
	}
	return m[1]
}

var (
	storePath  = regexp.MustCompile(`^/detail/(?:[^/]+/)?([a-p]{32})(?:/.*)?$`)
	legacyPath = regexp.MustCompile(`^/webstore/detail/(?:[^/]+/)?([a-p]{32})(?:/.*)?$`)
)

// StoreID is the Extension a Store page is about, read from its address
// only (R-WEB-141).
func StoreID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	var re *regexp.Regexp
	switch strings.ToLower(u.Host) {
	case "chromewebstore.google.com":
		re = storePath
	case "chrome.google.com":
		re = legacyPath
	default:
		return ""
	}
	if m := re.FindStringSubmatch(u.Path); m != nil {
		return m[1]
	}
	return ""
}

// IsStorePage reports a Chrome Web Store page.
func IsStorePage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Host)
	return h == "chromewebstore.google.com" || h == "chrome.google.com" && strings.HasPrefix(u.Path, "/webstore")
}

// DownloadURL is where Chromium browsers ask for an id (R-WEB-143).
func DownloadURL(id string) string {
	q := url.Values{}
	q.Set("response", "redirect")
	q.Set("prodversion", ChromeVersion)
	q.Set("acceptformat", "crx3")
	q.Set("x", "id="+id+"&installsource=ondemand&uc")
	return "https://clients2.google.com/service/update2/crx?" + q.Encode()
}

// httpsOnly refuses a redirect to anything but https (R-WEB-230).
var client = &http.Client{
	Timeout: 60 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("a redirect left https")
		}
		if len(via) > 10 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

// Fetch downloads a URL over https.
func Fetch(ctx context.Context, raw string) ([]byte, string, error) {
	if !strings.HasPrefix(raw, "https://") {
		return nil, "", errors.New("downloads go over https only")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/"+ChromeVersion+" Safari/537.36")
	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, "", fmt.Errorf("The Chrome Web Store answered %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 200<<20))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", ErrEmpty
	}
	return data, res.Request.URL.String(), nil
}

// Letters writes bytes as a Chrome id: each half byte a letter a–p.
func Letters(b []byte) string {
	out := make([]byte, 0, len(b)*2)
	for _, x := range b {
		out = append(out, 'a'+x>>4, 'a'+x&0x0f)
	}
	return string(out)
}

type field struct {
	n   int
	val []byte
}

// protobuf reads the length-delimited fields of a message; others are skipped.
func protobuf(b []byte) []field {
	var out []field
	i := 0
	varint := func() (int, bool) {
		v, shift := 0, 0
		for i < len(b) {
			c := int(b[i])
			i++
			v |= (c & 0x7f) << shift
			if c&0x80 == 0 {
				return v, true
			}
			shift += 7
			if shift > 56 {
				return 0, false
			}
		}
		return 0, false
	}
	for i < len(b) {
		key, ok := varint()
		if !ok {
			break
		}
		switch key & 7 {
		case 2:
			n, ok := varint()
			if !ok || n < 0 || i+n > len(b) {
				return out
			}
			out = append(out, field{key >> 3, b[i : i+n]})
			i += n
		case 0:
			varint()
		case 1:
			i += 8
		case 5:
			i += 4
		default:
			return out
		}
	}
	return out
}

func first(fs []field, n int) []byte {
	for _, f := range fs {
		if f.n == n {
			return f.val
		}
	}
	return nil
}

// VerifiedZip returns the zip inside a CRX3 once its signature holds for
// id, and the id when id is "" (a file with its own id) (R-WEB-144).
func VerifiedZip(crx []byte, id string) ([]byte, string, error) {
	if len(crx) <= 12 || !bytes.Equal(crx[:4], []byte("Cr24")) || binary.LittleEndian.Uint32(crx[4:8]) != 3 {
		return nil, "", ErrNotCrx
	}
	n := int(binary.LittleEndian.Uint32(crx[8:12]))
	if n < 0 || 12+n > len(crx) {
		return nil, "", ErrNotCrx
	}
	header, zip := crx[12:12+n], crx[12+n:]
	fs := protobuf(header)
	signed := first(fs, 10000)
	if signed == nil {
		return nil, "", ErrSignature
	}
	crxID := first(protobuf(signed), 1)
	if crxID == nil {
		return nil, "", ErrSignature
	}
	if id == "" {
		id = Letters(crxID)
	}
	if Letters(crxID) != id {
		return nil, "", ErrSignature
	}
	var msg bytes.Buffer
	msg.WriteString("CRX3 SignedData")
	msg.WriteByte(0)
	var l [4]byte
	binary.LittleEndian.PutUint32(l[:], uint32(len(signed)))
	msg.Write(l[:])
	msg.Write(signed)
	msg.Write(zip)
	digest := sha256.Sum256(msg.Bytes())
	for _, f := range fs {
		if f.n != 2 { // sha256_with_rsa proofs
			continue
		}
		proof := protobuf(f.val)
		key, sig := first(proof, 1), first(proof, 2)
		if key == nil || sig == nil {
			continue
		}
		h := sha256.Sum256(key)
		if Letters(h[:16]) != id { // the Store's own proof does not count
			continue
		}
		pub, err := x509.ParsePKIXPublicKey(key)
		if err != nil {
			continue
		}
		if rk, ok := pub.(*rsa.PublicKey); ok && rsa.VerifyPKCS1v15(rk, crypto.SHA256, digest[:], sig) == nil {
			return zip, id, nil
		}
	}
	return nil, "", ErrSignature
}

// Unpack unzips into folder, replaced whole, mode 0700; a package that
// holds a link is refused (R-WEB-145, R-WEB-232).
func Unpack(zip []byte, folder string) error {
	scratch, err := os.MkdirTemp("", "notch-crx")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	archive := filepath.Join(scratch, "x.zip")
	if err := os.WriteFile(archive, zip, 0o600); err != nil {
		return err
	}
	out := filepath.Join(scratch, "out")
	if err := exec.Command("/usr/bin/ditto", "-x", "-k", archive, out).Run(); err != nil {
		return ErrUnpack
	}
	if _, err := os.Stat(filepath.Join(out, "manifest.json")); err != nil {
		return ErrUnpack
	}
	link := false
	_ = filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			link = true
		}
		return nil
	})
	if link {
		return ErrUnpack
	}
	_ = filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	if err := os.MkdirAll(filepath.Dir(folder), 0o700); err != nil {
		return err
	}
	old := folder + ".old"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(folder); err == nil {
		if err := os.Rename(folder, old); err != nil { // the old one stays until the new is in place (R-WEB-172)
			return err
		}
	}
	if err := os.Rename(out, folder); err != nil {
		_ = os.Rename(old, folder)
		return err
	}
	return os.RemoveAll(old)
}
