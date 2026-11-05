package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"tingfeng/internal/notify"
	"tingfeng/internal/secure"
	"tingfeng/internal/source"
	"tingfeng/internal/storage"
	"tingfeng/internal/testutil"
)

type fixture struct {
	e      *Engine
	s      *storage.Store
	price  atomic.Int64
	status atomic.Int64
	sent   atomic.Int64
	calls  atomic.Int64
}

func setup(t *testing.T) *fixture {
	t.Helper()
	x := &fixture{}
	x.price.Store(129)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		x.calls.Add(1)
		if status := x.status.Load(); status != 0 {
			w.WriteHeader(int(status))
			return
		}
		_, _ = fmt.Fprintf(w, `{"price":%d}`, x.price.Load())
	}))
	t.Cleanup(src.Close)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { x.sent.Add(1); w.WriteHeader(204) }))
	t.Cleanup(hook.Close)
	var err error
	x.s, err = storage.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = x.s.Close() })
	n := secure.Network{AllowPrivate: true, Timeout: time.Second}
	x.e, err = New(x.s, source.New(n, 2<<20), notify.New(n), 2)
	if err != nil {
		t.Fatal(err)
	}
	c := testutil.Config(src.URL, hook.URL)
	if _, err = x.e.SaveConfig(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return x
}
func TestChangeQueueAndRetention(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	if _, e := x.e.Check(ctx, "mon"); e != nil {
		t.Fatal(e)
	}
	p, _ := x.s.Deliveries(ctx, "mon", "", 1, 25)
	if p.Total != 0 {
		t.Fatal("first collection notified")
	}
	x.price.Store(131)
	if _, e := x.e.Check(ctx, "mon"); e != nil {
		t.Fatal(e)
	}
	p, _ = x.s.Deliveries(ctx, "mon", "", 1, 25)
	if p.Total != 1 {
		t.Fatal("change not queued", p.Total)
	}
	if !x.e.DeliverOne(ctx) || x.sent.Load() != 1 {
		t.Fatal("not delivered")
	}
	p, _ = x.s.Deliveries(ctx, "mon", "success", 1, 25)
	if p.Total != 1 {
		t.Fatal(p)
	}
	c := x.e.Config()
	c.Monitors[0].Storage.MaxRows = 1
	if _, e := x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	if _, e := x.e.Check(ctx, "mon"); e != nil {
		t.Fatal(e)
	}
	history, e := x.s.Records(ctx, "mon", 1, 25)
	if e != nil || history.Total != 1 {
		t.Fatal(history.Total, e)
	}
	c = x.e.Config()
	c.Monitors[0].Storage.History = false
	if _, e = x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	x.price.Store(132)
	if _, e = x.e.Check(ctx, "mon"); e != nil {
		t.Fatal(e)
	}
	history, _ = x.s.Records(ctx, "mon", 1, 25)
	if history.Total != 1 {
		t.Fatal("disabling history destroyed prior data or added a new row")
	}
	rt, _ := x.s.Runtime(ctx, "mon")
	if rt.Latest.Display != "132" {
		t.Fatal("latest stopped updating with history off")
	}
}
func TestFailureKeepsBaselineAndRecovers(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	_, _ = x.e.Check(ctx, "mon")
	x.status.Store(503)
	rt, e := x.e.Check(ctx, "mon")
	if e != nil {
		t.Fatal(e)
	}
	if rt.Latest.Display != "129" || rt.Failures != 1 || rt.Incident == nil {
		t.Fatal(rt)
	}
	_, _ = x.e.Check(ctx, "mon")
	logs, e := x.s.Logs(ctx, "mon", "fetch", 1, 25)
	if e != nil || logs.Total != 2 {
		t.Fatal(logs, e)
	}
	c := x.e.Config()
	c.Monitors[0].Storage.LogLevel = "off"
	_, _ = x.e.SaveConfig(ctx, c)
	_, _ = x.e.Check(ctx, "mon")
	logs, _ = x.s.Logs(ctx, "mon", "fetch", 1, 25)
	if logs.Total != 2 {
		t.Fatal("disabled logs kept accumulating")
	}
	x.status.Store(0)
	rt, e = x.e.Check(ctx, "mon")
	if e != nil || rt.Failures != 0 || !rt.Incident.Resolved {
		t.Fatal(rt, e)
	}
	p, _ := x.s.Deliveries(ctx, "mon", "", 1, 25)
	if p.Total != 0 {
		t.Fatal("failure or unchanged recovery notified")
	}
}
func TestPeriodicIsIndependentDurableAndNotReplayed(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	c := x.e.Config()
	c.Monitors[0].Rules[0].Trigger = "periodic"
	c.Monitors[0].Rules[0].Unit = "minute"
	c.Monitors[0].Rules[0].Cooldown = 1440
	if _, e := x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	if _, e := x.e.Check(ctx, "mon"); e != nil {
		t.Fatal(e)
	}
	rt, _ := x.s.Runtime(ctx, "mon")
	anchor := rt.Rules["rule"].Anchor
	due := rt.Rules["rule"].NextSend
	n := secure.Network{AllowPrivate: true, Timeout: time.Second}
	restarted, e := New(x.s, source.New(n, 2<<20), notify.New(n), 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = restarted.sendDue(ctx, "mon", time.UnixMilli(due+1000)); e != nil {
		t.Fatal(e)
	}
	if e = restarted.sendDue(ctx, "mon", time.UnixMilli(due+1000)); e != nil {
		t.Fatal(e)
	}
	p, _ := x.s.Deliveries(ctx, "mon", "", 1, 25)
	if p.Total != 1 {
		t.Fatal("same time slot duplicated or suppressed by cooldown", p.Total)
	}
	rt, _ = x.s.Runtime(ctx, "mon")
	if rt.Rules["rule"].Anchor != anchor {
		t.Fatal("restart reset interval anchor")
	}
	if e = restarted.sendDue(ctx, "mon", time.UnixMilli(due+10*60000)); e != nil {
		t.Fatal(e)
	}
	p, _ = x.s.Deliveries(ctx, "mon", "", 1, 25)
	if p.Total != 2 {
		t.Fatal("overdue slots replayed instead of coalescing", p.Total)
	}
}
func TestDisabledMonitorNeverDispatches(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	_, _ = x.e.Check(ctx, "mon")
	x.price.Store(131)
	_, _ = x.e.Check(ctx, "mon")
	c := x.e.Config()
	c.Monitors[0].Enabled = false
	if _, e := x.e.SaveConfig(ctx, c); e != nil {
		t.Fatal(e)
	}
	if x.e.DeliverOne(ctx) || x.sent.Load() != 0 {
		t.Fatal("paused queue delivered")
	}
	x.price.Store(132)
	_, _ = x.e.Check(ctx, "mon")
	p, _ := x.s.Deliveries(ctx, "mon", "pending", 1, 25)
	if p.Total != 0 {
		t.Fatal("manual check while paused queued messages")
	}
}
func TestConcurrentChecksAreSerialized(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	var group sync.WaitGroup
	for i := 0; i < 10; i++ {
		group.Add(1)
		go func() { defer group.Done(); _, _ = x.e.Check(ctx, "mon") }()
	}
	group.Wait()
	rt, e := x.s.Runtime(ctx, "mon")
	if e != nil || rt.Latest == nil {
		t.Fatal(e)
	}
	p, _ := x.s.Deliveries(ctx, "mon", "", 1, 25)
	if p.Total != 0 {
		t.Fatal("concurrent baseline duplicated events")
	}
}
func TestConfigurationRevisionAndSourceChangeKeepHistory(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	_, _ = x.e.Check(ctx, "mon")
	old := x.e.Config()
	next := x.e.Config()
	next.Monitors[0].Name = "new name"
	if _, e := x.e.SaveConfig(ctx, next); e != nil {
		t.Fatal(e)
	}
	if _, e := x.e.SaveConfig(ctx, old); e != storage.ErrConflict {
		t.Fatal("stale configuration accepted", e)
	}
	next = x.e.Config()
	next.Monitors[0].Source.Path = "other"
	if _, e := x.e.SaveConfig(ctx, next); e != nil {
		t.Fatal(e)
	}
	rows, _ := x.s.Records(ctx, "mon", 1, 25)
	if rows.Total != 1 {
		t.Fatal("changing source silently removed history")
	}
	rt, _ := x.s.Runtime(ctx, "mon")
	if rt.Latest != nil {
		t.Fatal("changed field reused the previous baseline")
	}
}
