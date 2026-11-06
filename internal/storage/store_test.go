package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/testutil"
)

func TestEncryptionLockAndBackup(t *testing.T) {
	ctx := context.Background()
	s, e := Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if duplicate, e := Open(s.Dir, true); e == nil {
		duplicate.Close()
		t.Fatal("two writers allowed")
	}
	if _, e = s.Config(ctx); e != nil {
		t.Fatal(e)
	}
	c := testutil.Config("https://example.com/data", "https://example.com/hook?secret=fixture")
	c.Channels[0].Secret = "fixture-secret-value"
	c.Revision++
	if e = s.SaveConfig(ctx, c, 1, nil, nil); e != nil {
		t.Fatal(e)
	}
	var b []byte
	if e = s.DB.QueryRow("SELECT body FROM config").Scan(&b); e != nil {
		t.Fatal(e)
	}
	if string(b) == "" || contains(b, "fixture-secret-value") {
		t.Fatal("unencrypted config")
	}
	var vacuum int
	s.DB.QueryRow("PRAGMA auto_vacuum").Scan(&vacuum)
	if vacuum != 2 {
		t.Fatal("incremental vacuum disabled", vacuum)
	}
	dest := t.TempDir()
	if e = s.BackupDatabase(ctx, filepath.Join(dest, "tingfeng.db")); e != nil {
		t.Fatal(e)
	}
	key, e := os.ReadFile(filepath.Join(s.Dir, "master.key"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dest, "master.key"), key, 0600); e != nil {
		t.Fatal(e)
	}
	restored, e := Open(dest, true)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	got, e := restored.Config(ctx)
	if e != nil || got.Channels[0].Secret != c.Channels[0].Secret {
		t.Fatal("backup did not restore encrypted config", e)
	}
}
func contains(b []byte, s string) bool {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}
func TestDeliveryRetryLeaseAndRetention(t *testing.T) {
	ctx := context.Background()
	s, e := Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := testutil.Config("https://example.com/data", "https://example.com/hook")
	rt := domain.Runtime{MonitorID: "mon"}
	now := time.Now().UnixMilli()
	d := domain.Delivery{ID: "one", MonitorID: "mon", RuleID: "rule", Key: "unique", NextAttempt: now, CreatedAt: now, Kind: "monitor"}
	if e = s.SaveRun(ctx, rt, nil, nil, []domain.Delivery{d}, false, c.Monitors[0].Storage); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveRun(ctx, rt, nil, nil, []domain.Delivery{d}, false, c.Monitors[0].Storage); e != nil {
		t.Fatal(e)
	}
	p, _ := s.Deliveries(ctx, "mon", "", 1, 25)
	if p.Total != 1 {
		t.Fatal("dedupe failed")
	}
	for i := 1; i <= 3; i++ {
		row, e := s.ClaimDelivery(ctx, time.Now().Add(time.Hour).UnixMilli())
		if e != nil || row.Attempts != i {
			t.Fatal(i, e, row.Attempts)
		}
		if e = s.FinishDelivery(ctx, row, errors.New("fixture failure")); e != nil {
			t.Fatal(e)
		}
	}
	row, _ := s.Delivery(ctx, "one")
	if row.Status != "failed" {
		t.Fatal("retry never exhausted")
	}
	if e = s.Retry(ctx, "one"); e != nil {
		t.Fatal(e)
	}
	row, e = s.ClaimDelivery(ctx, time.Now().UnixMilli())
	if e != nil || row.Attempts != 1 {
		t.Fatal(e)
	}
	if _, e = s.ClaimDelivery(ctx, time.Now().UnixMilli()); e == nil {
		t.Fatal("lease stolen")
	}
	if _, e = s.ClaimDelivery(ctx, time.Now().Add(2*time.Minute).UnixMilli(), row.ID); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("expired lease was reclaimed while the original sender was active", e)
	}
	row, e = s.ClaimDelivery(ctx, time.Now().Add(2*time.Minute).UnixMilli())
	if e != nil || row.Attempts != 2 {
		t.Fatal("expired lease not reclaimed", e)
	}
	if e = s.FinishDelivery(ctx, row, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("UPDATE deliveries SET created_at=1"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Cleanup(ctx, c); e != nil {
		t.Fatal(e)
	}
	p, _ = s.Deliveries(ctx, "mon", "", 1, 25)
	if p.Total != 0 {
		t.Fatal("old deliveries retained")
	}
}

func TestOldSettingsUseDefaultDeliveryConcurrency(t *testing.T) {
	s, err := Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sealed, err := s.box.Seal([]byte(`{"revision":1,"settings":{"timeZone":"UTC"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("INSERT INTO config(id,revision,body) VALUES(1,1,?)", sealed); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.Config(context.Background())
	if err != nil || cfg.Settings.DeliveryConcurrency != 5 || cfg.Settings.TimeZone != "UTC" {
		t.Fatal("existing database lost its settings or did not receive the default", err)
	}
}
