package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProviderBodiesAndSignatures(t *testing.T) {
	for _, kind := range []string{"wecom", "dingtalk", "feishu", "serverchan", "serverchan3", "webhook"} {
		t.Run(kind, func(t *testing.T) {
			secret := "fixture-sign"
			typ := kind
			if kind == "serverchan" {
				secret = "SCTtestkey"
			}
			if kind == "serverchan3" {
				typ = "serverchan"
				secret = "sctp123tTestkey"
			}
			ch := domain.Channel{ID: "ch", Name: "fixture", Type: typ, Enabled: true, URL: "https://example.com/hook", Secret: secret, Method: "PUT"}
			n := New(secure.Network{Timeout: time.Second})
			n.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(r.Body)
				if r.Header.Get("X-Tingfeng-Delivery-ID") != "delivery-id" {
					t.Error("delivery id missing")
				}
				switch kind {
				case "wecom":
					var v struct {
						Msgtype string
						Text    struct{ Content string }
					}
					if json.Unmarshal(b, &v) != nil || v.Msgtype != "text" || v.Text.Content != "hello 中文" {
						t.Error("invalid wecom body")
					}
				case "dingtalk":
					q := r.URL.Query()
					mac := hmac.New(sha256.New, []byte(secret))
					mac.Write([]byte(q.Get("timestamp") + "\n" + secret))
					if q.Get("sign") != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
						t.Error("invalid DingTalk signature")
					}
				case "feishu":
					var v map[string]any
					if json.Unmarshal(b, &v) != nil {
						t.Fatal("invalid JSON")
					}
					ts, _ := v["timestamp"].(string)
					mac := hmac.New(sha256.New, []byte(ts+"\n"+secret))
					if ts == "" || v["sign"] != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
						t.Error("invalid Feishu signature")
					}
				case "serverchan", "serverchan3":
					v, _ := url.ParseQuery(string(b))
					if v.Get("title") != "title" || v.Get("desp") != "hello 中文" {
						t.Error("invalid form")
					}
					if kind == "serverchan3" && r.URL.Host != "123.push.ft07.com" {
						t.Error("invalid SC3 endpoint")
					}
				case "webhook":
					if r.Method != "PUT" || !json.Valid(b) {
						t.Error("invalid custom webhook")
					}
				}
				response := `{"errcode":0,"code":0}`
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
			})}
			if e := n.Send(context.Background(), ch, "", domain.Payload{Title: "title", Text: "hello 中文"}, "delivery-id"); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestFailureAcknowledgementAndTemplate(t *testing.T) {
	for _, body := range []string{`{"errcode":4001}`, `{"message":"ok"}`, `{"errcode":"0"}`, `not JSON`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
		n := New(secure.Network{AllowPrivate: true, Timeout: time.Second})
		c := domain.Channel{Name: "fixture", Type: "wecom", URL: server.URL}
		if e := n.Send(context.Background(), c, "", domain.Payload{Text: "text"}, "id"); e == nil {
			t.Error("unconfirmed delivery reported success", body)
		}
		server.Close()
	}
	p := domain.Payload{Text: "a\"b\n中文"}
	got, e := Render(`{"text": {{json .Text}}}`, p, 1000)
	if e != nil || !json.Valid([]byte(got)) {
		t.Fatal(got, e)
	}
	if _, e = Render("{{.Unknown}}", p, 1000); e == nil {
		t.Fatal("unknown variable accepted")
	}
	if _, e = Render("{{.Text}}{{.Text}}", p, 3); e == nil {
		t.Fatal("unbounded template")
	}
}
