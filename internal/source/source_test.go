package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
)

func TestJSONAndHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json" {
			if r.URL.Query().Get("symbol") != "gold" || r.Header.Get("Authorization") != "test-only" {
				w.WriteHeader(403)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"price":123.45,"updated":"2026-11-05T09:00:00+08:00"},"secret":"hidden"}`))
		} else {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><div class="price" data-stock="7"> ¥ 123.45 <script>doNotInclude()</script></div></body></html>`))
		}
	}))
	defer srv.Close()
	f := New(secure.Network{AllowPrivate: true, Timeout: time.Second}, 2<<20)
	s := domain.Source{Type: "api", URL: srv.URL + "/json", Method: "GET", Path: "data.price", TimePath: "/data/updated", Headers: map[string]string{"Authorization": "test-only"}, Query: map[string]string{"symbol": "gold"}}
	out, e := f.Fetch(context.Background(), s)
	if e != nil {
		t.Fatal(e)
	}
	if out.Observation.Display != "123.45" || out.Observation.SourceAt == 0 {
		t.Fatal(out.Observation)
	}
	for _, field := range out.Fields {
		if field.Path == "/secret" {
			t.Fatal("secret offered as field suggestion")
		}
	}
	s = domain.Source{Type: "page", URL: srv.URL, Method: "GET", Selector: ".price"}
	out, e = f.Fetch(context.Background(), s)
	if e != nil || out.Observation.Display != "¥ 123.45" {
		t.Fatal(out.Observation, e)
	}
	s.Attribute = "data-stock"
	out, e = f.Fetch(context.Background(), s)
	if e != nil || out.Observation.Display != "7" {
		t.Fatal(out.Observation, e)
	}
	s.Selector = "??"
	if _, e = f.Fetch(context.Background(), s); e == nil {
		t.Fatal("invalid CSS accepted")
	}
}
func TestFeedsAndIdentity(t *testing.T) {
	rss := []byte(`<rss><channel><item><guid>a</guid><title>One</title><link>https://example.com/a</link><pubDate>Thu, 05 Nov 2026 09:00:00 +0800</pubDate></item><item><guid>a</guid><title>Repeated</title></item></channel></rss>`)
	items, e := parseFeed(rss)
	if e != nil || len(items) != 1 || items[0].PublishedAt == 0 {
		t.Fatal(items, e)
	}
	atom := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>b</id><title>Two</title><link href="https://example.com/b"/><updated>2026-11-05T09:00:00+08:00</updated></entry></feed>`)
	items, e = parseFeed(atom)
	if e != nil || len(items) != 1 || items[0].Link != "https://example.com/b" {
		t.Fatal(items, e)
	}
	if _, e = parseFeed([]byte("<html/>")); e == nil {
		t.Fatal("HTML treated as RSS")
	}
}
func TestBodyLimitAndPrivateNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 1025)) }))
	defer srv.Close()
	s := domain.Source{Type: "api", URL: srv.URL, Method: "GET"}
	f := New(secure.Network{Timeout: time.Second}, 1024)
	if _, e := f.Fetch(context.Background(), s); e == nil {
		t.Fatal("loopback allowed by default")
	}
	f = New(secure.Network{AllowPrivate: true, Timeout: time.Second}, 1024)
	if _, e := f.Fetch(context.Background(), s); e == nil {
		t.Fatal("oversized response accepted")
	}
}
func TestExtractArraysAndPointer(t *testing.T) {
	value := map[string]any{"a/b": []any{map[string]any{"~key": "value"}}}
	got, e := Extract(value, "/a~1b/0/~0key")
	if e != nil || got != "value" {
		t.Fatal(got, e)
	}
	if _, e = Extract(value, "/a~1b/99"); e == nil {
		t.Fatal("invalid index accepted")
	}
}
