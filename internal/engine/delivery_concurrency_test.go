package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/notify"
	"tingfeng/internal/secure"
)

func TestDispatchResizesWhileRequestsAreInFlight(t *testing.T) {
	const targets = 12
	type attempt struct {
		id    string
		reply chan int
	}
	arrivals := make(chan attempt, targets+1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := attempt{r.Header.Get("X-Tingfeng-Delivery-ID"), make(chan int, 1)}
		arrivals <- a
		select {
		case status := <-a.reply:
			w.WriteHeader(status)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(hook.Close)
	x := setup(t)
	x.e.Sender = notify.New(secure.Network{AllowPrivate: true, Timeout: 15 * time.Second})
	c := x.e.Config()
	c.Channels[0].URL = hook.URL
	c.Recipients[0].Methods = nil
	c.Monitors[0].Rules[0].Selections = nil
	for i := range targets {
		id := fmt.Sprintf("m%d", i)
		c.Recipients[0].Methods = append(c.Recipients[0].Methods, domain.Method{ID: id, ChannelID: "ch"})
		c.Monitors[0].Rules[0].Selections = append(c.Monitors[0].Rules[0].Selections, "me:"+id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if _, err := x.e.SaveConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := x.e.Check(ctx, "mon"); err != nil {
		t.Fatal(err)
	}
	x.e.Start(ctx)
	t.Cleanup(func() { cancel(); x.e.Wait() })
	x.price.Store(131)
	if _, err := x.e.Check(ctx, "mon"); err != nil {
		t.Fatal(err)
	}
	next := func() attempt {
		t.Helper()
		select {
		case a := <-arrivals:
			return a
		case <-ctx.Done():
			t.Fatal("waiting for concurrent delivery", ctx.Err())
			return attempt{}
		}
	}
	expectFull := func() {
		t.Helper()
		select {
		case a := <-arrivals:
			t.Fatal("started another request despite the current limit", a.id)
		case <-time.After(80 * time.Millisecond):
		}
	}
	setLimit := func(n int) {
		t.Helper()
		cfg := x.e.Config()
		cfg.Settings.DeliveryConcurrency = n
		if _, err := x.e.SaveConfig(ctx, cfg); err != nil {
			t.Fatal(err)
		}
	}
	var held []attempt
	for range 5 {
		held = append(held, next())
	}
	expectFull()
	setLimit(7)
	held = append(held, next(), next())
	expectFull()
	setLimit(2)
	held[0].reply <- http.StatusServiceUnavailable
	for _, a := range held[1:5] {
		a.reply <- http.StatusNoContent
	}
	expectFull()
	held[5].reply <- http.StatusNoContent
	// One deliberately slow channel stays open while all remaining notifications finish.
	for range targets - len(held) {
		next().reply <- http.StatusNoContent
	}
	held[6].reply <- http.StatusNoContent
	waitForSuccess := func(count int) {
		t.Helper()
		for {
			rows, err := x.s.Deliveries(ctx, "mon", "success", 1, targets)
			if err != nil {
				t.Fatal(err)
			}
			if rows.Total == count {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatalf("got %d successful deliveries, want %d", rows.Total, count)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	waitForSuccess(targets - 1)
	failed, err := x.s.Delivery(ctx, held[0].id)
	if err != nil || failed.Status != "pending" || failed.Attempts != 1 {
		t.Fatal("failed channel was not queued for an independent retry", err)
	}
	if _, err := x.s.DB.ExecContext(ctx, "UPDATE deliveries SET next_attempt=0 WHERE id=?", failed.ID); err != nil {
		t.Fatal(err)
	}
	retry := next()
	if retry.id != failed.ID {
		t.Fatal("unexpected retry", retry.id)
	}
	retry.reply <- http.StatusNoContent
	waitForSuccess(targets)
	rows, err := x.s.Deliveries(ctx, "mon", "", 1, targets)
	if err != nil || rows.Total != targets {
		t.Fatal("incorrect number of persisted notifications", err)
	}
	for _, d := range rows.Items {
		if d.ID == failed.ID && d.Attempts != 2 || d.ID != failed.ID && d.Attempts != 1 {
			t.Fatal("incorrect number of attempts", d.ID, d.Attempts)
		}
	}
}
