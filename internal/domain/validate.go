package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"text/template"
	"time"
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)
var clock = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func validID(s string) bool { return identifier.MatchString(s) }
func named(id, name string, seen map[string]bool) error {
	if !validID(id) || seen[id] {
		return fmt.Errorf("配置标识无效或重复")
	}
	seen[id] = true
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 100 {
		return fmt.Errorf("名称必须为 1～100 个字符")
	}
	return nil
}
func integer(v, a, b int) bool { return v >= a && v <= b }
func HTTPURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || len(raw) > 4096 {
		return fmt.Errorf("请输入完整的 HTTP(S) 地址，认证信息请填在请求头中")
	}
	return nil
}
func ValidateSource(s Source) error {
	if e := ValidateFields(s); e != nil {
		return e
	}
	if s.Type != "api" && s.Type != "page" && s.Type != "rss" {
		return fmt.Errorf("来源类型无效")
	}
	if e := HTTPURL(s.URL); e != nil {
		return e
	}
	if s.Method != "GET" && s.Method != "POST" {
		return fmt.Errorf("请求方法只支持 GET / POST")
	}
	if s.Type != "api" && s.Method != "GET" {
		return fmt.Errorf("网页与 RSS 请使用 GET")
	}
	if len(s.Body) > 65536 || len(s.Path) > 256 || len(s.Selector) > 512 || len(s.Attribute) > 100 || len(s.TimePath) > 256 || len(s.Unit) > 30 {
		return fmt.Errorf("来源配置过长")
	}
	if e := ValidateHeaders(s.Headers); e != nil {
		return e
	}
	if len(s.Query) > 64 {
		return fmt.Errorf("请求参数过多")
	}
	for k, v := range s.Query {
		if len(k) > 200 || len(v) > 4096 {
			return fmt.Errorf("请求参数过长")
		}
	}
	return nil
}
func ValidateHeaders(h map[string]string) error {
	if len(h) > 32 {
		return fmt.Errorf("请求头最多 32 项")
	}
	for k, v := range h {
		if strings.TrimSpace(k) == "" || len(k) > 100 || len(v) > 8192 || strings.ContainsAny(k+v, "\r\n\x00") {
			return fmt.Errorf("请求头格式无效")
		}
		for _, c := range k {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
				return fmt.Errorf("请求头名称无效")
			}
		}
		switch strings.ToLower(k) {
		case "host", "content-length", "connection", "transfer-encoding", "proxy-authorization":
			return fmt.Errorf("不支持自定义 %s 请求头", k)
		}
	}
	return nil
}
func ParseTemplate(s string) (*template.Template, error) {
	return template.New("message").Option("missingkey=error").Funcs(template.FuncMap{"json": func(v any) string { b, _ := json.Marshal(v); return string(b) }}).Parse(s)
}
func ValidateRule(r Rule) error {
	if r.Field != "" && !validFieldKey(r.Field) {
		return fmt.Errorf("判断字段变量名无效")
	}
	if !validID(r.ID) {
		return fmt.Errorf("提醒标识无效")
	}
	if len(r.Selections) > 128 || len(r.Template) > 4096 {
		return fmt.Errorf("提醒配置过长")
	}
	if r.Template != "" {
		if _, e := ParseTemplate(r.Template); e != nil {
			return fmt.Errorf("消息模板语法有误")
		}
	}
	switch r.Trigger {
	case "scheduled":
		if !clock.MatchString(r.Time) {
			return fmt.Errorf("请选择每天的发送时间")
		}
	case "periodic":
		u := map[string]int{"minute": 1, "hour": 60, "day": 1440}[r.Unit]
		if u == 0 || r.Every < 1 || r.Every > 525600/u {
			return fmt.Errorf("发送间隔必须为正整数，最长 365 天")
		}
		if r.Timing != "always" && r.Timing != "window" {
			return fmt.Errorf("请选择发送时段")
		}
		if r.Timing == "window" {
			if !clock.MatchString(r.WindowStart) || !clock.MatchString(r.WindowEnd) || r.WindowStart == r.WindowEnd {
				return fmt.Errorf("请设置有效且不同的时段起止时间")
			}
			length := (ClockMinutes(r.WindowEnd) - ClockMinutes(r.WindowStart) + 1440) % 1440
			if r.Every*u > length {
				return fmt.Errorf("发送间隔不能超过所选时段；跨天周期请选择全天")
			}
		}
	case "change", "new", "threshold":
		if !integer(r.Cooldown, 0, 1440) {
			return fmt.Errorf("冷却时间为 0～1440 分钟")
		}
		if r.Trigger == "threshold" && (r.Operator != "gt" && r.Operator != "lt" || math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0)) {
			return fmt.Errorf("阈值或比较方向无效")
		}
	default:
		return fmt.Errorf("提醒类型无效")
	}
	return nil
}
func ClockMinutes(s string) int { var h, m int; _, _ = fmt.Sscanf(s, "%d:%d", &h, &m); return h*60 + m }
func ValidateChannel(c Channel, requireSecrets bool) error {
	if c.Type == "email" {
		if c.Host == "" || strings.ContainsAny(c.Host, "/\\ \r\n") || !integer(c.Port, 1, 65535) {
			return fmt.Errorf("SMTP 地址或端口无效")
		}
		if _, e := mail.ParseAddress(c.Sender); e != nil || strings.ContainsAny(c.Sender, "\r\n") {
			return fmt.Errorf("发件邮箱无效")
		}
		if c.TLS != "tls" && c.TLS != "starttls" {
			return fmt.Errorf("SMTP 必须启用 TLS 或 STARTTLS")
		}
	} else if c.Type == "serverchan" {
		if requireSecrets && c.Secret == "" {
			return fmt.Errorf("请填写 Server 酱 SendKey")
		}
	} else {
		if c.Type != "wecom" && c.Type != "dingtalk" && c.Type != "feishu" && c.Type != "webhook" {
			return fmt.Errorf("渠道类型无效")
		}
		if requireSecrets || c.URL != "" {
			if e := HTTPURL(c.URL); e != nil {
				return e
			}
		}
	}
	if len(c.Password) > 8192 || len(c.Secret) > 8192 || len(c.User) > 320 || len(c.Host) > 253 || len(c.BodyTemplate) > 8192 {
		return fmt.Errorf("渠道字段过长")
	}
	if e := ValidateHeaders(c.Headers); e != nil {
		return e
	}
	if c.Type == "webhook" && (c.Method != "POST" && c.Method != "PUT" && c.Method != "") {
		return fmt.Errorf("Webhook 支持 POST 或 PUT")
	}
	if c.BodyTemplate != "" {
		if _, e := ParseTemplate(c.BodyTemplate); e != nil {
			return fmt.Errorf("Webhook 模板语法有误")
		}
	}
	return nil
}
func ValidateDeliveryConcurrency(n int) error {
	if !integer(n, 1, MaxDeliveryConcurrency) {
		return fmt.Errorf("通知发送并发数必须为 1～%d 的整数", MaxDeliveryConcurrency)
	}
	return nil
}

func Validate(c Config) error {
	if len(c.Monitors) > 500 || len(c.Recipients) > 500 || len(c.Channels) > 500 {
		return fmt.Errorf("单实例最多配置各 500 项")
	}
	st := c.Settings
	if err := ValidateDeliveryConcurrency(st.DeliveryConcurrency); err != nil {
		return err
	}
	if _, e := time.LoadLocation(st.TimeZone); e != nil {
		return fmt.Errorf("时区无效")
	}
	if !integer(st.HistoryDays, 1, 365) || !integer(st.MaxRows, 1, 100000) || !integer(st.LogDays, 1, 30) || !integer(st.DeliveryDays, 1, 90) || !integer(st.DiskLimitMB, 32, 102400) || !logLevel(st.LogLevel) {
		return fmt.Errorf("默认保留策略无效")
	}
	seen := map[string]bool{}
	for _, ch := range c.Channels {
		if e := named(ch.ID, ch.Name, seen); e != nil {
			return e
		}
		if e := ValidateChannel(ch, !ch.ClearSecrets && ch.Enabled); e != nil {
			return fmt.Errorf("渠道「%s」：%w", ch.Name, e)
		}
	}
	keys := map[string]bool{}
	seen = map[string]bool{}
	for _, p := range c.Recipients {
		if e := named(p.ID, p.Name, seen); e != nil {
			return e
		}
		if p.Kind != "person" && p.Kind != "group" {
			return fmt.Errorf("接收人类型无效")
		}
		if len(p.Methods) > 32 || len(p.Note) > 1000 {
			return fmt.Errorf("接收人配置过长")
		}
		ms := map[string]bool{}
		for _, m := range p.Methods {
			if !validID(m.ID) || ms[m.ID] {
				return fmt.Errorf("通知方式标识无效或重复")
			}
			ms[m.ID] = true
			ch, ok := c.FindChannel(m.ChannelID)
			if !ok {
				return fmt.Errorf("接收人「%s」引用了不存在的渠道", p.Name)
			}
			if ch.Type == "email" {
				if _, e := mail.ParseAddress(m.Address); e != nil || strings.ContainsAny(m.Address, "\r\n") {
					return fmt.Errorf("请为「%s」填写有效的接收邮箱", p.Name)
				}
			}
			if len(m.Address) > 320 {
				return fmt.Errorf("接收地址过长")
			}
			keys[p.ID+":"+m.ID] = true
		}
	}
	seen = map[string]bool{}
	for _, m := range c.Monitors {
		if e := named(m.ID, m.Name, seen); e != nil {
			return e
		}
		if m.Draft && m.Enabled {
			return fmt.Errorf("草稿不能启用")
		}
		if m.NeedsReview && m.Enabled {
			return fmt.Errorf("恢复的监控需完成测试后再启用")
		}
		if !integer(m.Interval, 1, 1440) {
			return fmt.Errorf("采集间隔为 1～1440 分钟")
		}
		if !m.Draft {
			if e := ValidateSource(m.Source); e != nil {
				return fmt.Errorf("监控「%s」：%w", m.Name, e)
			}
		}
		st := m.Storage
		if !integer(st.Days, 1, 365) || !integer(st.MaxRows, 1, 100000) || !integer(st.LogDays, 1, 30) || !logLevel(st.LogLevel) {
			return fmt.Errorf("监控「%s」的保留策略无效", m.Name)
		}
		if len(m.Rules) > 24 || !m.Draft && len(m.Rules) == 0 {
			return fmt.Errorf("每个监控需要 1～24 条提醒")
		}
		rs := map[string]bool{}
		for _, r := range m.Rules {
			if rs[r.ID] {
				return fmt.Errorf("提醒标识重复")
			}
			rs[r.ID] = true
			if e := ValidateRule(r); e != nil {
				return fmt.Errorf("监控「%s」：%w", m.Name, e)
			}
			if !m.Draft {
				if e := ValidateRuleField(r, m.Source); e != nil {
					return fmt.Errorf("监控「%s」：%w", m.Name, e)
				}
			}
			for _, k := range r.Selections {
				if !keys[k] {
					return fmt.Errorf("监控「%s」引用了不存在的通知方式", m.Name)
				}
			}
			if m.Enabled && len(r.Selections) == 0 {
				return fmt.Errorf("监控「%s」每条提醒都需要可用的接收方式", m.Name)
			}
			if r.Trigger == "new" && m.Source.Type != "rss" {
				return fmt.Errorf("新条目提醒只适用于 RSS")
			}
		}
	}
	return nil
}
func logLevel(v string) bool { return v == "off" || v == "error" || v == "summary" }
