package domain

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"
)

type Config struct {
	Revision   int64       `json:"revision"`
	Monitors   []Monitor   `json:"monitors"`
	Recipients []Recipient `json:"recipients"`
	Channels   []Channel   `json:"channels"`
	Settings   Settings    `json:"settings"`
}
type Settings struct {
	AllowPrivate        *bool  `json:"allowPrivate,omitempty"`
	DeliveryConcurrency int    `json:"deliveryConcurrency"`
	TimeZone            string `json:"timeZone"`
	HistoryDays         int    `json:"historyDays"`
	MaxRows             int    `json:"maxRows"`
	LogLevel            string `json:"logLevel"`
	LogDays             int    `json:"logDays"`
	DeliveryDays        int    `json:"deliveryDays"`
	DiskLimitMB         int    `json:"diskLimitMB"`
}
type Monitor struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Enabled     bool    `json:"enabled"`
	Draft       bool    `json:"draft"`
	NeedsReview bool    `json:"needsReview,omitempty"`
	Source      Source  `json:"source"`
	Interval    int     `json:"interval"`
	Rules       []Rule  `json:"rules"`
	Storage     Storage `json:"storage"`
	Generation  int64   `json:"generation"`
	CopyFrom    string  `json:"copyFrom,omitempty"`
	ActivatedAt int64   `json:"activatedAt"`
}
type Source struct {
	Type                string            `json:"type"`
	URL                 string            `json:"url"`
	Method              string            `json:"method"`
	Query               map[string]string `json:"query,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	Body                string            `json:"body,omitempty"`
	Path                string            `json:"path"`
	Selector            string            `json:"selector"`
	Attribute           string            `json:"attribute"`
	TimePath            string            `json:"timePath"`
	Unit                string            `json:"unit"`
	Fields              []SourceField     `json:"fields,omitempty"`
	HasRequestSecrets   bool              `json:"hasRequestSecrets,omitempty"`
	ClearRequestSecrets bool              `json:"clearRequestSecrets,omitempty"`
}
type Rule struct {
	ID          string   `json:"id"`
	Field       string   `json:"field,omitempty"`
	Trigger     string   `json:"trigger"`
	Operator    string   `json:"operator"`
	Threshold   float64  `json:"threshold"`
	Cooldown    int      `json:"cooldown"`
	Every       int      `json:"every"`
	Unit        string   `json:"unit"`
	Timing      string   `json:"timing"`
	WindowStart string   `json:"windowStart"`
	WindowEnd   string   `json:"windowEnd"`
	Time        string   `json:"time"`
	Selections  []string `json:"selections"`
	Template    string   `json:"template,omitempty"`
}
type Storage struct {
	History  bool            `json:"history"`
	Days     int             `json:"days"`
	MaxRows  int             `json:"maxRows"`
	LogLevel string          `json:"logLevel"`
	LogDays  int             `json:"logDays"`
	Stages   map[string]bool `json:"stages"`
}
type Recipient struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Note    string   `json:"note"`
	Methods []Method `json:"methods"`
}
type Method struct {
	ID        string `json:"id"`
	ChannelID string `json:"channelId"`
	Address   string `json:"address"`
	Default   bool   `json:"default"`
}
type Channel struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Enabled      bool              `json:"enabled"`
	URL          string            `json:"url,omitempty"`
	Secret       string            `json:"secret,omitempty"`
	Host         string            `json:"host,omitempty"`
	Port         int               `json:"port,omitempty"`
	User         string            `json:"user,omitempty"`
	Password     string            `json:"password,omitempty"`
	Sender       string            `json:"sender,omitempty"`
	TLS          string            `json:"tls,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyTemplate string            `json:"bodyTemplate,omitempty"`
	Method       string            `json:"method,omitempty"`
	SecretsSet   []string          `json:"secretsSet,omitempty"`
	ClearSecrets bool              `json:"clearSecrets,omitempty"`
}
type Target struct {
	Key     string
	Person  Recipient
	Method  Method
	Channel Channel
}
type Item struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	PublishedAt int64  `json:"publishedAt,omitempty"`
}
type Observation struct {
	Value     json.RawMessage `json:"value"`
	Display   string          `json:"display"`
	Fields    []FieldValue    `json:"fields,omitempty"`
	Items     []Item          `json:"items,omitempty"`
	SourceAt  int64           `json:"sourceAt"`
	CheckedAt int64           `json:"checkedAt"`
}
type RuleState struct {
	LastQueued int64  `json:"lastQueued"`
	NextSend   int64  `json:"nextSend"`
	Anchor     int64  `json:"anchor"`
	Signature  string `json:"signature"`
}
type Incident struct {
	At       int64  `json:"at"`
	Message  string `json:"message"`
	Resolved bool   `json:"resolved"`
}
type Runtime struct {
	MonitorID   string               `json:"monitorId"`
	Generation  int64                `json:"generation"`
	NextCheck   int64                `json:"nextCheck"`
	LastCheck   int64                `json:"lastCheck"`
	LastSuccess int64                `json:"lastSuccess"`
	Failures    int                  `json:"failures"`
	Latest      *Observation         `json:"latest"`
	Fetch       string               `json:"fetch"`
	Evaluate    string               `json:"evaluate"`
	Send        string               `json:"send"`
	Incident    *Incident            `json:"incident,omitempty"`
	Seen        []string             `json:"seen,omitempty"`
	Rules       map[string]RuleState `json:"rules,omitempty"`
}
type Payload struct {
	Fields        map[string]string `json:"fields,omitempty"`
	Units         map[string]string `json:"units,omitempty"`
	RawFields     map[string]string `json:"rawFields,omitempty"`
	Title         string            `json:"title"`
	Text          string            `json:"text"`
	Monitor       string            `json:"monitor"`
	Value         string            `json:"value"`
	Previous      string            `json:"previous"`
	Rule          string            `json:"rule"`
	SourceURL     string            `json:"sourceURL"`
	SourceTime    string            `json:"sourceTime"`
	CollectedTime string            `json:"collectedTime"`
	Items         []Item            `json:"items,omitempty"`
}
type Delivery struct {
	ID          string  `json:"id"`
	MonitorID   string  `json:"monitorId"`
	RuleID      string  `json:"ruleId"`
	Generation  int64   `json:"generation"`
	Target      string  `json:"target"`
	Recipient   string  `json:"recipient"`
	Channel     string  `json:"channel"`
	Kind        string  `json:"kind"`
	Key         string  `json:"-"`
	Payload     Payload `json:"payload"`
	Status      string  `json:"status"`
	Attempts    int     `json:"attempts"`
	NextAttempt int64   `json:"nextAttempt"`
	CreatedAt   int64   `json:"createdAt"`
	UpdatedAt   int64   `json:"updatedAt"`
	Error       string  `json:"error,omitempty"`
}
type Record struct {
	Fields   []FieldValue    `json:"fields,omitempty"`
	ID       int64           `json:"id"`
	At       int64           `json:"at"`
	SourceAt int64           `json:"sourceAt"`
	Value    json.RawMessage `json:"value"`
	Display  string          `json:"display"`
}
type Log struct {
	ID        int64  `json:"id"`
	MonitorID string `json:"monitorId"`
	At        int64  `json:"at"`
	Stage     string `json:"stage"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

func ID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

const DefaultDeliveryConcurrency = 5
const MaxDeliveryConcurrency = 20

func DefaultSettings() Settings {
	return Settings{TimeZone: "Asia/Shanghai", HistoryDays: 7, MaxRows: 10000, LogLevel: "error", LogDays: 3, DeliveryDays: 7, DiskLimitMB: 200, DeliveryConcurrency: DefaultDeliveryConcurrency}
}
func EmptyConfig() Config {
	return Config{Revision: 1, Monitors: []Monitor{}, Recipients: []Recipient{}, Channels: []Channel{}, Settings: DefaultSettings()}
}
func Clone[T any](v T) T { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }
func (c Config) FindMonitor(id string) (Monitor, bool) {
	for _, m := range c.Monitors {
		if m.ID == id {
			return m, true
		}
	}
	return Monitor{}, false
}
func (c Config) FindChannel(id string) (Channel, bool) {
	for _, v := range c.Channels {
		if v.ID == id {
			return v, true
		}
	}
	return Channel{}, false
}
func (c Config) Targets(keys []string) []Target {
	out := []Target{}
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		pid, mid, ok := strings.Cut(key, ":")
		if !ok {
			continue
		}
		for _, p := range c.Recipients {
			if p.ID != pid {
				continue
			}
			for _, m := range p.Methods {
				if m.ID != mid {
					continue
				}
				ch, ok := c.FindChannel(m.ChannelID)
				if ok && ch.Enabled {
					out = append(out, Target{key, p, m, ch})
				}
			}
		}
	}
	return out
}
func PublicURL(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
func Redacted(c Config) Config {
	c = Clone(c)
	for i := range c.Channels {
		ch := &c.Channels[i]
		ch.SecretsSet = []string{}
		for name, val := range map[string]string{"url": ch.URL, "secret": ch.Secret, "password": ch.Password} {
			if val != "" {
				ch.SecretsSet = append(ch.SecretsSet, name)
			}
		}
		if len(ch.Headers) > 0 {
			ch.SecretsSet = append(ch.SecretsSet, "headers")
		}
		ch.URL = ""
		ch.Secret = ""
		ch.Password = ""
		ch.Headers = nil
	}
	for i := range c.Monitors {
		s := &c.Monitors[i].Source
		s.HasRequestSecrets = s.URL != PublicURL(s.URL) || len(s.Query) > 0 || len(s.Headers) > 0 || s.Body != ""
		s.URL = PublicURL(s.URL)
		s.Query = nil
		s.Headers = nil
		s.Body = ""
	}
	return c
}
func MergeSecrets(next *Config, old Config) {
	for i := range next.Channels {
		n := &next.Channels[i]
		o, ok := old.FindChannel(n.ID)
		if ok && o.Type == n.Type && !n.ClearSecrets {
			if n.URL == "" {
				n.URL = o.URL
			}
			if n.Secret == "" && sameOrigin(n.URL, o.URL) {
				n.Secret = o.Secret
			}
			if n.Password == "" && n.Host == o.Host && n.User == o.User {
				n.Password = o.Password
			}
			if len(n.Headers) == 0 && sameOrigin(n.URL, o.URL) {
				n.Headers = o.Headers
			}
		}
		n.ClearSecrets = false
		n.SecretsSet = nil
	}
	for i := range next.Monitors {
		n := &next.Monitors[i]
		o, ok := old.FindMonitor(n.ID)
		if !ok && n.CopyFrom != "" {
			o, ok = old.FindMonitor(n.CopyFrom)
		}
		n.CopyFrom = ""
		if ok {
			n.Source = MergeSource(n.Source, o.Source)
		}
		n.Source.HasRequestSecrets = false
		n.Source.ClearRequestSecrets = false
	}
}
func sameOrigin(a, b string) bool {
	x, e := url.Parse(a)
	y, f := url.Parse(b)
	return e == nil && f == nil && x.Scheme == y.Scheme && x.Host == y.Host
}
func MergeSource(n, o Source) Source {
	nu, _ := url.Parse(n.URL)
	ou, _ := url.Parse(o.URL)
	if !n.ClearRequestSecrets && n.Type == o.Type && nu != nil && ou != nil && nu.Scheme == ou.Scheme && nu.Host == ou.Host {
		if n.URL == PublicURL(o.URL) {
			n.URL = o.URL
		}
		if len(n.Headers) == 0 {
			n.Headers = o.Headers
		}
		if len(n.Query) == 0 {
			n.Query = o.Query
		}
		if n.Body == "" {
			n.Body = o.Body
		}
	}
	return n
}
func Prepare(next *Config, old Config, now time.Time) {
	next.Revision = old.Revision + 1
	for i := range next.Monitors {
		n := &next.Monitors[i]
		o, ok := old.FindMonitor(n.ID)
		n.ActivatedAt = o.ActivatedAt
		if n.Enabled && (!ok || !o.Enabled) {
			n.ActivatedAt = now.UnixMilli()
		}
		n.Generation = o.Generation
		if !ok {
			n.Generation = now.UnixMilli()
		} else if !reflect.DeepEqual(*n, o) || next.Settings.TimeZone != old.Settings.TimeZone {
			n.ActivatedAt = o.ActivatedAt
			if n.Enabled && (!ok || !o.Enabled) {
				n.ActivatedAt = now.UnixMilli()
			}
			n.Generation = o.Generation + 1
		}
	}
}
func Display(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return string(v)
}
func RuleLabel(r Rule) string {
	switch r.Trigger {
	case "threshold":
		if r.Operator == "lt" {
			return fmt.Sprintf("向下越过 %g", r.Threshold)
		}
		return fmt.Sprintf("向上越过 %g", r.Threshold)
	case "new":
		return "发现新条目"
	case "scheduled":
		return "每天 " + r.Time + " 发送"
	case "periodic":
		u := map[string]string{"minute": "分钟", "hour": "小时", "day": "天"}[r.Unit]
		s := fmt.Sprintf("每 %d %s发送一次", r.Every, u)
		if r.Timing == "window" {
			s += "（" + r.WindowStart + "–" + r.WindowEnd + "）"
		}
		return s
	default:
		return "数据有变化"
	}
}
