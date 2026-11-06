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
	"sync/atomic"
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
	n := secure.Network{PrivatePolicy: new(atomic.Bool), Timeout: time.Second}
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
	enabled := true
	c.Settings.AllowPrivate = &enabled
	request("PUT", "/api/config", c, 403, srv.URL, false)
	request("PUT", "/api/config", c, 403, "https://evil.example", true)
	if n.AllowsPrivate() {
		t.Fatal("unauthorized request changed network access")
	}
	for _, invalid := range []int{0, domain.MaxDeliveryConcurrency + 1} {
		c.Settings.DeliveryConcurrency = invalid
		request("PUT", "/api/config", c, 400, srv.URL, true)
	}
	c.Settings.DeliveryConcurrency = 3
	out := request("PUT", "/api/config", c, 200, srv.URL, true)
	var system struct {
		AllowPrivate bool `json:"allowPrivate"`
	}
	json.Unmarshal(request("GET", "/api/system", nil, 200, "", false), &system)
	if !system.AllowPrivate {
		t.Fatal("system status did not report live policy")
	}
	if bytes.Contains(out, []byte("fixture-request-secret")) || bytes.Contains(out, []byte("fixture-channel-secret")) {
		t.Fatal("secret reflected")
	}
	request("PUT", "/api/config", c, 409, srv.URL, true)
	var got domain.Config
	json.Unmarshal(out, &got)
	disabled := false
	got.Settings.AllowPrivate = &disabled
	oldConfig := func(cfg domain.Config) map[string]any {
		raw, _ := json.Marshal(cfg)
		var old map[string]any
		if err := json.Unmarshal(raw, &old); err != nil {
			t.Fatal(err)
		}
		delete(old["settings"].(map[string]any), "deliveryConcurrency")
		return old
	}
	json.Unmarshal(request("PUT", "/api/config", oldConfig(got), 200, srv.URL, true), &got)
	if got.Settings.DeliveryConcurrency != 3 {
		t.Fatal("old client reset the saved delivery limit")
	}
	backup := request("GET", "/api/backup", nil, 200, "", false)
	if strings.Contains(string(backup), "fixture-request-secret") {
		t.Fatal("backup exported request secret")
	}
	var archive backupFile
	if e = json.Unmarshal(backup, &archive); e != nil {
		t.Fatal(e)
	}
	archive.Config.Settings.AllowPrivate = &enabled
	legacyBackup := map[string]any{"format": archive.Format, "version": archive.Version, "config": oldConfig(archive.Config)}
	restore := map[string]any{"backup": legacyBackup, "preview": true, "revision": got.Revision}
	request("POST", "/api/restore", restore, 200, srv.URL, true)
	if !eng.Config().Monitors[0].Enabled {
		t.Fatal("preview mutated config")
	}
	restore["preview"] = false
	request("POST", "/api/restore", restore, 200, srv.URL, true)
	imported := eng.Config()
	if imported.Settings.DeliveryConcurrency != 3 {
		t.Fatal("old backup reset the saved delivery limit")
	}
	if n.AllowsPrivate() {
		t.Fatal("restoring a backup widened internal network access")
	}
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
