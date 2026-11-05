package secure

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var ErrPrivate = errors.New("目标属于内网、回环或保留地址；内部来源需在服务器显式启用 TINGFENG_ALLOW_PRIVATE")
var blocked = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32")}

func PublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, p := range blocked {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

type Network struct {
	AllowPrivate bool
	Timeout      time.Duration
}

func (n Network) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, fmt.Errorf("目标地址无效")
	}
	ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if e != nil {
		return nil, fmt.Errorf("DNS 解析失败")
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("DNS 没有返回地址")
	}
	for _, ip := range ips {
		if !n.AllowPrivate && !PublicAddress(ip) {
			return nil, ErrPrivate
		}
	}
	d := net.Dialer{Timeout: n.Timeout, KeepAlive: 30 * time.Second}
	var conn net.Conn
	for _, ip := range ips {
		conn, e = d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("无法连接目标主机")
}
func URL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("目标 URL 无效")
	}
	return nil
}
func (n Network) Client() *http.Client {
	tr := &http.Transport{DialContext: n.DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, MaxIdleConns: 8, MaxIdleConnsPerHost: 2, IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: n.Timeout, ResponseHeaderTimeout: n.Timeout, DisableCompression: false}
	return &http.Client{Timeout: n.Timeout, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("重定向次数过多")
		}
		if e := URL(req.URL.String()); e != nil {
			return e
		}
		if len(via) > 0 && (req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme) {
			req.Header = make(http.Header)
			req.Header.Set("User-Agent", "Tingfeng/1.0")
		}
		return nil
	}}
}
func SafeError(err error) string {
	if errors.Is(err, ErrPrivate) {
		return ErrPrivate.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "请求超时"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "请求超时"
	}
	if errors.Is(err, context.Canceled) {
		return "操作已取消"
	}
	s := err.Error()
	for _, known := range []string{"DNS 解析失败", "无法连接目标主机", "重定向次数过多", "certificate", "TLS", "connection refused"} {
		if strings.Contains(s, known) {
			if known == "certificate" || known == "TLS" {
				return "TLS 证书或握手失败"
			}
			if known == "connection refused" {
				return "目标拒绝连接"
			}
			return known
		}
	}
	return "请求失败，请检查地址、网络及认证配置"
}
