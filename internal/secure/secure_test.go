package secure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptionIntegrityAndLostKey(t *testing.T) {
	dir := t.TempDir()
	box, e := OpenBox(dir)
	if e != nil {
		t.Fatal(e)
	}
	raw := []byte("fixture-value")
	encrypted, e := box.Seal(raw)
	if e != nil {
		t.Fatal(e)
	}
	plain, e := box.Open(encrypted)
	if e != nil || string(plain) != string(raw) {
		t.Fatal(e)
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, e = box.Open(encrypted); e == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	_ = os.Remove(filepath.Join(dir, "master.key"))
	_ = os.WriteFile(filepath.Join(dir, "tingfeng.db"), []byte("existing"), 0600)
	if _, e = OpenBox(dir); e == nil {
		t.Fatal("missing key silently replaced")
	}
}
func TestPrivateAndReservedAddresses(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.1.1", "172.16.0.1", "192.168.1.2", "169.254.169.254", "100.64.1.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "0.0.0.0", "192.0.2.1"} {
		if PublicAddress(netip.MustParseAddr(s)) {
			t.Error("address allowed", s)
		}
	}
	if !PublicAddress(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public address blocked")
	}
}
func TestCrossOriginRedirectDropsSecrets(t *testing.T) {
	got := ""
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header.Get("X-Api-Key"); w.WriteHeader(200) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	c := (Network{AllowPrivate: true, Timeout: time.Second}).Client()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", source.URL, nil)
	req.Header.Set("X-Api-Key", "test-fixture")
	resp, e := c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	_ = resp.Body.Close()
	if got != "" {
		t.Fatal("credential header followed cross-origin redirect")
	}
}
