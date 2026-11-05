package engine

import (
	"context"
	"fmt"
	"testing"
	"time"
	"tingfeng/internal/domain"
)

func TestScheduleReenableAndTimezone(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	c := x.e.Config()
	c.Monitors[0].Rules[0].Trigger = "periodic"
	if _, e := x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	rt, e := x.e.Check(ctx, "mon")
	if e != nil {
		t.Fatal(e)
	}
	old := rt.Rules["rule"]
	if old.NextSend-old.Anchor != int64(time.Hour/time.Millisecond) {
		t.Fatal("interval anchor", old)
	}
	c = x.e.Config()
	c.Monitors[0].Enabled = false
	if _, e = x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	c = x.e.Config()
	c.Monitors[0].Enabled = true
	if _, e = x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	rt, e = x.e.Check(ctx, "mon")
	if e != nil {
		t.Fatal(e)
	}
	state := rt.Rules["rule"]
	m, _ := x.e.Config().FindMonitor("mon")
	if state.Anchor != m.ActivatedAt || state.NextSend != m.ActivatedAt+int64(time.Hour/time.Millisecond) {
		t.Fatal("reenable failed to restart interval")
	}
	c = x.e.Config()
	c.Monitors[0].Rules[0].Trigger = "scheduled"
	c.Monitors[0].Rules[0].Time = "09:00"
	c.Settings.TimeZone = "UTC"
	if _, e = x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	rt, e = x.e.Check(ctx, "mon")
	if e != nil {
		t.Fatal(e)
	}
	if time.UnixMilli(rt.Rules["rule"].NextSend).UTC().Hour() != 9 {
		t.Fatal("wrong UTC time")
	}
	c = x.e.Config()
	c.Settings.TimeZone = "Asia/Shanghai"
	if _, e = x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	rt, e = x.e.Check(ctx, "mon")
	if e != nil {
		t.Fatal(e)
	}
	if time.UnixMilli(rt.Rules["rule"].NextSend).UTC().Hour() != 1 {
		t.Fatal("timezone edit did not reset schedule")
	}
}
func TestFullQueueKeepsComparisonBaseline(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	rt, e := x.e.Check(ctx, "mon")
	if e != nil {
		t.Fatal(e)
	}
	ds := make([]domain.Delivery, 1000)
	for i := range ds {
		ds[i] = domain.Delivery{ID: fmt.Sprint(i), Key: fmt.Sprint(i), MonitorID: "mon", CreatedAt: time.Now().UnixMilli(), NextAttempt: time.Now().Add(time.Hour).UnixMilli()}
	}
	c := x.e.Config()
	if e = x.s.SaveRun(ctx, rt, nil, nil, ds, false, c.Monitors[0].Storage); e != nil {
		t.Fatal(e)
	}
	x.price.Store(131)
	if _, e = x.e.Check(ctx, "mon"); e == nil {
		t.Fatal("full queue accepted")
	}
	got, e := x.s.Runtime(ctx, "mon")
	if e != nil || got.Latest.Display != "129" {
		t.Fatal("failed enqueue advanced comparison baseline", e)
	}
	if _, e = x.s.DB.Exec("DELETE FROM deliveries"); e != nil {
		t.Fatal(e)
	}
	if _, e = x.e.Check(ctx, "mon"); e != nil {
		t.Fatal(e)
	}
	p, e := x.s.Deliveries(ctx, "mon", "pending", 1, 25)
	if e != nil || p.Total != 1 {
		t.Fatal("change notification lost after recovery", e)
	}
}
