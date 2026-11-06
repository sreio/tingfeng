package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
)

func TestTelegramDestinationsAndText(t *testing.T) {
	ch := domain.Channel{Type: "telegram", Secret: "42:fixture_only"}
	message := "金价 904.41 元/克\n<买入> & [卖出] 🙂"
	long := strings.Repeat("金", 4094) + "🙂尾"
	for _, tt := range []struct{ name, address, wireID, text, want string }{
		{"private", "123456", "123456", message, message},
		{"group", "-1001234567890", "-1001234567890", long, long},
		{"channel", "@example_channel", `"@example_channel"`, long + "额外", strings.Repeat("金", 4094) + "🙂…"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := New(secure.Network{Timeout: time.Second})
			calls := 0
			s.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "POST" || r.URL.Scheme != "https" || r.URL.Host != "api.telegram.org" || r.URL.Path != "/bot"+ch.Secret+"/sendMessage" || r.URL.RawQuery != "" {
					t.Fatal("unexpected Telegram endpoint")
				}
				var body struct {
					ChatID    json.RawMessage `json:"chat_id"`
					Text      string          `json:"text"`
					ParseMode string          `json:"parse_mode"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if string(body.ChatID) != tt.wireID || body.Text != tt.want || body.ParseMode != "" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Tingfeng-Delivery-ID") != "fixture-id" {
					t.Fatal("unexpected recipient or formatted content")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":7}}`))}, nil
			})
			if err := s.Send(context.Background(), ch, tt.address, domain.Payload{Text: tt.text}, "fixture-id"); err != nil || calls != 1 {
				t.Fatal("Telegram delivery failed", err)
			}
		})
	}
}

func TestTelegramRejectsUnconfirmedMessagesWithoutLeakingToken(t *testing.T) {
	ch := domain.Channel{Type: "telegram", Secret: "42:fixture_only"}
	for _, tt := range []struct {
		status int
		body   string
	}{
		{200, `{"ok":false,"error_code":400,"description":"42:fixture_only"}`},
		{200, `{"ok":"true"}`}, {200, `{"errcode":0}`}, {200, "not-json"},
		{403, "42:fixture_only"}, {429, "42:fixture_only"}, {307, ""}, {0, ""},
	} {
		s := New(secure.Network{Timeout: time.Second})
		calls := 0
		s.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if tt.status == 0 {
				return nil, errors.New("fixture failure " + r.URL.String())
			}
			headers := make(http.Header)
			if tt.status == 307 {
				headers.Set("Location", "https://untrusted.example/forward")
			}
			return &http.Response{StatusCode: tt.status, Header: headers, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
		})
		err := s.Send(context.Background(), ch, "123", domain.Payload{Text: "test"}, "fixture-id")
		if err == nil || strings.Contains(err.Error(), ch.Secret) || calls != 1 {
			t.Fatal("invalid acknowledgement, redirect or credential exposure", tt.status)
		}
	}
}
