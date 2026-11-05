package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	DataDir, Listen, PublicURL string
	AllowPrivate               bool
	Workers                    int
	Timeout                    time.Duration
	MaxResponse                int64
	CookieSecure               bool
}

func Load() (Config, error) {
	c := Config{DataDir: env("TINGFENG_DATA_DIR", "./data"), Listen: env("TINGFENG_LISTEN", "127.0.0.1:8080"), PublicURL: os.Getenv("TINGFENG_PUBLIC_URL"), Workers: 1, Timeout: 15 * time.Second, MaxResponse: 2 << 20}
	c.AllowPrivate = os.Getenv("TINGFENG_ALLOW_PRIVATE") == "true"
	c.CookieSecure = os.Getenv("TINGFENG_COOKIE_SECURE") == "true"
	if s := os.Getenv("TINGFENG_WORKERS"); s != "" {
		v, e := strconv.Atoi(s)
		if e != nil || v < 1 || v > 4 {
			return c, fmt.Errorf("TINGFENG_WORKERS 必须为 1～4")
		}
		c.Workers = v
	}
	var err error
	c.DataDir, err = filepath.Abs(c.DataDir)
	return c, err
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
