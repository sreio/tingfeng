package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/notify"
	"tingfeng/internal/source"
	"tingfeng/internal/storage"
)

type Engine struct {
	Store       *storage.Store
	Fetcher     *source.Fetcher
	Sender      *notify.Sender
	mu          sync.RWMutex
	cfg         domain.Config
	slots       chan struct{}
	locks       sync.Map
	wg          sync.WaitGroup
	wake        chan struct{}
	lastCleanup time.Time
}

func New(s *storage.Store, f *source.Fetcher, n *notify.Sender, workers int) (*Engine, error) {
	c, e := s.Config(context.Background())
	if e != nil {
		return nil, e
	}
	return &Engine{Store: s, Fetcher: f, Sender: n, cfg: c, slots: make(chan struct{}, workers), wake: make(chan struct{}, 1)}, nil
}
func (e *Engine) Config() domain.Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return domain.Clone(e.cfg)
}
func (e *Engine) SaveConfig(ctx context.Context, c domain.Config) (domain.Config, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c.Revision != e.cfg.Revision {
		return domain.Config{}, storage.ErrConflict
	}
	domain.MergeSecrets(&c, e.cfg)
	if err := domain.Validate(c); err != nil {
		return domain.Config{}, err
	}
	removed := []string{}
	changed := []string{}
	for _, old := range e.cfg.Monitors {
		n, ok := c.FindMonitor(old.ID)
		if !ok {
			removed = append(removed, old.ID)
		} else if !reflect.DeepEqual(n.Source, old.Source) {
			changed = append(changed, old.ID)
		}
	}
	domain.Prepare(&c, e.cfg, time.Now())
	if err := e.Store.SaveConfig(ctx, c, e.cfg.Revision, removed, changed); err != nil {
		return domain.Config{}, err
	}
	e.cfg = c
	select {
	case e.wake <- struct{}{}:
	default:
	}
	return domain.Redacted(c), nil
}
func (e *Engine) lock(id string) *sync.Mutex {
	v, _ := e.locks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}
func (e *Engine) Start(ctx context.Context) {
	e.wg.Add(2)
	go func() {
		defer e.wg.Done()
		timer := time.NewTicker(time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				e.tick(ctx, time.Now())
			case <-e.wake:
				e.tick(ctx, time.Now())
			}
		}
	}()
	go func() {
		defer e.wg.Done()
		timer := time.NewTicker(time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				for i := 0; i < 4; i++ {
					if !e.DeliverOne(ctx) {
						break
					}
				}
			}
		}
	}()
}
func (e *Engine) Wait() { e.wg.Wait() }
func (e *Engine) tick(ctx context.Context, now time.Time) {
	c := e.Config()
	states, err := e.Store.Runtimes(ctx)
	if err != nil {
		slog.Error("读取调度状态失败")
		return
	}
	byID := map[string]domain.Runtime{}
	for _, rt := range states {
		byID[rt.MonitorID] = rt
	}
	for _, m := range c.Monitors {
		if !m.Enabled || m.Draft || m.NeedsReview {
			continue
		}
		if rt := byID[m.ID]; rt.NextCheck <= now.UnixMilli() || rt.Generation != m.Generation {
			lock := e.lock(m.ID)
			if !lock.TryLock() {
				continue
			}
			select {
			case e.slots <- struct{}{}:
				e.wg.Add(1)
				go func(id string) {
					defer e.wg.Done()
					defer func() { <-e.slots; lock.Unlock() }()
					if _, err := e.check(ctx, id); err != nil && !errors.Is(err, context.Canceled) {
						slog.Warn("采集未完成", "monitor", id, "reason", err.Error())
					}
				}(m.ID)
			default:
				lock.Unlock()
			}
		} else {
			_ = e.sendDue(ctx, m.ID, now)
		}
	}
	if now.Sub(e.lastCleanup) >= time.Hour {
		e.lastCleanup = now
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			if _, err := e.Store.Cleanup(ctx, e.Config()); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("自动清理失败")
			}
		}()
	}
}
func (e *Engine) Check(ctx context.Context, id string) (domain.Runtime, error) {
	select {
	case e.slots <- struct{}{}:
		defer func() { <-e.slots }()
	case <-ctx.Done():
		return domain.Runtime{}, ctx.Err()
	}
	lock := e.lock(id)
	if !lock.TryLock() {
		return domain.Runtime{}, fmt.Errorf("这条监控正在检查，请稍后再试")
	}
	defer lock.Unlock()
	return e.check(ctx, id)
}
func (e *Engine) check(ctx context.Context, id string) (domain.Runtime, error) {
	c := e.Config()
	m, ok := c.FindMonitor(id)
	if !ok {
		return domain.Runtime{}, fmt.Errorf("监控不存在")
	}
	if m.Draft {
		return domain.Runtime{}, fmt.Errorf("请先完成草稿配置")
	}
	rt, err := e.Store.Runtime(ctx, id)
	if err != nil {
		return rt, err
	}
	original := domain.Clone(rt)
	now := time.Now()
	at := now.UnixMilli()
	rt.Generation = m.Generation
	rt.LastCheck = at
	preview, fetchErr := e.Fetcher.Fetch(ctx, m.Source)
	if ctx.Err() != nil {
		return rt, ctx.Err()
	}
	logs := []domain.Log{}
	ds := []domain.Delivery{}
	log := func(stage, level, msg string) {
		if e.Store.HasRoom(c.Settings.DiskLimitMB) && ShouldLog(m, stage, level) {
			logs = append(logs, domain.Log{MonitorID: id, At: at, Stage: stage, Level: level, Message: msg})
		}
	}
	var obs *domain.Observation
	if fetchErr != nil {
		rt.Failures++
		rt.Fetch = fetchErr.Error()
		rt.Evaluate = "采集失败，未判断"
		rt.Send = "保留上次成功值，未发送"
		delay := time.Duration(m.Interval) * time.Minute * time.Duration(1<<min(rt.Failures-1, 4))
		if delay > 6*time.Hour {
			delay = 6 * time.Hour
		}
		rt.NextCheck = now.Add(delay).UnixMilli()
		rt.Incident = &domain.Incident{At: at, Message: "采集失败：" + rt.Fetch}
		log("fetch", "error", rt.Fetch)
	} else {
		rt.Failures = 0
		rt.NextCheck = now.Add(time.Duration(m.Interval) * time.Minute).UnixMilli()
		rt.LastSuccess = at
		rt.Fetch = fmt.Sprintf("成功 · %d ms", preview.Millis)
		if rt.Incident != nil && !rt.Incident.Resolved && strings.HasPrefix(rt.Incident.Message, "采集") {
			rt.Incident.Resolved = true
		}
		current := preview.Observation
		obs = &current
		previous := rt.Latest
		first := previous == nil
		seen := map[string]bool{}
		for _, v := range rt.Seen {
			seen[v] = true
		}
		fresh := []domain.Item{}
		for _, v := range current.Items {
			if !seen[v.ID] {
				fresh = append(fresh, v)
				rt.Seen = append(rt.Seen, v.ID)
				seen[v.ID] = true
			}
		}
		if len(rt.Seen) > 2000 {
			rt.Seen = rt.Seen[len(rt.Seen)-2000:]
		}
		reasons := []string{}
		loc, _ := time.LoadLocation(c.Settings.TimeZone)
		for _, r := range m.Rules {
			state := rt.Rules[r.ID]
			initial := initialRuleState(m, r, c.Settings.TimeZone, rt, now, loc)
			newRule := state.Signature != initial.Signature
			if newRule {
				state = initial
			}

			reason := "数据未变化"
			should := false
			items := []domain.Item{}
			if Timed(r) {
				reason = "等待定时发送"
			} else if first || newRule {
				reason = "首次建立基准，不发送"
			} else {
				switch r.Trigger {
				case "threshold":
					should = Crossed(r, previous.Value, current.Value)
					reason = "没有越过阈值"
				case "new":
					should = len(fresh) > 0
					items = fresh
					reason = "没有新条目"
				default:
					should = ValueChanged(previous.Value, current.Value)
				}
				if should && at-state.LastQueued < int64(r.Cooldown)*60000 {
					should = false
					reason = "冷却中，跳过提醒"
				}
			}
			if should && m.Enabled {
				targets := c.Targets(r.Selections)
				if len(targets) == 0 {
					reason = "没有可用的通知渠道"
					rt.Incident = &domain.Incident{At: at, Message: reason}
				} else {
					payload, err := notify.Payload(m, r, current, previous.Display, items, loc)
					if err != nil {
						reason = err.Error()
						log("evaluate", "error", reason)
					} else {
						for _, t := range targets {
							ds = append(ds, newDelivery(m, r, t, payload, at, fmt.Sprint(at)))
						}
						state.LastQueued = at
						reason = fmt.Sprintf("符合条件，排队 %d 条", len(targets))
					}
				}
			} else if should && !m.Enabled {
				reason = "符合条件，但监控已暂停"
			}
			rt.Rules[r.ID] = state
			reasons = append(reasons, domain.RuleLabel(r)+"："+reason)
		}
		rt.Latest = &current
		rt.Evaluate = strings.Join(reasons, "；")
		rt.Send = "无需发送"
		if len(ds) > 0 {
			rt.Send = fmt.Sprintf("%d 条待投递", len(ds))
		}
		log("fetch", "info", rt.Fetch)
		log("evaluate", "info", rt.Evaluate)
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	latest, exists := e.cfg.FindMonitor(id)
	if !exists || latest.Generation != m.Generation {
		return rt, fmt.Errorf("配置已改变，这次结果已丢弃")
	}
	keep := m.Storage.History && e.Store.HasRoom(c.Settings.DiskLimitMB)
	if err = e.Store.SaveRun(ctx, rt, obs, logs, ds, keep, m.Storage); err != nil {
		rt = original
		rt.NextCheck = time.Now().Add(time.Minute).UnixMilli()
		rt.Incident = &domain.Incident{At: at, Message: "运行结果保存失败，请检查磁盘或队列"}
		_ = e.Store.SaveRun(ctx, rt, nil, nil, nil, false, m.Storage)
		return rt, fmt.Errorf("运行结果保存失败，请检查磁盘或队列")
	}
	return rt, nil
}
func newDelivery(m domain.Monitor, r domain.Rule, t domain.Target, p domain.Payload, at int64, slot string) domain.Delivery {
	return domain.Delivery{ID: domain.ID("delivery"), MonitorID: m.ID, RuleID: r.ID, Generation: m.Generation, Target: t.Key, Recipient: t.Person.Name, Channel: t.Channel.Name, Kind: "monitor", Key: fmt.Sprintf("%s:%d:%s:%s:%s", m.ID, m.Generation, r.ID, t.Key, slot), Payload: p, Status: "pending", NextAttempt: at, CreatedAt: at}
}
func (e *Engine) sendDue(ctx context.Context, id string, now time.Time) error {
	lock := e.lock(id)
	if !lock.TryLock() {
		return nil
	}
	defer lock.Unlock()
	c := e.Config()
	m, ok := c.FindMonitor(id)
	if !ok || !m.Enabled || m.Draft || m.NeedsReview {
		return nil
	}
	rt, err := e.Store.Runtime(ctx, id)
	if err != nil {
		return err
	}
	rt.Generation = m.Generation
	loc, _ := time.LoadLocation(c.Settings.TimeZone)
	ds := []domain.Delivery{}
	dirty := false
	for _, r := range m.Rules {
		if !Timed(r) {
			continue
		}
		state := rt.Rules[r.ID]
		initial := initialRuleState(m, r, c.Settings.TimeZone, rt, now, loc)
		if state.Signature != initial.Signature || state.NextSend == 0 {
			state = initial
			rt.Rules[r.ID] = state
			dirty = true
			continue
		}

		if state.NextSend > now.UnixMilli() {
			continue
		}
		slot := state.NextSend
		state.NextSend = NextSend(r, time.UnixMilli(state.Anchor), now, loc).UnixMilli()
		dirty = true
		if !withinWindow(r, now, loc) {
			rt.Send = "不在发送时段内，跳过已过期的提醒"
			rt.Rules[r.ID] = state
			continue
		}
		if rt.Latest != nil {
			p, err := notify.Payload(m, r, *rt.Latest, rt.Latest.Display, nil, loc)
			if err != nil {
				rt.Send = err.Error()
			} else {
				ts := c.Targets(r.Selections)
				for _, t := range ts {
					ds = append(ds, newDelivery(m, r, t, p, now.UnixMilli(), fmt.Sprint(slot)))
				}
				if len(ts) == 0 {
					rt.Send = "定时发送缺少可用渠道"
				} else {
					rt.Send = fmt.Sprintf("定时触发，%d 条待投递", len(ts))
					state.LastQueued = now.UnixMilli()
				}
			}
		} else {
			rt.Send = "尚无成功采集结果，跳过本次定时发送"
		}
		rt.Rules[r.ID] = state
	}
	if !dirty {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	current, ok := e.cfg.FindMonitor(id)
	if !ok || current.Generation != m.Generation {
		return nil
	}
	return e.Store.SaveRun(ctx, rt, nil, nil, ds, false, m.Storage)
}
func (e *Engine) DeliverOne(ctx context.Context) bool {
	d, err := e.Store.ClaimDelivery(ctx, time.Now().UnixMilli())
	if errors.Is(err, sql.ErrNoRows) || ctx.Err() != nil {
		return false
	}
	if err != nil {
		slog.Error("投递队列读取失败")
		return false
	}
	c := e.Config()
	m, ok := c.FindMonitor(d.MonitorID)
	if !ok || !m.Enabled || m.Generation != d.Generation {
		_ = e.Store.CancelDelivery(ctx, d.ID, "监控已删除、修改或暂停")
		return true
	}
	loc, _ := time.LoadLocation(c.Settings.TimeZone)
	for _, rule := range m.Rules {
		if rule.ID == d.RuleID && !withinWindow(rule, time.Now(), loc) {
			_ = e.Store.CancelDelivery(ctx, d.ID, "已超出发送时段，等待下一次提醒")
			return true
		}
	}
	targets := c.Targets([]string{d.Target})
	if len(targets) == 0 {
		err = fmt.Errorf("通知方式不可用")
	} else {
		t := targets[0]
		err = e.Sender.Send(ctx, t.Channel, t.Method.Address, d.Payload, d.ID)
	}

	if finishErr := e.Store.FinishDelivery(ctx, d, err); finishErr != nil {
		slog.Error("投递状态保存失败")
	}
	lock := e.lock(m.ID)
	lock.Lock()
	defer lock.Unlock()
	rt, getErr := e.Store.Runtime(ctx, m.ID)
	if getErr == nil {
		level := "info"
		text := "已送达 " + d.Recipient + " · " + d.Channel
		if err != nil {
			level = "error"
			text = "投递失败：" + err.Error()
			if d.Attempts < 3 {
				text += "，稍后重试"
			}
			rt.Incident = &domain.Incident{At: time.Now().UnixMilli(), Message: text}
		} else if rt.Incident != nil && strings.HasPrefix(rt.Incident.Message, "投递失败") {
			rt.Incident.Resolved = true
		}
		rt.Send = text
		logs := []domain.Log{}
		if ShouldLog(m, "send", level) && e.Store.HasRoom(c.Settings.DiskLimitMB) {
			logs = append(logs, domain.Log{MonitorID: m.ID, At: time.Now().UnixMilli(), Stage: "send", Level: level, Message: text})
		}
		e.mu.RLock()
		current, exists := e.cfg.FindMonitor(m.ID)
		if exists && current.Generation == m.Generation {
			_ = e.Store.SaveRun(ctx, rt, nil, logs, nil, false, m.Storage)
		}
		e.mu.RUnlock()
	}
	return true
}

func (e *Engine) Preview(ctx context.Context, s domain.Source) (source.Preview, error) {
	select {
	case e.slots <- struct{}{}:
		defer func() { <-e.slots }()
	case <-ctx.Done():
		return source.Preview{}, ctx.Err()
	}
	return e.Fetcher.Fetch(ctx, s)
}
