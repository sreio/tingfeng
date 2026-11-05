package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
	"unicode/utf8"
)

type Sender struct {
	Network   secure.Network
	Client    *http.Client
	TLSConfig *tls.Config
}

func New(n secure.Network) *Sender { return &Sender{Network: n, Client: n.Client()} }

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("消息模板结果过长")
	}
	return b.Buffer.Write(p)
}
func Render(t string, p domain.Payload, limit int) (string, error) {
	tpl, e := domain.ParseTemplate(t)
	if e != nil {
		return "", fmt.Errorf("消息模板语法有误")
	}
	b := limitedBuffer{limit: limit}
	if e = tpl.Execute(&b, p); e != nil {
		return "", fmt.Errorf("消息模板执行失败，请检查变量和长度")
	}
	return b.String(), nil
}
func Payload(m domain.Monitor, r domain.Rule, obs domain.Observation, previous string, items []domain.Item, loc *time.Location) (domain.Payload, error) {
	stamp := func(n int64) string {
		if n == 0 {
			return "来源未提供"
		}
		return time.UnixMilli(n).In(loc).Format("2006-01-02 15:04:05")
	}
	p := domain.Payload{Title: m.Name + " · " + domain.RuleLabel(r), Monitor: m.Name, Value: obs.Display, Previous: previous, Rule: domain.RuleLabel(r), SourceURL: domain.PublicURL(m.Source.URL), SourceTime: stamp(obs.SourceAt), CollectedTime: stamp(obs.CheckedAt), Items: items}
	if r.Trigger == "periodic" || r.Trigger == "scheduled" {
		p.Text = "最新值：" + clip(obs.Display, 600) + m.Source.Unit
	} else if r.Trigger == "new" {
		lines := []string{fmt.Sprintf("发现 %d 条新内容", len(items))}
		for i, it := range items {
			if i == 10 {
				lines = append(lines, "其余条目可在后台查看")
				break
			}
			line := "• " + clip(it.Title, 200)
			if it.Link != "" {
				line += "\n" + it.Link
			}
			lines = append(lines, line)
		}
		p.Text = strings.Join(lines, "\n")
	} else {
		p.Text = clip(previous, 400) + " → " + clip(obs.Display, 400) + m.Source.Unit
	}
	p.Text = p.Title + "\n" + p.Text + "\n来源更新：" + p.SourceTime + "\n采集时间：" + p.CollectedTime + "\n查看来源：" + p.SourceURL
	if r.Template != "" {
		text, e := Render(r.Template, p, 16000)
		if e != nil {
			return p, e
		}
		p.Text = text
	}
	return p, nil
}
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}

var sc3 = regexp.MustCompile(`^sctp([0-9]+)t[A-Za-z0-9_-]+$`)
var sct = regexp.MustCompile(`^SCT[A-Za-z0-9_-]+$`)

func (s *Sender) Send(ctx context.Context, c domain.Channel, address string, p domain.Payload, id string) error {
	if e := domain.ValidateChannel(c, true); e != nil {
		return e
	}
	if c.Type == "email" {
		return s.email(ctx, c, address, p, id)
	}
	rawURL := c.URL
	method := "POST"
	contentType := "application/json"
	var body []byte
	text := p.Text
	payload := map[string]any{}
	switch c.Type {
	case "wecom":
		payload = map[string]any{"msgtype": "text", "text": map[string]string{"content": clip(text, 1900)}}
	case "dingtalk":
		payload = map[string]any{"msgtype": "text", "text": map[string]string{"content": clip(text, 16000)}}
		if c.Secret != "" {
			timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
			mac := hmac.New(sha256.New, []byte(c.Secret))
			_, _ = mac.Write([]byte(timestamp + "\n" + c.Secret))
			u, _ := url.Parse(rawURL)
			q := u.Query()
			q.Set("timestamp", timestamp)
			q.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
			u.RawQuery = q.Encode()
			rawURL = u.String()
		}
	case "feishu":
		payload = map[string]any{"msg_type": "text", "content": map[string]string{"text": clip(text, 16000)}}
		if c.Secret != "" {
			ts := strconv.FormatInt(time.Now().Unix(), 10)
			mac := hmac.New(sha256.New, []byte(ts+"\n"+c.Secret))
			payload["timestamp"] = ts
			payload["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
		}
	case "serverchan":
		if match := sc3.FindStringSubmatch(c.Secret); match != nil {
			rawURL = "https://" + match[1] + ".push.ft07.com/send/" + c.Secret + ".send"
		} else if sct.MatchString(c.Secret) {
			rawURL = "https://sctapi.ftqq.com/" + c.Secret + ".send"
		} else {
			return fmt.Errorf("SendKey 格式无效，应以 SCT 或 sctp 开头")
		}
		contentType = "application/x-www-form-urlencoded"
		body = []byte(url.Values{"title": {clip(p.Title, 90)}, "desp": {p.Text}}.Encode())
	case "webhook":
		if c.Method != "" {
			method = c.Method
		}
		if c.BodyTemplate != "" {
			v, e := Render(c.BodyTemplate, p, 32768)
			if e != nil {
				return e
			}
			if !json.Valid([]byte(v)) {
				return fmt.Errorf("Webhook 模板输出不是有效 JSON，请用 json 函数转义变量")
			}
			body = []byte(v)
		} else {
			body, _ = json.Marshal(p)
		}
	default:
		return fmt.Errorf("不支持的通知渠道")
	}
	if body == nil {
		body, _ = json.Marshal(payload)
	}
	req, e := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if e != nil {
		return fmt.Errorf("通知地址无效")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "Tingfeng/1.0")
	req.Header.Set("X-Tingfeng-Delivery-ID", id)
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	resp, e := s.Client.Do(req)
	if e != nil {
		return fmt.Errorf("%s", secure.SafeError(e))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("通知服务返回 HTTP %d", resp.StatusCode)
	}
	if c.Type == "webhook" {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if e != nil || len(data) > 65536 {
		return fmt.Errorf("通知服务响应过大或读取失败")
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(data, &result) != nil {
		return fmt.Errorf("通知服务未返回有效的确认结果")
	}
	keys := []string{"errcode"}
	if c.Type == "feishu" {
		keys = []string{"code", "StatusCode"}
	}
	if c.Type == "serverchan" {
		keys = []string{"code", "errno"}
	}
	found := false
	for _, key := range keys {
		if v, ok := result[key]; ok {
			found = true
			var n int
			if json.Unmarshal(v, &n) != nil {
				return fmt.Errorf("通知服务确认格式无效")
			}
			if n != 0 {
				return fmt.Errorf("通知服务错误码 %d，请检查机器人权限、签名、关键词或额度", n)
			}
		}
	}
	if !found {
		return fmt.Errorf("通知服务没有明确确认成功")
	}
	return nil
}
