package httpapi

import (
	"fmt"
	"net/http"
	"time"
	"tingfeng/internal/domain"
)

type backupFile struct {
	Format     string        `json:"format"`
	Version    int           `json:"version"`
	ExportedAt int64         `json:"exportedAt"`
	Config     domain.Config `json:"config"`
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	c := domain.Redacted(s.Engine.Config())
	c.Revision = 0
	for i := range c.Monitors {
		c.Monitors[i].Enabled = false
		c.Monitors[i].Generation = 0
		c.Monitors[i].NeedsReview = true
	}
	w.Header().Set("Content-Disposition", `attachment; filename="tingfeng-config.json"`)
	write(w, 200, backupFile{"tingfeng-backup", 1, time.Now().UnixMilli(), c})
}
func merge[T any](old, update []T, id func(T) string) []T {
	present := map[string]bool{}
	for _, v := range update {
		present[id(v)] = true
	}
	out := []T{}
	for _, v := range old {
		if !present[id(v)] {
			out = append(out, v)
		}
	}
	return append(out, update...)
}
func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Backup   backupFile `json:"backup"`
		Preview  bool       `json:"preview"`
		Revision int64      `json:"revision"`
	}
	if !decode(w, r, &v) {
		return
	}
	if v.Backup.Format != "tingfeng-backup" || v.Backup.Version != 1 {
		failure(w, 400, "不是听风正式版配置备份，或版本不支持")
		return
	}
	c := v.Backup.Config
	for i := range c.Monitors {
		c.Monitors[i].Enabled = false
		c.Monitors[i].NeedsReview = true
		c.Monitors[i].Source.ClearRequestSecrets = true
		c.Monitors[i].Source.Query = nil
		c.Monitors[i].Source.Headers = nil
		c.Monitors[i].Source.Body = ""
	}
	for i := range c.Channels {
		ch := &c.Channels[i]
		ch.Enabled = false
		ch.ClearSecrets = true
		ch.URL = ""
		ch.Secret = ""
		ch.Password = ""
		ch.Headers = nil
		ch.SecretsSet = nil
	}
	if err := domain.Validate(c); err != nil {
		failure(w, 400, fmt.Sprintf("备份校验失败：%s", err))
		return
	}
	old := s.Engine.Config()
	// Importing monitors must not silently change this server's network access policy.
	c.Settings.AllowPrivate = old.Settings.AllowPrivate
	for i, p := range c.Recipients {
		for _, existing := range old.Recipients {
			if existing.ID == p.ID {
				c.Recipients[i].Methods = merge(existing.Methods, p.Methods, func(m domain.Method) string { return m.ID })
				break
			}
		}
	}
	merged := domain.Config{Revision: v.Revision, Settings: c.Settings,
		Monitors:   merge(old.Monitors, c.Monitors, func(v domain.Monitor) string { return v.ID }),
		Channels:   merge(old.Channels, c.Channels, func(v domain.Channel) string { return v.ID }),
		Recipients: merge(old.Recipients, c.Recipients, func(v domain.Recipient) string { return v.ID })}
	if v.Preview {
		write(w, 200, map[string]any{"monitors": len(c.Monitors), "recipients": len(c.Recipients), "channels": len(c.Channels), "message": "同标识的配置将覆盖，其余保留。导入的渠道停用、监控暂停；请补充密钥并测试。"})
		return
	}
	out, e := s.Engine.SaveConfig(r.Context(), merged)
	if e != nil {
		failure(w, 409, e.Error())
		return
	}
	write(w, 200, out)
}
