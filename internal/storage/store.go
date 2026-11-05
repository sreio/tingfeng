package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
)

var ErrConflict = errors.New("配置已被其他页面修改，请刷新后再保存")

type Store struct {
	DB   *sql.DB
	Dir  string
	box  *secure.Box
	lock *os.File
}

func Open(dir string, exclusive bool) (*Store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	var lock *os.File
	if exclusive {
		var e error
		lock, e = os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
		if e != nil {
			return nil, e
		}
		if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
			_ = lock.Close()
			return nil, fmt.Errorf("已有听风实例使用这个数据目录")
		}
	}
	cleanup := func() {
		if lock != nil {
			_ = lock.Close()
		}
	}
	box, e := secure.OpenBox(dir)
	if e != nil {
		cleanup()
		return nil, e
	}
	path := filepath.Join(dir, "tingfeng.db")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		cleanup()
		return nil, e
	}
	_ = f.Close()
	q := url.Values{}
	for _, p := range []string{"busy_timeout(5000)", "foreign_keys(ON)", "journal_mode(WAL)", "synchronous(NORMAL)", "cache_size(-2048)", "mmap_size(0)", "wal_autocheckpoint(256)"} {
		q.Add("_pragma", p)
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	db, e := sql.Open("sqlite", u.String())
	if e != nil {
		cleanup()
		return nil, e
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{DB: db, Dir: dir, box: box, lock: lock}
	if e = s.migrate(); e != nil {
		_ = s.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error {
	e := s.DB.Close()
	if s.lock != nil {
		_ = s.lock.Close()
	}
	return e
}
func (s *Store) migrate() error {
	var version int
	if e := s.DB.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	if version > 1 {
		return fmt.Errorf("数据库版本高于程序支持版本，请勿降级")
	}
	if version == 1 {
		return nil
	}
	_, e := s.DB.Exec(`
 PRAGMA auto_vacuum=INCREMENTAL;
 VACUUM;
 CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY,value BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS config(id INTEGER PRIMARY KEY CHECK(id=1),revision INTEGER NOT NULL,body BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS runtime(monitor_id TEXT PRIMARY KEY,generation INTEGER NOT NULL,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS records(id INTEGER PRIMARY KEY,monitor_id TEXT NOT NULL,at INTEGER NOT NULL,source_at INTEGER NOT NULL,value TEXT NOT NULL,display TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS records_monitor_time ON records(monitor_id,at DESC,id DESC);
 CREATE TABLE IF NOT EXISTS logs(id INTEGER PRIMARY KEY,monitor_id TEXT NOT NULL,at INTEGER NOT NULL,stage TEXT NOT NULL,level TEXT NOT NULL,message TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS logs_monitor_time ON logs(monitor_id,at DESC,id DESC);
 CREATE TABLE IF NOT EXISTS deliveries(id TEXT PRIMARY KEY,monitor_id TEXT NOT NULL,rule_id TEXT NOT NULL,generation INTEGER NOT NULL,target TEXT NOT NULL,recipient TEXT NOT NULL,channel TEXT NOT NULL,kind TEXT NOT NULL,dedupe_key TEXT NOT NULL UNIQUE,payload TEXT NOT NULL,status TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_attempt INTEGER NOT NULL,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,error TEXT NOT NULL DEFAULT '');
 CREATE INDEX IF NOT EXISTS delivery_due ON deliveries(status,next_attempt);
 CREATE INDEX IF NOT EXISTS delivery_monitor ON deliveries(monitor_id,created_at DESC);
 CREATE TABLE IF NOT EXISTS sessions(hash TEXT PRIMARY KEY,csrf TEXT NOT NULL,expires INTEGER NOT NULL);
 PRAGMA user_version=1;
 `)
	return e
}
func (s *Store) Config(ctx context.Context) (domain.Config, error) {
	var body []byte
	e := s.DB.QueryRowContext(ctx, "SELECT body FROM config WHERE id=1").Scan(&body)
	if errors.Is(e, sql.ErrNoRows) {
		c := domain.EmptyConfig()
		raw, _ := json.Marshal(c)
		sealed, e := s.box.Seal(raw)
		if e != nil {
			return c, e
		}
		_, e = s.DB.ExecContext(ctx, "INSERT OR IGNORE INTO config VALUES(1,?,?)", c.Revision, sealed)
		if e != nil {
			return c, e
		}
		return c, nil
	}
	if e != nil {
		return domain.Config{}, e
	}
	raw, e := s.box.Open(body)
	if e != nil {
		return domain.Config{}, e
	}
	var c domain.Config
	e = json.Unmarshal(raw, &c)
	return c, e
}
func (s *Store) SaveConfig(ctx context.Context, c domain.Config, expected int64, removed, changedSources []string) error {
	raw, e := json.Marshal(c)
	if e != nil {
		return e
	}
	sealed, e := s.box.Seal(raw)
	if e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(ctx, "UPDATE config SET revision=?,body=? WHERE id=1 AND revision=?", c.Revision, sealed, expected)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	for _, id := range removed {
		for _, table := range []string{"runtime", "records", "logs"} {
			if _, e = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE monitor_id=?", id); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "UPDATE deliveries SET status='cancelled',error='监控已删除' WHERE monitor_id=? AND status IN ('pending','sending')", id); e != nil {
			return e
		}
	}
	for _, id := range changedSources {
		if _, e = tx.ExecContext(ctx, "DELETE FROM runtime WHERE monitor_id=?", id); e != nil {
			return e
		}
	}
	for _, m := range c.Monitors {
		if _, e = tx.ExecContext(ctx, "UPDATE deliveries SET status='cancelled',error='配置已修改或暂停' WHERE monitor_id=? AND status IN ('pending','sending') AND (generation<>? OR ?=0)", m.ID, m.Generation, m.Enabled); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) Runtime(ctx context.Context, id string) (domain.Runtime, error) {
	rt := domain.Runtime{MonitorID: id, Rules: map[string]domain.RuleState{}}
	var raw []byte
	e := s.DB.QueryRowContext(ctx, "SELECT body FROM runtime WHERE monitor_id=?", id).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return rt, nil
	}
	if e != nil {
		return rt, e
	}
	e = json.Unmarshal(raw, &rt)
	if rt.Rules == nil {
		rt.Rules = map[string]domain.RuleState{}
	}
	return rt, e
}
func (s *Store) Runtimes(ctx context.Context) ([]domain.Runtime, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT body FROM runtime")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Runtime{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var rt domain.Runtime
		if e = json.Unmarshal(b, &rt); e != nil {
			return nil, e
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}
func (s *Store) SaveRun(ctx context.Context, rt domain.Runtime, obs *domain.Observation, logs []domain.Log, deliveries []domain.Delivery, keep bool, policy domain.Storage) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	raw, e := json.Marshal(rt)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO runtime VALUES(?,?,?) ON CONFLICT(monitor_id) DO UPDATE SET generation=excluded.generation,body=excluded.body", rt.MonitorID, rt.Generation, string(raw)); e != nil {
		return e
	}
	if keep && obs != nil {
		value := obs.Value
		if len(obs.Items) > 0 {
			value, e = json.Marshal(obs.Items)
			if e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO records(monitor_id,at,source_at,value,display) VALUES(?,?,?,?,?)", rt.MonitorID, obs.CheckedAt, obs.SourceAt, string(value), obs.Display); e != nil {
			return e
		}
	}
	for _, l := range logs {
		if _, e = tx.ExecContext(ctx, "INSERT INTO logs(monitor_id,at,stage,level,message) VALUES(?,?,?,?,?)", rt.MonitorID, l.At, l.Stage, l.Level, l.Message); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM records WHERE monitor_id=? AND (at<? OR id IN (SELECT id FROM records WHERE monitor_id=? ORDER BY at DESC,id DESC LIMIT -1 OFFSET ?))", rt.MonitorID, nowMS()-int64(policy.Days)*86400000, rt.MonitorID, policy.MaxRows); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM logs WHERE monitor_id=? AND (at<? OR id IN (SELECT id FROM logs WHERE monitor_id=? ORDER BY at DESC,id DESC LIMIT -1 OFFSET 5000))", rt.MonitorID, nowMS()-int64(policy.LogDays)*86400000, rt.MonitorID); e != nil {
		return e
	}

	if len(deliveries) > 0 {
		var count int
		if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM deliveries WHERE status IN ('pending','sending')").Scan(&count); e != nil {
			return e
		}
		if count+len(deliveries) > 1000 {
			return fmt.Errorf("待发送队列达到 1000 条，请先处理失败渠道")
		}
	}
	for _, d := range deliveries {
		if e = insertDelivery(ctx, tx, d); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func insertDelivery(ctx context.Context, tx *sql.Tx, d domain.Delivery) error {
	raw, e := json.Marshal(d.Payload)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO deliveries(id,monitor_id,rule_id,generation,target,recipient,channel,kind,dedupe_key,payload,status,next_attempt,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'pending',?,?,?)`, d.ID, d.MonitorID, d.RuleID, d.Generation, d.Target, d.Recipient, d.Channel, d.Kind, d.Key, string(raw), d.NextAttempt, d.CreatedAt, d.CreatedAt)
	return e
}
func (s *Store) Meta(ctx context.Context, key string) ([]byte, error) {
	var v []byte
	e := s.DB.QueryRowContext(ctx, "SELECT value FROM meta WHERE key=?", key).Scan(&v)
	if errors.Is(e, sql.ErrNoRows) {
		e = nil
	}
	return v, e
}
func (s *Store) SetMeta(ctx context.Context, key string, value []byte) error {
	_, e := s.DB.ExecContext(ctx, "INSERT INTO meta VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return e
}
func (s *Store) DiskBytes() int64 {
	var n int64
	for _, name := range []string{"tingfeng.db", "tingfeng.db-wal", "tingfeng.db-shm"} {
		if st, e := os.Stat(filepath.Join(s.Dir, name)); e == nil {
			n += st.Size()
		}
	}
	return n
}
func (s *Store) HasRoom(limit int) bool         { return s.DiskBytes() < int64(limit)*1024*1024*9/10 }
func (s *Store) Ping(ctx context.Context) error { return s.DB.PingContext(ctx) }
func nowMS() int64                              { return time.Now().UnixMilli() }
