package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
	"tingfeng/internal/auth"
	"tingfeng/internal/config"
	"tingfeng/internal/domain"
	"tingfeng/internal/engine"
	"tingfeng/internal/notify"
	"tingfeng/internal/storage"
)

type Server struct {
	Engine  *engine.Engine
	Auth    *auth.Service
	Config  config.Config
	Assets  fs.FS
	Started time.Time
}

func New(e *engine.Engine, c config.Config, assets fs.FS) *Server {
	return &Server{Engine: e, Auth: auth.New(e.Store), Config: c, Assets: assets, Started: time.Now()}
}

const cookieName = "tingfeng_session"

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if s.Engine.Store.Ping(ctx) != nil {
			write(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		write(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/auth/status", s.status)
	mux.HandleFunc("POST /api/auth/setup", s.setup)
	mux.HandleFunc("POST /api/auth/login", s.login)
	route := func(pattern string, fn http.HandlerFunc) { mux.Handle(pattern, s.protect(fn)) }
	route("POST /api/auth/logout", s.logout)
	route("POST /api/auth/password", s.password)
	route("GET /api/config", func(w http.ResponseWriter, r *http.Request) { write(w, 200, domain.Redacted(s.Engine.Config())) })
	route("PUT /api/config", s.saveConfig)
	route("POST /api/sources/test", s.testSource)
	route("POST /api/rules/preview", s.rulePreview)
	route("POST /api/channels/test", s.testChannel)
	route("POST /api/messages/preview", s.messagePreview)
	route("GET /api/monitors/status", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Engine.Store.Runtimes(r.Context())
		if internal(w, e) {
			return
		}
		for i := range out {
			out[i].Seen = nil
		}
		write(w, 200, out)
	})
	route("POST /api/monitors/{id}/check", s.check)
	route("GET /api/monitors/{id}/records", s.records)
	route("GET /api/monitors/{id}/records.csv", s.csv)
	route("DELETE /api/monitors/{id}/records", s.clearRecords)
	route("GET /api/monitors/{id}/logs", s.logs)
	route("GET /api/deliveries", s.deliveries)
	route("POST /api/deliveries/{id}/retry", s.retry)
	route("GET /api/system", s.system)
	route("POST /api/maintenance/cleanup", s.cleanup)
	route("GET /api/backup", s.backup)
	route("POST /api/restore", s.restore)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { failure(w, 404, "接口不存在") })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			failure(w, 405, "请求方法不支持")
			return
		}
		s.static(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if s.Config.CookieSecure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				failure(w, 500, "服务暂时无法完成操作")
			}
		}()
		mux.ServeHTTP(w, r)
	})
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]string{"error": msg})
}
func internal(w http.ResponseWriter, e error) bool {
	if e != nil {
		failure(w, 500, "数据操作失败，请检查服务状态及磁盘空间")
		return true
	}
	return false
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		failure(w, 415, "请使用 JSON 请求")
		return false
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		failure(w, 400, "请求内容无效、字段不支持或超过大小限制")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		failure(w, 400, "请求必须是一份 JSON")
		return false
	}
	return true
}
func (s *Server) origin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
		return false
	}
	raw := r.Header.Get("Origin")
	if raw == "" {
		return true
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if s.Config.PublicURL != "" {
		expected, e := url.Parse(s.Config.PublicURL)
		return e == nil && strings.EqualFold(u.Host, expected.Host) && u.Scheme == expected.Scheme
	}
	return strings.EqualFold(u.Host, r.Host)
}
func token(r *http.Request) string {
	c, e := r.Cookie(cookieName)
	if e != nil {
		return ""
	}
	return c.Value
}
func (s *Server) protect(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, e := s.Auth.Session(r.Context(), token(r))
		if e != nil {
			failure(w, 401, "登录已过期，请重新登录")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !s.origin(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
				failure(w, 403, "请求验证失败，请刷新页面后重试")
				return
			}
		}
		next(w, r)
	})
}
func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, session auth.Session) {
	secure := s.Config.CookieSecure || r.TLS != nil || strings.HasPrefix(s.Config.PublicURL, "https://")
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: session.Token, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, Path: "/", MaxAge: 86400, Expires: time.UnixMilli(session.Expires)})
}
func (s *Server) forget(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	initialized, updated, e := s.Auth.Initialized(r.Context())
	if internal(w, e) {
		return
	}
	session, e := s.Auth.Session(r.Context(), token(r))
	ok := e == nil
	write(w, 200, map[string]any{"initialized": initialized, "authenticated": ok, "csrf": session.CSRF, "expires": session.Expires, "passwordUpdatedAt": updated})
}
func (s *Server) authInput(w http.ResponseWriter, r *http.Request) bool {
	if !s.origin(r) {
		failure(w, 403, "请求来源不允许")
		return false
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !s.Auth.Allow(ip) {
		failure(w, 429, "尝试次数过多，请 15 分钟后重试")
		return false
	}
	return true
}
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if !s.authInput(w, r) {
		return
	}
	var v struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &v) {
		return
	}
	if e := s.Auth.Setup(r.Context(), v.Token, v.Password); e != nil {
		failure(w, 400, e.Error())
		return
	}
	s.finishLogin(w, r, v.Password)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.authInput(w, r) {
		return
	}
	var v struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &v) {
		return
	}
	s.finishLogin(w, r, v.Password)
}
func (s *Server) finishLogin(w http.ResponseWriter, r *http.Request, password string) {
	session, e := s.Auth.Login(r.Context(), password)
	if e != nil {
		failure(w, 401, "密码不正确")
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	s.Auth.Succeeded(ip)
	s.setCookie(w, r, session)
	write(w, 200, session)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if internal(w, s.Auth.Logout(r.Context(), token(r))) {
		return
	}
	s.forget(w)
	write(w, 200, map[string]bool{"ok": true})
}
func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	if !s.authInput(w, r) {
		return
	}
	var v struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !decode(w, r, &v) {
		return
	}
	if e := s.Auth.ChangePassword(r.Context(), v.Current, v.Password); e != nil {
		failure(w, 400, e.Error())
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	s.Auth.Succeeded(ip)
	s.forget(w)
	write(w, 200, map[string]bool{"ok": true})
}
func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request) {
	var c domain.Config
	c.Settings.DeliveryConcurrency = s.Engine.DeliveryConcurrency()
	if !decode(w, r, &c) {
		return
	}
	out, e := s.Engine.SaveConfig(r.Context(), c)
	if e != nil {
		status := 400
		if errors.Is(e, storage.ErrConflict) {
			status = 409
		}
		failure(w, status, e.Error())
		return
	}
	write(w, 200, out)
}
func (s *Server) testSource(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Source    domain.Source `json:"source"`
		MonitorID string        `json:"monitorId"`
	}
	if !decode(w, r, &v) {
		return
	}
	if old, ok := s.Engine.Config().FindMonitor(v.MonitorID); ok {
		v.Source = domain.MergeSource(v.Source, old.Source)
	}
	result, e := s.Engine.Preview(r.Context(), v.Source)
	if e != nil {
		write(w, 422, map[string]any{"error": e.Error(), "fields": result.Fields})
		return
	}
	write(w, 200, result)
}
func (s *Server) rulePreview(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Source   domain.Source     `json:"source"`
		Rule     domain.Rule       `json:"rule"`
		Values   []json.RawMessage `json:"values"`
		Interval int               `json:"interval"`
		Anchor   int64             `json:"anchor"`
	}
	if !decode(w, r, &v) {
		return
	}
	if e := domain.ValidateRule(v.Rule); e != nil {
		failure(w, 400, e.Error())
		return
	}
	if v.Interval < 1 || v.Interval > 1440 || len(v.Values) > 30 {
		failure(w, 400, "试算需要 1～1440 分钟间隔和最多 30 个示例值")
		return
	}
	if err := domain.ValidateFields(v.Source); err != nil {
		failure(w, 400, err.Error())
		return
	}
	if err := domain.ValidateRuleField(v.Rule, v.Source); err != nil {
		failure(w, 400, err.Error())
		return
	}
	c := s.Engine.Config()
	loc, _ := time.LoadLocation(c.Settings.TimeZone)
	now := time.Now()
	anchor := now
	if v.Anchor > 0 && v.Anchor < now.Add(366*24*time.Hour).UnixMilli() {
		anchor = time.UnixMilli(v.Anchor)
	}
	times := []int64{}
	cursor := now
	if engine.Timed(v.Rule) {
		for i := 0; i < 3; i++ {
			cursor = engine.NextSend(v.Rule, anchor, cursor, loc)
			if cursor.IsZero() {
				break
			}
			times = append(times, cursor.UnixMilli())
		}
	}
	write(w, 200, map[string]any{"next": times, "timeZone": c.Settings.TimeZone, "rows": engine.Simulate(v.Rule, v.Values, v.Interval)})
}
func (s *Server) testChannel(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Channel domain.Channel  `json:"channel"`
		Address string          `json:"address"`
		Text    string          `json:"text"`
		Payload *domain.Payload `json:"payload,omitempty"`
	}
	if !decode(w, r, &v) {
		return
	}
	current := s.Engine.Config()
	temp := domain.Config{Channels: []domain.Channel{v.Channel}}
	domain.MergeSecrets(&temp, current)
	ch := temp.Channels[0]
	text := "听风测试通知：收到这条消息说明渠道配置成功。"
	if v.Text != "" {
		if len(v.Text) > 16000 {
			failure(w, 400, "测试消息过长")
			return
		}
		text = v.Text
	}
	p := domain.Payload{Title: "听风 · 渠道测试", Text: text, Monitor: "渠道测试", Value: "连接成功", Rule: "手动测试", CollectedTime: time.Now().Format(time.RFC3339)}
	if v.Payload != nil {
		raw, _ := json.Marshal(v.Payload)
		if len(raw) > 128<<10 || len(v.Payload.Text) > 16000 || len(v.Payload.Title) > 400 || len(v.Payload.Fields) > 16 || len(v.Payload.Units) > 16 || len(v.Payload.RawFields) > 16 {
			failure(w, 400, "测试消息过长")
			return
		}
		p = *v.Payload
	}
	testID := domain.ID("test")
	sendErr := s.Engine.Sender.Send(r.Context(), ch, v.Address, p, testID)
	_ = s.Engine.Store.RecordTest(r.Context(), testID, ch.Name, v.Address, p, sendErr)
	if e := sendErr; e != nil {
		failure(w, 422, e.Error())
		return
	}
	write(w, 200, map[string]string{"message": "通知服务已确认接收，请检查你的终端。"})
}
func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	rt, e := s.Engine.Check(r.Context(), r.PathValue("id"))
	if e != nil {
		failure(w, 422, e.Error())
		return
	}
	rt.Seen = nil
	write(w, 200, rt)
}
func page(r *http.Request) (int, int) {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	n, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	return p, n
}
func (s *Server) records(w http.ResponseWriter, r *http.Request) {
	p, n := page(r)
	out, e := s.Engine.Store.Records(r.Context(), r.PathValue("id"), p, n)
	if internal(w, e) {
		return
	}
	write(w, 200, out)
}
func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	p, n := page(r)
	out, e := s.Engine.Store.Logs(r.Context(), r.PathValue("id"), r.URL.Query().Get("stage"), p, n)
	if internal(w, e) {
		return
	}
	write(w, 200, out)
}
func (s *Server) deliveries(w http.ResponseWriter, r *http.Request) {
	p, n := page(r)
	out, e := s.Engine.Store.Deliveries(r.Context(), r.URL.Query().Get("monitorId"), r.URL.Query().Get("status"), p, n)
	if internal(w, e) {
		return
	}
	write(w, 200, out)
}
func (s *Server) retry(w http.ResponseWriter, r *http.Request) {
	d, e := s.Engine.Store.Delivery(r.Context(), r.PathValue("id"))
	if e != nil {
		failure(w, 404, "投递记录不存在")
		return
	}
	c := s.Engine.Config()
	m, ok := c.FindMonitor(d.MonitorID)
	if !ok || !m.Enabled || m.Generation != d.Generation || len(c.Targets([]string{d.Target})) == 0 {
		failure(w, 400, "关联监控或渠道已变更，请检查配置后等待新的提醒")
		return
	}
	if e = s.Engine.RetryDelivery(r.Context(), d.ID); e != nil {
		failure(w, 400, e.Error())
		return
	}
	write(w, 200, map[string]bool{"ok": true})
}
func (s *Server) clearRecords(w http.ResponseWriter, r *http.Request) {
	if internal(w, s.Engine.Store.ClearRecords(r.Context(), r.PathValue("id"))) {
		return
	}
	write(w, 200, map[string]bool{"ok": true})
}

func csvSafe(v string) string {
	if strings.HasPrefix(v, "=") || strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") || strings.HasPrefix(v, "@") || strings.HasPrefix(v, "\t") || strings.HasPrefix(v, "\r") || strings.HasPrefix(v, "\n") {
		return "'" + v
	}
	return v
}
func (s *Server) csv(w http.ResponseWriter, r *http.Request) {
	loc, _ := time.LoadLocation(s.Engine.Config().Settings.TimeZone)
	rows, e := s.Engine.Store.RecordsAfter(r.Context(), r.PathValue("id"), 0)
	if internal(w, e) {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="tingfeng-records.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"采集时间", "来源时间", "采集值", "提取数据 JSON", "字段快照 JSON"})
	count := 0
	for len(rows) > 0 && count < 100000 {
		for _, row := range rows {
			if count >= 100000 {
				break
			}
			source := ""
			if row.SourceAt > 0 {
				source = time.UnixMilli(row.SourceAt).In(loc).Format(time.RFC3339)
			}
			fields, _ := json.Marshal(row.Fields)
			display := row.Display
			if len(row.Fields) > 0 {
				lines := []string{}
				for _, f := range row.Fields {
					lines = append(lines, f.Label+"："+f.Display+f.Unit)
				}
				display = strings.Join(lines, "\n")
			}
			if writer.Write([]string{time.UnixMilli(row.At).In(loc).Format(time.RFC3339), source, csvSafe(display), csvSafe(string(row.Value)), csvSafe(string(fields))}) != nil {
				return
			}
			count++
		}
		writer.Flush()
		if writer.Error() != nil {
			return
		}
		rows, e = s.Engine.Store.RecordsAfter(r.Context(), r.PathValue("id"), rows[len(rows)-1].ID)
		if e != nil {
			panic(http.ErrAbortHandler)
		}
	}
	writer.Flush()
}
func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	c := s.Engine.Config()
	write(w, 200, map[string]any{"heapBytes": stats.HeapAlloc, "goroutines": runtime.NumGoroutine(), "databaseBytes": s.Engine.Store.DiskBytes(), "diskLimitMB": c.Settings.DiskLimitMB, "storagePressure": !s.Engine.Store.HasRoom(c.Settings.DiskLimitMB), "lastCleanup": s.Engine.Store.LastCleanup(r.Context()), "uptimeSeconds": int(time.Since(s.Started).Seconds()), "timeZone": c.Settings.TimeZone, "workers": s.Config.Workers, "allowPrivate": s.Engine.Sender.Network.AllowsPrivate(), "maxResponseBytes": s.Config.MaxResponse})
}
func (s *Server) cleanup(w http.ResponseWriter, r *http.Request) {
	out, e := s.Engine.Store.Cleanup(r.Context(), s.Engine.Config())
	if internal(w, e) {
		return
	}
	write(w, 200, out)
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if strings.HasPrefix(name, "api/") {
		failure(w, 404, "接口不存在")
		return
	}
	if s.Assets == nil {
		http.Error(w, "前端尚未构建，请运行 make build", 503)
		return
	}
	f, e := s.Assets.Open(name)
	if e != nil {
		if strings.Contains(path.Base(name), ".") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		f, e = s.Assets.Open(name)
	}
	if e != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	data, e := io.ReadAll(f)
	if e != nil {
		http.Error(w, "资源读取失败", 500)
		return
	}
	http.ServeContent(w, r, name, st.ModTime(), strings.NewReader(string(data)))
}

func (s *Server) messagePreview(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Name        string              `json:"name"`
		Source      domain.Source       `json:"source"`
		Rule        domain.Rule         `json:"rule"`
		Observation *domain.Observation `json:"observation"`
	}
	if !decode(w, r, &v) {
		return
	}
	if err := domain.ValidateRule(v.Rule); err != nil {
		failure(w, 400, err.Error())
		return
	}
	if err := domain.ValidateFields(v.Source); err != nil {
		failure(w, 400, err.Error())
		return
	}
	if err := domain.ValidateRuleField(v.Rule, v.Source); err != nil {
		failure(w, 400, err.Error())
		return
	}
	if len(v.Name) > 400 {
		failure(w, 400, "监控名称过长")
		return
	}
	observation := domain.Observation{Display: "最近一次成功采集的结果", CheckedAt: time.Now().UnixMilli()}
	if v.Observation != nil {
		observation = *v.Observation
	}
	serialized, _ := json.Marshal(observation)
	if len(serialized) > 128<<10 || len(observation.Fields) > 16 || len(observation.Display) > 32768 || len(observation.Items) > 500 {
		failure(w, 400, "预览数据过大")
		return
	}
	if v.Observation == nil {
		for _, f := range v.Source.Fields {
			observation.Fields = append(observation.Fields, domain.FieldValue{Key: f.Key, Label: f.Label, Unit: f.Unit, Display: "待测试", Raw: json.RawMessage("null"), Value: json.RawMessage("null")})
		}
	}
	c := s.Engine.Config()
	loc, _ := time.LoadLocation(c.Settings.TimeZone)
	previous := "上一次采集值"
	if v.Rule.Trigger == "threshold" {
		previous = strconv.FormatFloat(v.Rule.Threshold, 'f', -1, 64)
	}
	payload, e := notify.Payload(domain.Monitor{Name: v.Name, Source: v.Source}, v.Rule, observation, previous, observation.Items, loc)
	if e != nil {
		failure(w, 400, e.Error())
		return
	}
	write(w, 200, payload)
}
