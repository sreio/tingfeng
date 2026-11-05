package storage

import (
	"context"
	"fmt"
	"strconv"
	"tingfeng/internal/domain"
)

type CleanupResult struct {
	Removed  int64 `json:"removed"`
	At       int64 `json:"at"`
	Bytes    int64 `json:"bytes"`
	Pressure bool  `json:"pressure"`
}

func (s *Store) Cleanup(ctx context.Context, c domain.Config) (CleanupResult, error) {
	result := CleanupResult{At: nowMS()}
	remove := func(query string, args ...any) error {
		r, e := s.DB.ExecContext(ctx, query, args...)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		result.Removed += n
		return nil
	}
	for _, m := range c.Monitors {
		st := m.Storage
		if e := remove("DELETE FROM records WHERE monitor_id=? AND (at<? OR id NOT IN (SELECT id FROM records WHERE monitor_id=? ORDER BY at DESC,id DESC LIMIT ?))", m.ID, result.At-int64(st.Days)*86400000, m.ID, st.MaxRows); e != nil {
			return result, e
		}
		if e := remove("DELETE FROM logs WHERE monitor_id=? AND (at<? OR id NOT IN (SELECT id FROM logs WHERE monitor_id=? ORDER BY at DESC,id DESC LIMIT 5000))", m.ID, result.At-int64(st.LogDays)*86400000, m.ID); e != nil {
			return result, e
		}
	}
	if e := remove("DELETE FROM deliveries WHERE status IN ('success','failed','cancelled') AND (created_at<? OR id NOT IN (SELECT id FROM deliveries ORDER BY created_at DESC LIMIT 10000))", result.At-int64(c.Settings.DeliveryDays)*86400000); e != nil {
		return result, e
	}
	if e := remove("DELETE FROM sessions WHERE expires<?", result.At); e != nil {
		return result, e
	}
	// Reclaim in bounded steps; no full VACUUM that duplicates the database on a small disk.
	if _, e := s.DB.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)"); e != nil {
		return result, e
	}
	if _, e := s.DB.ExecContext(ctx, "PRAGMA incremental_vacuum(512)"); e != nil {
		return result, e
	}
	result.Pressure = !s.HasRoom(c.Settings.DiskLimitMB)
	if result.Pressure {
		for i := 0; i < 10 && s.DiskBytes() > int64(c.Settings.DiskLimitMB)*1024*1024*8/10; i++ {
			if e := remove("DELETE FROM records WHERE id IN (SELECT id FROM records ORDER BY at LIMIT 500)"); e != nil {
				return result, e
			}
			if _, e := s.DB.ExecContext(ctx, "PRAGMA incremental_vacuum(512)"); e != nil {
				return result, e
			}
		}
	}
	if e := s.SetMeta(ctx, "last_cleanup", []byte(strconv.FormatInt(result.At, 10))); e != nil {
		return result, e
	}
	result.Bytes = s.DiskBytes()
	return result, nil
}
func (s *Store) BackupDatabase(ctx context.Context, path string) error {
	// The destination must not already exist; SQLite creates a consistent snapshot including committed WAL entries.
	if path == "" {
		return fmt.Errorf("备份目标路径为空")
	}
	_, e := s.DB.ExecContext(ctx, "VACUUM INTO ?", path)
	return e
}
func (s *Store) LastCleanup(ctx context.Context) int64 {
	v, e := s.Meta(ctx, "last_cleanup")
	if e != nil {
		return 0
	}
	n, _ := strconv.ParseInt(string(v), 10, 64)
	return n
}
