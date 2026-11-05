package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"
	"tingfeng/internal/auth"
	"tingfeng/internal/config"
	"tingfeng/internal/engine"
	"tingfeng/internal/httpapi"
	"tingfeng/internal/notify"
	"tingfeng/internal/secure"
	"tingfeng/internal/source"
	"tingfeng/internal/storage"
	"tingfeng/web"
)

var version = "dev"

func main() {
	if e := run(); e != nil {
		slog.Error("听风未能完成操作", "reason", e.Error())
		os.Exit(1)
	}
}
func run() error {
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "version":
		fmt.Println("听风 " + version)
		return nil
	case "healthcheck":
		address := cfg.Listen
		if strings.HasPrefix(address, "0.0.0.0:") {
			address = "127.0.0.1:" + strings.TrimPrefix(address, "0.0.0.0:")
		}
		client := http.Client{Timeout: 3 * time.Second}
		resp, e := client.Get("http://" + address + "/healthz")
		if e != nil {
			return fmt.Errorf("健康检查失败")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("健康检查失败")
		}
		return nil
	case "reset-password":
		flags := flag.NewFlagSet(command, flag.ContinueOnError)
		yes := flags.Bool("confirm", false, "确认清除旧登录并重新进入初始化")
		if e = flags.Parse(os.Args[2:]); e != nil {
			return e
		}
		if !*yes {
			return fmt.Errorf("此操作使全部登录失效。确认后使用 reset-password --confirm；监控数据会保留")
		}
		s, e := storage.Open(cfg.DataDir, false)
		if e != nil {
			return e
		}
		defer s.Close()
		if e = auth.New(s).Reset(context.Background()); e != nil {
			return e
		}
		fmt.Println("管理员已重置；请在页面使用数据目录内的 setup.token 设置新密码。监控配置已保留。")
		return nil
	case "backup":
		flags := flag.NewFlagSet(command, flag.ContinueOnError)
		out := flags.String("out", "", "新的备份目录")
		if e = flags.Parse(os.Args[2:]); e != nil {
			return e
		}
		if *out == "" {
			return fmt.Errorf("用法：tingfeng backup --out 新目录")
		}
		if _, e = os.Stat(*out); !os.IsNotExist(e) {
			return fmt.Errorf("备份目录必须不存在")
		}
		if e = os.MkdirAll(*out, 0700); e != nil {
			return e
		}
		s, e := storage.Open(cfg.DataDir, false)
		if e != nil {
			return e
		}
		defer s.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if e = s.BackupDatabase(ctx, filepath.Join(*out, "tingfeng.db")); e != nil {
			return e
		}
		_ = os.Chmod(filepath.Join(*out, "tingfeng.db"), 0600)
		for _, name := range []string{"master.key", "setup.token"} {
			src, e := os.Open(filepath.Join(cfg.DataDir, name))
			if os.IsNotExist(e) && name == "setup.token" {
				continue
			}
			if e != nil {
				return e
			}
			dst, e := os.OpenFile(filepath.Join(*out, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				_ = src.Close()
				return e
			}
			_, copyErr := io.Copy(dst, src)
			_ = src.Close()
			closeErr := dst.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		fmt.Println("完整备份已保存。目录包含加密密钥，请私密保管。")
		return nil
	case "serve":
	default:
		return fmt.Errorf("命令：serve、version、healthcheck、reset-password、backup")
	}
	s, e := storage.Open(cfg.DataDir, true)
	if e != nil {
		return e
	}
	defer s.Close()
	network := secure.Network{AllowPrivate: cfg.AllowPrivate, Timeout: cfg.Timeout}
	eng, e := engine.New(s, source.New(network, cfg.MaxResponse), notify.New(network), cfg.Workers)
	if e != nil {
		return e
	}
	api := httpapi.New(eng, cfg, web.Assets())
	if e = api.Auth.EnsureBootstrap(context.Background()); e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: cfg.Listen, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	eng.Start(ctx)
	done := make(chan error, 1)
	go func() {
		slog.Info("听风已启动", "listen", cfg.Listen, "workers", cfg.Workers)
		done <- server.ListenAndServe()
	}()
	select {
	case e = <-done:
		stop()
		if errors.Is(e, http.ErrServerClosed) {
			e = nil
		}
	case <-ctx.Done():
		e = nil
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		_ = server.Close()
	}
	eng.Wait()
	return e
}
