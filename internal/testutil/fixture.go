package testutil

import (
	"tingfeng/internal/domain"
)

func Config(sourceURL, hookURL string) domain.Config {
	c := domain.EmptyConfig()
	c.Channels = []domain.Channel{{ID: "ch", Name: "Test hook", Type: "webhook", Enabled: true, URL: hookURL, Method: "POST"}}
	c.Recipients = []domain.Recipient{{ID: "me", Name: "Test user", Kind: "person", Methods: []domain.Method{{ID: "method", ChannelID: "ch", Default: true}}}}
	c.Monitors = []domain.Monitor{{ID: "mon", Name: "Test monitor", Enabled: true, Interval: 1,
		Source:  domain.Source{Type: "api", URL: sourceURL, Method: "GET", Path: "price", Unit: "元/克"},
		Rules:   []domain.Rule{{ID: "rule", Trigger: "change", Operator: "gt", Threshold: 130, Cooldown: 0, Every: 1, Unit: "hour", Timing: "always", WindowStart: "09:00", WindowEnd: "18:00", Time: "09:00", Selections: []string{"me:method"}}},
		Storage: domain.Storage{History: true, Days: 7, MaxRows: 10, LogLevel: "error", LogDays: 3, Stages: map[string]bool{"fetch": true, "evaluate": true, "send": true}}}}
	return c
}
