package storage

import (
	"context"
	"encoding/json"
	"tingfeng/internal/domain"
)

func (s *Store) RecordTest(ctx context.Context, id, channel, address string, p domain.Payload, sendErr error) error {
	status := "success"
	message := ""
	if sendErr != nil {
		status = "failed"
		message = sendErr.Error()
	}
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO deliveries(id,monitor_id,rule_id,generation,target,recipient,channel,kind,dedupe_key,payload,status,attempts,next_attempt,created_at,updated_at,error) VALUES(?,'','',0,'',?,?,'test',?,?,?,1,0,?,?,?)`, id, "手动测试", channel, id, string(raw), status, nowMS(), nowMS(), message)
	return e
}
