package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
)

func TestBarkPush(t *testing.T) {
	p := domain.Payload{Title: "金价提醒", Text: "买入：904.41\n卖出：883.90"}
	for _, tt := range []struct {
		name, server, endpoint, ack string
		status                      int
		success                     bool
	}{
		{"official", "", "https://api.day.app/push", `{"code":200,"message":"success"}`, 200, true},
		{"self-hosted", "https://bark.example:8443/", "https://bark.example:8443/push", `{"code":200}`, 200, true},
		{"device rejected", "", "https://api.day.app/push", `{"code":400,"message":"fixture-device-key"}`, 200, false},
		{"unconfirmed", "", "https://api.day.app/push", `{"message":"success"}`, 200, false},
		{"invalid confirmation", "", "https://api.day.app/push", `{"code":"200"}`, 200, false},
		{"HTTP failure", "", "https://api.day.app/push", `{"code":200}`, 403, false},
		{"redirect", "https://bark.example", "https://bark.example/push", "", 307, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ch := domain.Channel{Type: "bark", BarkServer: tt.server, Secret: "fixture-device-key"}
			calls := 0
			s := New(secure.Network{Timeout: time.Second})
			s.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "POST" || r.URL.String() != tt.endpoint || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Tingfeng-Delivery-ID") != "fixture-id" {
					t.Fatal("incorrect endpoint or request headers")
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["device_key"] != ch.Secret || body["title"] != p.Title || body["body"] != p.Text {
					t.Fatal("incorrect device, title or message")
				}
				h := make(http.Header)
				if tt.status == 307 {
					h.Set("Location", "https://untrusted.example/forward")
				}
				return &http.Response{StatusCode: tt.status, Header: h, Body: io.NopCloser(strings.NewReader(tt.ack))}, nil
			})
			err := s.Send(context.Background(), ch, "", p, "fixture-id")
			if (err == nil) != tt.success || calls != 1 || (err != nil && strings.Contains(err.Error(), ch.Secret)) {
				t.Fatal("unexpected delivery outcome, redirect or device key disclosure")
			}
		})
	}
}
