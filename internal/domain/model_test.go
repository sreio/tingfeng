package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"tingfeng/internal/domain"
	"tingfeng/internal/testutil"
)

func TestSecretsAreRedactedPreservedAndNotForwardedToNewHosts(t *testing.T) {
	original := testutil.Config("https://a.example/price?key=fixture-value", "https://notify.example/send?key=fixture-value")
	original.Monitors[0].Source.Headers = map[string]string{"Authorization": "fixture-value"}
	original.Channels[0].Secret = "fixture-value"
	original.Channels[0].Password = "fixture-value"
	original.Channels[0].Headers = map[string]string{"X-Api-Key": "fixture-value"}
	sanitized := domain.Redacted(original)
	raw, _ := json.Marshal(sanitized)
	if strings.Contains(string(raw), "fixture-value") {
		t.Fatal("secret exposed")
	}
	domain.MergeSecrets(&sanitized, original)
	if sanitized.Monitors[0].Source.URL != original.Monitors[0].Source.URL || sanitized.Channels[0].URL != original.Channels[0].URL {
		t.Fatal("editing non-secret metadata erased secrets")
	}
	next := domain.Redacted(original)
	next.Monitors[0].Source.URL = "https://different.example/price"
	domain.MergeSecrets(&next, original)
	if len(next.Monitors[0].Source.Headers) != 0 {
		t.Fatal("old authorization forwarded to a new origin")
	}
	copied := domain.Redacted(original)
	copied.Monitors[0].CopyFrom = "mon"
	copied.Monitors[0].ID = "copy"
	domain.MergeSecrets(&copied, original)
	if copied.Monitors[0].Source.Headers["Authorization"] != "fixture-value" || copied.Monitors[0].CopyFrom != "" {
		t.Fatal("copy could not retain stored request credentials")
	}
}
func TestTargetsDeduplicateAndUnavailableChannelsDoNotBlockConfig(t *testing.T) {
	c := testutil.Config("https://example.com", "https://example.com/hook")
	targets := c.Targets([]string{"me:method", "me:method"})
	if len(targets) != 1 {
		t.Fatal(targets)
	}
	c.Channels[0].Enabled = false
	if len(c.Targets([]string{"me:method"})) != 0 {
		t.Fatal("disabled target available")
	}
	if e := domain.Validate(c); e != nil {
		t.Fatal("could not disable an in-use channel", e)
	}
	c.Recipients = nil
	if domain.Validate(c) == nil {
		t.Fatal("dangling recipient reference accepted")
	}
}

func TestTelegramRecipientValidationAndSecretRoundTrip(t *testing.T) {
	c := testutil.Config("https://example.com", "")
	ch := &c.Channels[0]
	ch.Type = "telegram"
	ch.Secret = "42:fixture_only"
	method := &c.Recipients[0].Methods[0]
	for _, address := range []string{"1234567890", "-1001234567890", "@example_channel"} {
		method.Address = address
		if err := domain.Validate(c); err != nil {
			t.Fatal("valid Telegram recipient rejected", err)
		}
	}
	for _, address := range []string{"", "0", "12.34", "99999999999999999999", "@", "https://t.me/example", "user@example.com"} {
		method.Address = address
		if domain.Validate(c) == nil {
			t.Fatal("invalid Telegram recipient accepted")
		}
	}
	method.Address = "-1001234567890"
	public := domain.Redacted(c)
	raw, _ := json.Marshal(public)
	if strings.Contains(string(raw), ch.Secret) || public.Channels[0].Secret != "" {
		t.Fatal("bot token exposed in config or backup")
	}
	domain.MergeSecrets(&public, c)
	if public.Channels[0].Secret != ch.Secret {
		t.Fatal("metadata edit discarded the saved token")
	}
	converted := domain.Redacted(c)
	converted.Channels[0].Type = "webhook"
	converted.Channels[0].URL = "https://other.example/send"
	domain.MergeSecrets(&converted, c)
	if converted.Channels[0].Secret != "" {
		t.Fatal("channel type change inherited a Telegram token")
	}
	for _, invalid := range []string{"", "invalid", "42:bad/path", "42:bad?key=x"} {
		ch.Secret = invalid
		if domain.ValidateChannel(*ch, true) == nil {
			t.Fatal("invalid token accepted")
		}
	}
	ch.Enabled = false
	ch.Secret = ""
	if err := domain.Validate(c); err != nil {
		t.Fatal("restored disabled channel requires a token", err)
	}
}
