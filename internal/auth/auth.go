package auth

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"tingfeng/internal/storage"
	"unicode/utf8"
)

var ErrDenied = errors.New("密码或初始化令牌不正确")
var ErrInitialized = errors.New("管理员已初始化，请登录")

type passwordRecord struct {
	Salt      string `json:"salt"`
	Hash      string `json:"hash"`
	UpdatedAt int64  `json:"updatedAt"`
}
type Session struct {
	Token             string `json:"-"`
	CSRF              string `json:"csrf"`
	Expires           int64  `json:"expires"`
	PasswordUpdatedAt int64  `json:"passwordUpdatedAt"`
}
type Service struct {
	Store    *storage.Store
	mu       sync.Mutex
	attempts map[string]attempt
}
type attempt struct {
	Count int
	Since time.Time
}

func New(s *storage.Store) *Service { return &Service{Store: s, attempts: map[string]attempt{}} }
func Random() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func ValidatePassword(s string) error {
	n := utf8.RuneCountInString(s)
	if n < 8 || n > 128 || strings.TrimSpace(s) == "" {
		return fmt.Errorf("密码需为 8～128 个字符")
	}
	return nil
}
func hashPassword(password string) (passwordRecord, error) {
	if e := ValidatePassword(password); e != nil {
		return passwordRecord{}, e
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return passwordRecord{}, e
	}
	hash, e := pbkdf2.Key(sha256.New, password, salt, 210000, 32)
	if e != nil {
		return passwordRecord{}, e
	}
	return passwordRecord{hex.EncodeToString(salt), hex.EncodeToString(hash), time.Now().UnixMilli()}, nil
}
func verify(password string, r passwordRecord) bool {
	if ValidatePassword(password) != nil {
		return false
	}
	salt, e := hex.DecodeString(r.Salt)
	if e != nil || len(salt) != 16 {
		return false
	}
	expected, e := hex.DecodeString(r.Hash)
	if e != nil || len(expected) != 32 {
		return false
	}
	got, e := pbkdf2.Key(sha256.New, password, salt, 210000, 32)
	return e == nil && subtle.ConstantTimeCompare(got, expected) == 1
}
func (a *Service) Initialized(ctx context.Context) (bool, int64, error) {
	raw, e := a.Store.Meta(ctx, "admin")
	if e != nil {
		return false, 0, e
	}
	if len(raw) == 0 {
		return false, 0, nil
	}
	var r passwordRecord
	if e = json.Unmarshal(raw, &r); e != nil {
		return false, 0, fmt.Errorf("管理员数据损坏，请使用命令行重置")
	}
	return true, r.UpdatedAt, nil
}
func (a *Service) EnsureBootstrap(ctx context.Context) error {
	ok, _, e := a.Initialized(ctx)
	if e != nil || ok {
		return e
	}
	path := filepath.Join(a.Store.Dir, "setup.token")
	token, e := os.ReadFile(path)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if len(token) == 0 {
		token = []byte(Random())
		if e = os.WriteFile(path, token, 0600); e != nil {
			return e
		}
	}
	return a.Store.SetMeta(ctx, "bootstrap", []byte(digest(strings.TrimSpace(string(token)))))
}
func (a *Service) Setup(ctx context.Context, token, password string) error {
	record, e := hashPassword(password)
	if e != nil {
		return e
	}
	tx, e := a.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var exists int
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM meta WHERE key='admin'").Scan(&exists); e != nil {
		return e
	}
	if exists != 0 {
		return ErrInitialized
	}
	var expected string
	if e = tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='bootstrap'").Scan(&expected); e != nil {
		return ErrDenied
	}
	if len(token) > 256 || subtle.ConstantTimeCompare([]byte(expected), []byte(digest(token))) != 1 {
		return ErrDenied
	}
	raw, _ := json.Marshal(record)
	if _, e = tx.ExecContext(ctx, "INSERT INTO meta(key,value) VALUES('admin',?)", raw); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM meta WHERE key='bootstrap'"); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	_ = os.Remove(filepath.Join(a.Store.Dir, "setup.token"))
	return nil
}
func (a *Service) Login(ctx context.Context, password string) (Session, error) {
	raw, e := a.Store.Meta(ctx, "admin")
	if e != nil {
		return Session{}, e
	}
	var record passwordRecord
	if json.Unmarshal(raw, &record) != nil || !verify(password, record) {
		return Session{}, ErrDenied
	}
	session := Session{Token: Random(), CSRF: Random(), Expires: time.Now().Add(24 * time.Hour).UnixMilli(), PasswordUpdatedAt: record.UpdatedAt}
	tx, e := a.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return Session{}, e
	}
	defer tx.Rollback()
	var current []byte
	if e = tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key='admin'").Scan(&current); e != nil || subtle.ConstantTimeCompare(raw, current) != 1 {
		return Session{}, ErrDenied
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO sessions(hash,csrf,expires) VALUES(?,?,?)", digest(session.Token), session.CSRF, session.Expires); e != nil {
		return Session{}, e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires<? OR hash NOT IN (SELECT hash FROM sessions ORDER BY expires DESC LIMIT 20)", time.Now().UnixMilli()); e != nil {
		return Session{}, e
	}
	return session, tx.Commit()
}
func (a *Service) Session(ctx context.Context, token string) (Session, error) {
	out := Session{}
	if len(token) != 64 {
		return out, ErrDenied
	}
	e := a.Store.DB.QueryRowContext(ctx, "SELECT csrf,expires FROM sessions WHERE hash=? AND expires>?", digest(token), time.Now().UnixMilli()).Scan(&out.CSRF, &out.Expires)
	if errors.Is(e, sql.ErrNoRows) {
		return out, ErrDenied
	}
	if e != nil {
		return out, e
	}
	_, out.PasswordUpdatedAt, e = a.Initialized(ctx)
	return out, e
}
func (a *Service) Logout(ctx context.Context, token string) error {
	_, e := a.Store.DB.ExecContext(ctx, "DELETE FROM sessions WHERE hash=?", digest(token))
	return e
}
func (a *Service) ChangePassword(ctx context.Context, old, newPassword string) error {
	raw, e := a.Store.Meta(ctx, "admin")
	if e != nil {
		return e
	}
	var record passwordRecord
	if json.Unmarshal(raw, &record) != nil || !verify(old, record) {
		return ErrDenied
	}
	if old == newPassword {
		return fmt.Errorf("新密码不能与当前密码相同")
	}
	next, e := hashPassword(newPassword)
	if e != nil {
		return e
	}
	b, _ := json.Marshal(next)
	tx, e := a.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(ctx, "UPDATE meta SET value=? WHERE key='admin' AND value=?", b, raw)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("密码已变更，请重新登录")
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM sessions"); e != nil {
		return e
	}
	return tx.Commit()
}
func (a *Service) Reset(ctx context.Context) error {
	tx, e := a.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	token := Random()
	if e = os.WriteFile(filepath.Join(a.Store.Dir, "setup.token"), []byte(token), 0600); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM meta WHERE key IN ('admin','bootstrap')"); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM sessions"); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO meta(key,value) VALUES('bootstrap',?)", digest(token)); e != nil {
		return e
	}
	return tx.Commit()
}
func (a *Service) Allow(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if len(a.attempts) > 2048 {
		for k, v := range a.attempts {
			if now.Sub(v.Since) > 15*time.Minute {
				delete(a.attempts, k)
			}
		}
		if len(a.attempts) > 2048 {
			return false
		}
	}
	v := a.attempts[key]
	if now.Sub(v.Since) > 15*time.Minute {
		v = attempt{Since: now}
	}
	if v.Count >= 10 {
		return false
	}
	v.Count++
	a.attempts[key] = v
	return true
}
func (a *Service) Succeeded(key string) { a.mu.Lock(); delete(a.attempts, key); a.mu.Unlock() }
