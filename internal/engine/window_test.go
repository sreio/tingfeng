package engine

import (
	"context"
	"testing"
	"time"
	"tingfeng/internal/domain"
)

func TestMissedWindowAndPendingDeliveryStayQuiet(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	c := x.e.Config()
	c.Settings.TimeZone = "UTC"
	now := time.Now().UTC()
	r := &c.Monitors[0].Rules[0]
	r.Trigger = "periodic"
	r.Unit = "minute"
	r.Every = 1
	r.Timing = "window"
	r.WindowStart = now.Add(-2 * time.Hour).Format("15:04")
	r.WindowEnd = now.Add(-time.Hour).Format("15:04")
	if _, e := x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	rt, e := x.e.Check(ctx, "mon")
	if e != nil {
		t.Fatal(e)
	}
	rs := rt.Rules["rule"]
	rs.NextSend = now.Add(-90 * time.Minute).UnixMilli()
	rt.Rules["rule"] = rs
	c = x.e.Config()
	if e = x.s.SaveRun(ctx, rt, nil, nil, nil, false, c.Monitors[0].Storage); e != nil {
		t.Fatal(e)
	}
	if e = x.e.sendDue(ctx, "mon", now); e != nil {
		t.Fatal(e)
	}
	deliveries, e := x.s.Deliveries(ctx, "mon", "", 1, 25)
	if e != nil || deliveries.Total != 0 {
		t.Fatal("late catch-up escaped quiet window", e)
	}
	rt, _ = x.s.Runtime(ctx, "mon")
	if rt.Rules["rule"].NextSend <= now.UnixMilli() {
		t.Fatal("missed slot not advanced")
	}
	m := c.Monitors[0]
	target := c.Targets(m.Rules[0].Selections)[0]
	d := newDelivery(m, m.Rules[0], target, domain.Payload{Text: "fixture"}, now.UnixMilli(), "expired-slot")
	if e = x.s.SaveRun(ctx, rt, nil, nil, []domain.Delivery{d}, false, m.Storage); e != nil {
		t.Fatal(e)
	}
	if !x.e.DeliverOne(ctx) {
		t.Fatal("queue not processed")
	}
	if x.sent.Load() != 0 {
		t.Fatal("queued retry escaped quiet window")
	}
	deliveries, _ = x.s.Deliveries(ctx, "mon", "cancelled", 1, 25)
	if deliveries.Total != 1 {
		t.Fatal("expired delivery not cancelled")
	}
}
func TestOvernightWindow(t *testing.T) {
	r := domain.Rule{Trigger: "periodic", Timing: "window", WindowStart: "22:00", WindowEnd: "02:00"}
	for _, v := range []struct {
		hour   int
		inside bool
	}{{21, false}, {22, true}, {0, true}, {1, true}, {2, false}, {12, false}} {
		if withinWindow(r, time.Date(2026, 11, 5, v.hour, 0, 0, 0, time.UTC), time.UTC) != v.inside {
			t.Fatal(v)
		}
	}
}
