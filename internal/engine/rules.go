package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
	"tingfeng/internal/domain"
)

func Timed(r domain.Rule) bool { return r.Trigger == "periodic" || r.Trigger == "scheduled" }
func NextSend(r domain.Rule, anchor, after time.Time, loc *time.Location) time.Time {
	if domain.ValidateRule(r) != nil || !Timed(r) {
		return time.Time{}
	}
	after = after.In(loc)
	midnight := time.Date(after.Year(), after.Month(), after.Day(), 0, 0, 0, 0, loc)
	if r.Trigger == "scheduled" {
		n := domain.ClockMinutes(r.Time)
		for d := 0; d < 4; d++ {
			date := midnight.AddDate(0, 0, d)
			t := time.Date(date.Year(), date.Month(), date.Day(), n/60, n%60, 0, 0, loc)
			if t.After(after) {
				return t
			}
		}
		return time.Time{}
	}
	unit := map[string]time.Duration{"minute": time.Minute, "hour": time.Hour, "day": 24 * time.Hour}[r.Unit]
	stride := time.Duration(r.Every) * unit
	if r.Timing != "window" {
		if anchor.IsZero() {
			anchor = after
		}
		steps := after.Sub(anchor)/stride + 1
		if steps < 1 {
			steps = 1
		}
		return anchor.Add(steps * stride)
	}
	start := domain.ClockMinutes(r.WindowStart)
	end := domain.ClockMinutes(r.WindowEnd)
	for d := -1; d < 4; d++ {
		date := midnight.AddDate(0, 0, d)
		open := time.Date(date.Year(), date.Month(), date.Day(), start/60, start%60, 0, 0, loc)
		close := time.Date(date.Year(), date.Month(), date.Day(), end/60, end%60, 0, 0, loc)
		if end < start {
			close = close.AddDate(0, 0, 1)
		}
		t := open
		if !t.After(after) {
			t = t.Add((after.Sub(t)/stride + 1) * stride)
		}
		if t.Before(close) {
			return t
		}
	}
	return time.Time{}
}
func RuleSignature(r domain.Rule) string {
	b, _ := json.Marshal(r)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func ValueChanged(before, after json.RawMessage) bool {
	var a, b any
	da := json.NewDecoder(bytes.NewReader(before))
	da.UseNumber()
	db := json.NewDecoder(bytes.NewReader(after))
	db.UseNumber()
	if da.Decode(&a) != nil || db.Decode(&b) != nil {
		return !bytes.Equal(before, after)
	}
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return !bytes.Equal(aa, bb)
}
func Crossed(r domain.Rule, previous, current json.RawMessage) bool {
	// Decimal comparison avoids a spurious crossing caused by float rounding of a source's numeric value.
	p, ok1 := new(big.Rat).SetString(numeric(previous))
	c, ok2 := new(big.Rat).SetString(numeric(current))
	threshold, ok3 := new(big.Rat).SetString(fmt.Sprintf("%.15g", r.Threshold))
	if !ok1 || !ok2 || !ok3 {
		return false
	}
	if r.Operator == "lt" {
		return p.Cmp(threshold) >= 0 && c.Cmp(threshold) < 0
	}
	return p.Cmp(threshold) <= 0 && c.Cmp(threshold) > 0
}
func numeric(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(v))
}
func ShouldLog(m domain.Monitor, stage, level string) bool {
	return m.Storage.LogLevel != "off" && m.Storage.Stages[stage] && (m.Storage.LogLevel == "summary" || level == "error")
}

type Trial struct {
	Index  int    `json:"index"`
	Minute int    `json:"minute"`
	Value  string `json:"value"`
	Send   bool   `json:"send"`
	Reason string `json:"reason"`
}

func Simulate(r domain.Rule, values []json.RawMessage, interval int) []Trial {
	out := []Trial{}
	last := -100000
	seen := map[string]bool{}
	for i, v := range values {
		t := Trial{Index: i, Minute: i * interval, Value: domain.Display(v), Reason: "未变化"}
		s := string(v)
		if i == 0 {
			t.Reason = "首次建立基准，不发送"
		} else {
			match := ValueChanged(values[i-1], v)
			if r.Trigger == "threshold" {
				match = Crossed(r, values[i-1], v)
				t.Reason = "没有越过阈值"
			}
			if r.Trigger == "new" {
				match = !seen[s]
				t.Reason = "已见过的条目"
			}
			if match {
				if t.Minute-last < r.Cooldown {
					t.Reason = fmt.Sprintf("冷却中，还剩 %d 分钟", r.Cooldown-t.Minute+last)
				} else {
					t.Send = true
					t.Reason = "符合提醒条件"
					last = t.Minute
				}
			}
		}
		seen[s] = true
		out = append(out, t)
	}
	return out
}

// Keep the timer anchor across restarts; edits and re-enabling explicitly start a new schedule.
func initialRuleState(m domain.Monitor, r domain.Rule, zone string, rt domain.Runtime, now time.Time, loc *time.Location) domain.RuleState {
	anchor := now
	old := rt.Rules[r.ID]
	if m.ActivatedAt > 0 && (rt.Latest == nil || m.ActivatedAt > old.Anchor) {
		anchor = time.UnixMilli(m.ActivatedAt)
	}
	state := domain.RuleState{Signature: fmt.Sprintf("%s:%s:%d", RuleSignature(r), zone, m.ActivatedAt), Anchor: anchor.UnixMilli()}
	if Timed(r) {
		state.NextSend = NextSend(r, anchor, now, loc).UnixMilli()
	}
	return state
}

func withinWindow(r domain.Rule, now time.Time, loc *time.Location) bool {
	if r.Trigger != "periodic" || r.Timing != "window" {
		return true
	}
	local := now.In(loc)
	minute := local.Hour()*60 + local.Minute()
	start, end := domain.ClockMinutes(r.WindowStart), domain.ClockMinutes(r.WindowEnd)
	if end > start {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}
