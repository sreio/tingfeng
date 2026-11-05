package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/notify"
	"tingfeng/internal/secure"
	"tingfeng/internal/source"
	"tingfeng/internal/storage"
)

func TestNetworkSettingPersistsAndAppliesToCollectionAndDelivery(t *testing.T) {
	store, err := storage.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.WriteHeader(http.StatusNoContent)
		} else {
			_, _ = io.WriteString(w, `{"price":123}`)
		}
	}))
	defer fixture.Close()
	create := func(fallback bool) *Engine {
		t.Helper()
		n := secure.Network{AllowPrivate: fallback, PrivatePolicy: new(atomic.Bool), Timeout: time.Second}
		e, err := New(store, source.New(n, 2<<20), notify.New(n), 1)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(e.Fetcher.Client.CloseIdleConnections)
		t.Cleanup(e.Sender.Client.CloseIdleConnections)
		return e
	}
	ctx := context.Background()
	check := func(e *Engine, allowed bool) {
		t.Helper()
		if e.Sender.Network.AllowsPrivate() != allowed {
			t.Fatal("reported policy disagrees with configuration")
		}
		_, fetchErr := e.Fetcher.Fetch(ctx, domain.Source{Type: "api", URL: fixture.URL, Method: "GET", Path: "price"})
		sendErr := e.Sender.Send(ctx, domain.Channel{Type: "webhook", URL: fixture.URL, Method: "POST"}, "", domain.Payload{Text: "fixture"}, "fixture")
		if (fetchErr == nil) != allowed || (sendErr == nil) != allowed {
			t.Fatalf("policy %v: fetch=%v send=%v", allowed, fetchErr, sendErr)
		}
	}
	e := create(true)
	check(e, true)
	set := func(enabled bool) {
		t.Helper()
		c := e.Config()
		c.Settings.AllowPrivate = &enabled
		if _, err := e.SaveConfig(ctx, c); err != nil {
			t.Fatal(err)
		}
		check(e, enabled)
	}
	set(false)
	set(true)
	set(false)
	stale := e.Config()
	stale.Revision--
	on := true
	stale.Settings.AllowPrivate = &on
	if _, err := e.SaveConfig(ctx, stale); err != storage.ErrConflict {
		t.Fatalf("stale settings were accepted: %v", err)
	}
	check(e, false)
	olderClient := e.Config()
	olderClient.Settings.AllowPrivate = nil
	if _, err := e.SaveConfig(ctx, olderClient); err != nil {
		t.Fatal(err)
	}
	check(e, false)
	// Saved false must override a true environment default after a restart.
	restarted := create(true)
	check(restarted, false)
}
