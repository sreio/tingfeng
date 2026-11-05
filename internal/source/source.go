package source

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
)

type Fetcher struct {
	Client   *http.Client
	MaxBytes int64
}
type Preview struct {
	Observation domain.Observation `json:"observation"`
	Fields      []Field            `json:"fields"`
	Millis      int64              `json:"millis"`
}
type Field struct {
	Path  string `json:"path"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

func New(n secure.Network, max int64) *Fetcher { return &Fetcher{Client: n.Client(), MaxBytes: max} }
func (f *Fetcher) Fetch(ctx context.Context, s domain.Source) (Preview, error) {
	out := Preview{Fields: []Field{}}
	start := time.Now()
	if e := domain.ValidateSource(s); e != nil {
		return out, e
	}
	u, _ := url.Parse(s.URL)
	q := u.Query()
	for k, v := range s.Query {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	req, e := http.NewRequestWithContext(ctx, s.Method, u.String(), strings.NewReader(s.Body))
	if e != nil {
		return out, fmt.Errorf("请求配置无效")
	}
	req.Header.Set("User-Agent", "Tingfeng/1.0 (+self-hosted monitor)")
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	if s.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, e := f.Client.Do(req)
	if e != nil {
		return out, fmt.Errorf("%s", secure.SafeError(e))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("来源返回 HTTP %d", resp.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, f.MaxBytes+1))
	if e != nil {
		return out, fmt.Errorf("读取响应失败")
	}
	if int64(len(data)) > f.MaxBytes {
		return out, fmt.Errorf("响应超过 %d MB 上限", f.MaxBytes>>20)
	}
	var value any
	obs := domain.Observation{CheckedAt: time.Now().UnixMilli()}
	if t, e := http.ParseTime(resp.Header.Get("Last-Modified")); e == nil {
		obs.SourceAt = t.UnixMilli()
	}
	switch s.Type {
	case "api":
		d := json.NewDecoder(bytes.NewReader(data))
		d.UseNumber()
		var root any
		if e = d.Decode(&root); e != nil {
			return out, fmt.Errorf("响应不是有效的 JSON")
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return out, fmt.Errorf("响应包含多份 JSON 数据")
		}
		out.Fields = fields(root, "", 0)
		value, e = Extract(root, s.Path)
		if e != nil {
			return out, e
		}
		if s.TimePath != "" {
			t, err := Extract(root, s.TimePath)
			if err != nil {
				return out, fmt.Errorf("来源时间字段不存在")
			}
			obs.SourceAt = parseTime(fmt.Sprint(t))
		}
	case "page":
		reader, err := charset.NewReader(bytes.NewReader(data), resp.Header.Get("Content-Type"))
		if err != nil {
			return out, fmt.Errorf("网页编码无法识别")
		}
		root, err := html.Parse(reader)
		if err != nil {
			return out, fmt.Errorf("网页解析失败")
		}
		selector := s.Selector
		if selector == "" {
			selector = "body"
		}
		matcher, err := cascadia.Compile(selector)
		if err != nil {
			return out, fmt.Errorf("CSS 选择器无效")
		}
		nodes := cascadia.QueryAll(root, matcher)
		if len(nodes) == 0 {
			return out, fmt.Errorf("页面没有匹配的元素，请检查选择器；JavaScript 渲染页面需要使用其数据接口")
		}
		parts := []string{}
		for _, n := range nodes {
			if len(parts) >= 100 {
				break
			}
			if s.Attribute != "" {
				for _, a := range n.Attr {
					if a.Key == s.Attribute {
						parts = append(parts, a.Val)
					}
				}
			} else {
				parts = append(parts, nodeText(n))
			}
		}
		if len(parts) == 0 {
			return out, fmt.Errorf("匹配元素中没有指定属性")
		}
		value = strings.TrimSpace(strings.Join(parts, "\n"))
	case "rss":
		items, err := parseFeed(data)
		if err != nil {
			return out, err
		}
		obs.Items = items
		if len(items) > 0 {
			value = items[0].Title
			obs.SourceAt = items[0].PublishedAt
		} else {
			value = "订阅暂无条目"
		}
	}
	obs.Value, e = json.Marshal(value)
	if e != nil {
		return out, fmt.Errorf("结果无法序列化")
	}
	if len(obs.Value) > 32768 {
		return out, fmt.Errorf("提取结果超过 32 KB，请选择更具体的字段或元素")
	}
	obs.Display = domain.Display(obs.Value)
	serialized, _ := json.Marshal(obs)
	if len(serialized) > 128<<10 {
		return out, fmt.Errorf("订阅内容超过 128 KB，请减少来源的条目数量")
	}
	out.Observation = obs
	out.Millis = time.Since(start).Milliseconds()
	return out, nil
}

// Paths support dot/index notation and RFC 6901 JSON Pointer, without executing expressions.
func Extract(root any, path string) (any, error) {
	if path == "" || path == "$" {
		return root, nil
	}
	var tokens []string
	if strings.HasPrefix(path, "/") {
		for _, p := range strings.Split(path[1:], "/") {
			tokens = append(tokens, strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~"))
		}
	} else {
		path = strings.TrimPrefix(path, "$.")
		path = strings.ReplaceAll(path, "[", ".")
		path = strings.ReplaceAll(path, "]", "")
		tokens = strings.Split(path, ".")
	}
	value := root
	for _, key := range tokens {
		if key == "" {
			return nil, fmt.Errorf("字段路径无效")
		}
		switch v := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = v[key]
			if !ok {
				return nil, fmt.Errorf("没有找到字段，请根据测试结果选择路径")
			}
		case []any:
			i, e := strconv.Atoi(key)
			if e != nil || i < 0 || i >= len(v) {
				return nil, fmt.Errorf("数组索引不存在")
			}
			value = v[i]
		default:
			return nil, fmt.Errorf("字段路径无法继续提取")
		}
	}
	return value, nil
}
func fields(v any, path string, depth int) []Field {
	if depth > 5 {
		return nil
	}
	out := []Field{}
	switch x := v.(type) {
	case map[string]any:
		keys := []string{}
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || lower == "authorization" {
				continue
			}
			p := path + "/" + strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
			out = append(out, fields(x[k], p, depth+1)...)
			if len(out) >= 40 {
				out = out[:40]
				break
			}
		}
	case []any:
		for i := 0; i < len(x) && i < 3; i++ {
			out = append(out, fields(x[i], path+"/"+strconv.Itoa(i), depth+1)...)
		}
	default:
		b, _ := json.Marshal(v)
		value := string(b)
		if len(value) > 120 {
			value = value[:120] + "…"
		}
		kind := "文本"
		switch x.(type) {
		case json.Number:
			kind = "数值"
		case bool:
			kind = "布尔"
		case nil:
			kind = "空值"
		}
		out = append(out, Field{path, value, kind})
	}
	return out
}
func nodeText(n *html.Node) string {
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript") {
		return ""
	}
	if n.Type == html.TextNode {
		return strings.Join(strings.Fields(n.Data), " ")
	}
	var parts []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := nodeText(c); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}
func parseTime(s string) int64 {
	if n, e := strconv.ParseFloat(s, 64); e == nil {
		if n > 1e12 {
			return int64(n)
		}
		if n > 1e9 {
			return int64(n * 1000)
		}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, e := time.Parse(layout, s); e == nil {
			return t.UnixMilli()
		}
	}
	return 0
}
func itemID(id, link, title string) string {
	if id == "" {
		id = link
	}
	if id == "" {
		id = title
	}
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:])
}
