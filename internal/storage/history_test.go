package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/testutil"
)

func TestRSSHistoryAndBatchedExport(t *testing.T) {
	s, e := Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	policy := testutil.Config("https://example.com", "https://example.com").Monitors[0].Storage
	policy.MaxRows = 100
	obs := domain.Observation{Value: json.RawMessage(`"latest article"`), Display: "latest article", CheckedAt: time.Now().UnixMilli(), Items: []domain.Item{{ID: "1", Title: "first"}, {ID: "2", Title: "second"}}}
	for i := 0; i < 70; i++ {
		obs.Items[0].Title = fmt.Sprint(i)
		if e = s.SaveRun(ctx, domain.Runtime{MonitorID: "rss"}, &obs, nil, nil, true, policy); e != nil {
			t.Fatal(e)
		}
	}
	page, e := s.Records(ctx, "rss", 1, 25)
	if e != nil || page.Total != 70 {
		t.Fatal(e, page.Total)
	}
	var items []domain.Item
	if json.Unmarshal(page.Items[0].Value, &items) != nil || len(items) != 2 || items[1].Title != "second" {
		t.Fatal("RSS history lost non-leading items")
	}
	batch, e := s.RecordsAfter(ctx, "rss", 0)
	if e != nil || len(batch) != 64 {
		t.Fatal(e, len(batch))
	}
	if e = s.SetMeta(ctx, "export-test", []byte("writes are not blocked")); e != nil {
		t.Fatal(e)
	}
	rest, e := s.RecordsAfter(ctx, "rss", batch[len(batch)-1].ID)
	if e != nil || len(rest) != 6 {
		t.Fatal(e, len(rest))
	}
}
