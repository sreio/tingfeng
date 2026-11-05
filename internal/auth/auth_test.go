package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"tingfeng/internal/storage"
)

func TestPasswordLifecycle(t *testing.T) {
	ctx := context.Background()
	s, e := storage.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	a := New(s)
	if e = a.EnsureBootstrap(ctx); e != nil {
		t.Fatal(e)
	}
	token, e := os.ReadFile(filepath.Join(s.Dir, "setup.token"))
	if e != nil {
		t.Fatal(e)
	}
	if e = a.Setup(ctx, "wrong", "first-password"); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if e = a.Setup(ctx, string(token), "first-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(s.Dir, "setup.token")); !os.IsNotExist(e) {
		t.Fatal("bootstrap token retained")
	}
	if e = a.Setup(ctx, string(token), "new-password"); !errors.Is(e, ErrInitialized) {
		t.Fatal(e)
	}
	if _, e = a.Login(ctx, "bad-password"); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	one, e := a.Login(ctx, "first-password")
	if e != nil {
		t.Fatal(e)
	}
	two, e := a.Login(ctx, "first-password")
	if e != nil {
		t.Fatal(e)
	}
	if e = a.ChangePassword(ctx, "bad-password", "second-password"); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if e = a.ChangePassword(ctx, "first-password", "second-password"); e != nil {
		t.Fatal(e)
	}
	for _, ss := range []Session{one, two} {
		if _, e = a.Session(ctx, ss.Token); !errors.Is(e, ErrDenied) {
			t.Fatal("old session survived password change")
		}
	}
	if _, e = a.Login(ctx, "first-password"); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if _, e = a.Login(ctx, "second-password"); e != nil {
		t.Fatal(e)
	}
	if e = a.Reset(ctx); e != nil {
		t.Fatal(e)
	}
	initialized, _, e := a.Initialized(ctx)
	if e != nil || initialized {
		t.Fatal(initialized, e)
	}
	token, e = os.ReadFile(filepath.Join(s.Dir, "setup.token"))
	if e != nil {
		t.Fatal(e)
	}
	if e = a.Setup(ctx, string(token), "third-password"); e != nil {
		t.Fatal(e)
	}
}
func TestRateLimit(t *testing.T) {
	a := New(nil)
	for i := 0; i < 10; i++ {
		if !a.Allow("ip") {
			t.Fatal(i)
		}
	}
	if a.Allow("ip") {
		t.Fatal("rate limit bypassed")
	}
	a.Succeeded("ip")
	if !a.Allow("ip") {
		t.Fatal("success did not reset")
	}
}
