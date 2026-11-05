package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tingfeng/internal/domain"
)

func bounds(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 25
	}
	if size > 100 {
		size = 100
	}
	if page > 100000 {
		page = 100000
	}
	return page, size
}
func (s *Store) Records(ctx context.Context, id string, page, size int) (domain.Page[domain.Record], error) {
	page, size = bounds(page, size)
	p := domain.Page[domain.Record]{Items: []domain.Record{}, Page: page, PageSize: size}
	if e := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM records WHERE monitor_id=?", id).Scan(&p.Total); e != nil {
		return p, e
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT id,at,source_at,value,display,fields FROM records WHERE monitor_id=? ORDER BY at DESC,id DESC LIMIT ? OFFSET ?", id, size, (page-1)*size)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.Record
		var raw, fields string
		if e = rows.Scan(&r.ID, &r.At, &r.SourceAt, &raw, &r.Display, &fields); e != nil {
			return p, e
		}
		if e = json.Unmarshal([]byte(fields), &r.Fields); e != nil {
			return p, e
		}
		r.Value = json.RawMessage(raw)
		p.Items = append(p.Items, r)
	}
	return p, rows.Err()
}
func (s *Store) Logs(ctx context.Context, id, stage string, page, size int) (domain.Page[domain.Log], error) {
	page, size = bounds(page, size)
	p := domain.Page[domain.Log]{Items: []domain.Log{}, Page: page, PageSize: size}
	where := "monitor_id=?"
	args := []any{id}
	if stage != "" {
		where += " AND stage=?"
		args = append(args, stage)
	}
	if e := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM logs WHERE "+where, args...).Scan(&p.Total); e != nil {
		return p, e
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT id,monitor_id,at,stage,level,message FROM logs WHERE "+where+" ORDER BY at DESC,id DESC LIMIT ? OFFSET ?", append(args, size, (page-1)*size)...)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		var l domain.Log
		if e = rows.Scan(&l.ID, &l.MonitorID, &l.At, &l.Stage, &l.Level, &l.Message); e != nil {
			return p, e
		}
		p.Items = append(p.Items, l)
	}
	return p, rows.Err()
}

const deliveryColumns = "id,monitor_id,rule_id,generation,target,recipient,channel,kind,payload,status,attempts,next_attempt,created_at,updated_at,error"

type scanner interface{ Scan(...any) error }

func scanDelivery(row scanner) (domain.Delivery, error) {
	var d domain.Delivery
	var raw string
	e := row.Scan(&d.ID, &d.MonitorID, &d.RuleID, &d.Generation, &d.Target, &d.Recipient, &d.Channel, &d.Kind, &raw, &d.Status, &d.Attempts, &d.NextAttempt, &d.CreatedAt, &d.UpdatedAt, &d.Error)
	if e != nil {
		return d, e
	}
	e = json.Unmarshal([]byte(raw), &d.Payload)
	return d, e
}
func (s *Store) Deliveries(ctx context.Context, id, status string, page, size int) (domain.Page[domain.Delivery], error) {
	page, size = bounds(page, size)
	p := domain.Page[domain.Delivery]{Items: []domain.Delivery{}, Page: page, PageSize: size}
	where := "1=1"
	args := []any{}
	if id != "" {
		where += " AND monitor_id=?"
		args = append(args, id)
	}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	if e := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM deliveries WHERE "+where, args...).Scan(&p.Total); e != nil {
		return p, e
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT "+deliveryColumns+" FROM deliveries WHERE "+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?", append(args, size, (page-1)*size)...)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		d, e := scanDelivery(rows)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, d)
	}
	return p, rows.Err()
}
func (s *Store) ClaimDelivery(ctx context.Context, at int64) (domain.Delivery, error) {
	// A lease recovers an interrupted HTTP request after restart. Provider acknowledgements are not transactional with SQLite.
	return scanDelivery(s.DB.QueryRowContext(ctx, "UPDATE deliveries SET status='sending',attempts=attempts+1,updated_at=?,next_attempt=? WHERE id=(SELECT id FROM deliveries WHERE status IN ('pending','sending') AND next_attempt<=? ORDER BY next_attempt LIMIT 1) RETURNING "+deliveryColumns, at, at+60000, at))
}
func (s *Store) FinishDelivery(ctx context.Context, d domain.Delivery, err error) error {
	status := "success"
	msg := ""
	next := int64(0)
	at := nowMS()
	if err != nil {
		status = "failed"
		msg = err.Error()
		if d.Attempts < 3 {
			status = "pending"
			delays := []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute}
			next = at + delays[min(d.Attempts-1, 2)].Milliseconds()
		}
	}
	_, e := s.DB.ExecContext(ctx, "UPDATE deliveries SET status=?,error=?,next_attempt=?,updated_at=? WHERE id=? AND status='sending'", status, msg, next, at, d.ID)
	return e
}
func (s *Store) CancelDelivery(ctx context.Context, id string, why string) error {
	_, e := s.DB.ExecContext(ctx, "UPDATE deliveries SET status='cancelled',error=?,updated_at=? WHERE id=?", why, nowMS(), id)
	return e
}
func (s *Store) Retry(ctx context.Context, id string) error {
	result, e := s.DB.ExecContext(ctx, "UPDATE deliveries SET status='pending',attempts=0,next_attempt=?,error='' WHERE id=? AND status='failed'", nowMS(), id)
	if e != nil {
		return e
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("只能重试失败的投递记录")
	}
	return nil
}
func (s *Store) Delivery(ctx context.Context, id string) (domain.Delivery, error) {
	return scanDelivery(s.DB.QueryRowContext(ctx, "SELECT "+deliveryColumns+" FROM deliveries WHERE id=?", id))
}
func (s *Store) AddLog(ctx context.Context, l domain.Log) error {
	_, e := s.DB.ExecContext(ctx, "INSERT INTO logs(monitor_id,at,stage,level,message) VALUES(?,?,?,?,?)", l.MonitorID, l.At, l.Stage, l.Level, l.Message)
	return e
}
func (s *Store) ClearRecords(ctx context.Context, id string) error {
	_, e := s.DB.ExecContext(ctx, "DELETE FROM records WHERE monitor_id=?", id)
	return e
}

func (s *Store) RecordsAfter(ctx context.Context, id string, after int64) ([]domain.Record, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT id,at,source_at,value,display,fields FROM records WHERE monitor_id=? AND id>? ORDER BY id LIMIT 64", id, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Record{}
	for rows.Next() {
		var row domain.Record
		var raw, fields string
		if e = rows.Scan(&row.ID, &row.At, &row.SourceAt, &raw, &row.Display, &fields); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(fields), &row.Fields); e != nil {
			return nil, e
		}
		row.Value = json.RawMessage(raw)
		out = append(out, row)
	}
	return out, rows.Err()
}
