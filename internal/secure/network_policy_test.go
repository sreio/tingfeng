package secure

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrivateSwitchBlocksReusedConnectionsAndRedirects(t *testing.T) {
	var policy atomic.Bool
	var calls, connections atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/redirect" {
			policy.Store(false)
			http.Redirect(w, r, "/data", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "fixture")
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	network := Network{PrivatePolicy: &policy, Timeout: time.Second}
	client := network.Client()
	defer client.CloseIdleConnections()
	get := func(path string, allowed bool) {
		t.Helper()
		response, err := client.Get(server.URL + path)
		if !allowed {
			if !errors.Is(err, ErrPrivate) {
				t.Fatalf("private request should be blocked: %v", err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	get("/data", false)
	policy.Store(true)
	get("/data", true)
	get("/data", true)
	if connections.Load() != 1 {
		t.Fatal("test did not establish a reusable connection")
	}
	policy.Store(false)
	get("/data", false)
	if calls.Load() != 2 {
		t.Fatal("disabled policy reused an internal connection")
	}
	if conn, err := network.DialContext(context.Background(), "tcp", strings.TrimPrefix(server.URL, "http://")); !errors.Is(err, ErrPrivate) {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatalf("direct connections must obey the switch: %v", err)
	}
	policy.Store(true)
	get("/redirect", false)
	if calls.Load() != 3 {
		t.Fatal("redirect bypassed the disabled policy")
	}
}
