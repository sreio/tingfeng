package engine

import (
	"encoding/json"
	"testing"
	"time"
	"tingfeng/internal/domain"
)

func clock(t *testing.T, s string) time.Time {
	t.Helper()
	v, e := time.Parse(time.RFC3339, s)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func rule() domain.Rule {
	return domain.Rule{ID: "r1", Trigger: "periodic", Every: 1, Unit: "hour", Timing: "always", Time: "09:00", WindowStart: "09:00", WindowEnd: "18:00", Cooldown: 30, Operator: "gt", Threshold: 130}
}
func TestSchedule(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	anchor := clock(t, "2026-11-04T23:25:00+08:00")
	after := clock(t, "2026-11-05T00:40:00+08:00")
	r := rule()
	if got := NextSend(r, anchor, after, loc); got.Format(time.RFC3339) != "2026-11-05T01:25:00+08:00" {
		t.Fatal(got)
	}
	r.Every = 2
	r.Unit = "day"
	if got := NextSend(r, anchor, after, loc); got.Format(time.RFC3339) != "2026-11-06T23:25:00+08:00" {
		t.Fatal(got)
	}
	r = rule()
	r.Timing = "window"
	r.Every = 30
	r.Unit = "minute"
	r.WindowStart = "23:30"
	r.WindowEnd = "01:00"
	after = clock(t, "2026-11-05T00:05:00+08:00")
	for _, expected := range []string{"2026-11-05T00:30:00+08:00", "2026-11-05T23:30:00+08:00", "2026-11-06T00:00:00+08:00"} {
		after = NextSend(r, anchor, after, loc)
		if after.Format(time.RFC3339) != expected {
			t.Fatal(after, expected)
		}
	}
	r = rule()
	r.Trigger = "scheduled"
	after = clock(t, "2026-11-05T09:00:00+08:00")
	if got := NextSend(r, anchor, after, loc); got.Format(time.RFC3339) != "2026-11-06T09:00:00+08:00" {
		t.Fatal(got)
	}
}
func TestInvalidCadence(t *testing.T) {
	r := rule()
	r.Every = 0
	if !NextSend(r, time.Now(), time.Now(), time.UTC).IsZero() {
		t.Fatal("zero period accepted")
	}
	r = rule()
	r.Timing = "window"
	r.WindowEnd = r.WindowStart
	if domain.ValidateRule(r) == nil {
		t.Fatal("zero-length window accepted")
	}
}
func TestCrossingCooldownAndBaseline(t *testing.T) {
	r := rule()
	r.Trigger = "threshold"
	values := []json.RawMessage{[]byte("129"), []byte("131"), []byte("132"), []byte("128"), []byte("131")}
	out := Simulate(r, values, 5)
	if out[0].Send || !out[1].Send || out[2].Send || out[4].Send || out[4].Reason != "冷却中，还剩 15 分钟" {
		t.Fatalf("unexpected transitions: %+v", out)
	}
	r.Cooldown = 0
	out = Simulate(r, values, 5)
	if !out[4].Send {
		t.Fatal("second crossing suppressed without cooldown")
	}
	if ValueChanged([]byte(`{"a":1,"b":2}`), []byte(`{ "b":2, "a":1 }`)) {
		t.Fatal("JSON key order triggered a change")
	}
	r.Threshold = 130
	r.Operator = "lt"
	if !Crossed(r, []byte("131"), []byte("129")) {
		t.Fatal("downward crossing missing")
	}
}
func TestRSSDedupTrial(t *testing.T) {
	r := rule()
	r.Trigger = "new"
	r.Cooldown = 0
	out := Simulate(r, []json.RawMessage{[]byte(`"old"`), []byte(`"a"`), []byte(`"a"`), []byte(`"b"`)}, 5)
	if out[0].Send || !out[1].Send || out[2].Send || !out[3].Send {
		t.Fatal(out)
	}
}
