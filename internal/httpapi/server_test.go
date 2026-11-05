package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"tingfeng/internal/auth"
	"tingfeng/internal/config"
	"tingfeng/internal/domain"
	"tingfeng/internal/engine"
	"tingfeng/internal/notify"
	"tingfeng/internal/secure"
	"tingfeng/internal/source"
	"tingfeng/internal/storage"
	"tingfeng/internal/testutil"
)

func TestAuthenticatedWorkflowAndRestore(t *testing.T) {
	store, e := storage.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	n := secure.Network{Timeout: time.Second}
	eng, e := engine.New(store, source.New(n, 2<<20), notify.New(n), 1)
	if e != nil {
		t.Fatal(e)
	}
	s := New(eng, config.Config{}, fstest.MapFS{"index.html": {Data: []byte("<h1>听风</h1>")}})
	if e = s.Auth.EnsureBootstrap(context.Background()); e != nil {
		t.Fatal(e)
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	request := func(method, path string, body any, status int, origin string, withCSRF bool) []byte {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if withCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != status {
			t.Fatalf("%s %s: status %d expected %d: %.500s", method, path, res.StatusCode, status, b)
		}
		if res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("security headers missing")
		}
		return b
	}
	request("GET", "/api/config", nil, 401, "", false)
	token, e := os.ReadFile(filepath.Join(store.Dir, "setup.token"))
	if e != nil {
		t.Fatal(e)
	}
	body := map[string]string{"token": string(token), "password": "fixture-password"}
	request("POST", "/api/auth/setup", body, 403, "https://different.example", false)
	var session auth.Session
	json.Unmarshal(request("POST", "/api/auth/setup", body, 200, srv.URL, false), &session)
	csrf = session.CSRF
	if csrf == "" {
		t.Fatal("missing CSRF")
	}
	c := testutil.Config("https://example.com/data?token=fixture-request-secret", "https://example.com/hook")
	c.Channels[0].Secret = "fixture-channel-secret"
	request("PUT", "/api/config", c, 403, srv.URL, false)
	request("PUT", "/api/config", c, 403, "https://evil.example", true)
	out := request("PUT", "/api/config", c, 200, srv.URL, true)
	if bytes.Contains(out, []byte("fixture-request-secret")) || bytes.Contains(out, []byte("fixture-channel-secret")) {
		t.Fatal("secret reflected")
	}
	request("PUT", "/api/config", c, 409, srv.URL, true)
	var got domain.Config
	json.Unmarshal(out, &got)
	backup := request("GET", "/api/backup", nil, 200, "", false)
	if strings.Contains(string(backup), "fixture-request-secret") {
		t.Fatal("backup exported request secret")
	}
	var archive backupFile
	if e = json.Unmarshal(backup, &archive); e != nil {
		t.Fatal(e)
	}
	restore := map[string]any{"backup": archive, "preview": true, "revision": got.Revision}
	request("POST", "/api/restore", restore, 200, srv.URL, true)
	if !eng.Config().Monitors[0].Enabled {
		t.Fatal("preview mutated config")
	}
	restore["preview"] = false
	request("POST", "/api/restore", restore, 200, srv.URL, true)
	imported := eng.Config()
	if imported.Monitors[0].Enabled || !imported.Monitors[0].NeedsReview || imported.Channels[0].Enabled || imported.Channels[0].Secret != "" {
		t.Fatal("restore unsafe")
	}
	if strings.Contains(imported.Monitors[0].Source.URL, "token=") {
		t.Fatal("restore preserved URL secret")
	}
	request("POST", "/api/auth/password", map[string]string{"current": "fixture-password", "password": "replacement-password"}, 200, srv.URL, true)
	request("GET", "/api/config", nil, 401, "", false)
	request("GET", "/healthz", nil, 200, "", false)
	request("GET", "/nonexistent/route", nil, 200, "", false)
	request("GET", "/missing.js", nil, 404, "", false)
}
