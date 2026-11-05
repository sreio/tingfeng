package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
)

func (s *Sender) email(ctx context.Context, c domain.Channel, address string, p domain.Payload, id string) error {
	to, e := mail.ParseAddress(address)
	if e != nil || strings.ContainsAny(address, "\r\n") {
		return fmt.Errorf("接收邮箱无效")
	}
	from, e := mail.ParseAddress(c.Sender)
	if e != nil {
		return fmt.Errorf("发件邮箱无效")
	}
	conn, e := s.Network.DialContext(ctx, "tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	if e != nil {
		return fmt.Errorf("%s", secure.SafeError(e))
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(s.Network.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.Host}
	if s.TLSConfig != nil {
		tlsConfig = s.TLSConfig.Clone()
		tlsConfig.ServerName = c.Host
	}
	if c.TLS == "tls" {
		t := tls.Client(conn, tlsConfig)
		if e = t.HandshakeContext(ctx); e != nil {
			return fmt.Errorf("SMTP TLS 连接失败")
		}
		conn = t
	}
	client, e := smtp.NewClient(conn, c.Host)
	if e != nil {
		return fmt.Errorf("SMTP 握手失败")
	}
	defer client.Close()
	if c.TLS == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP 服务不支持 STARTTLS，拒绝明文认证")
		}
		if e = client.StartTLS(tlsConfig); e != nil {
			return fmt.Errorf("SMTP STARTTLS 失败")
		}
	}
	if c.User != "" {
		if e = client.Auth(smtp.PlainAuth("", c.User, c.Password, c.Host)); e != nil {
			return fmt.Errorf("SMTP 认证失败，请检查账号与授权码")
		}
	}
	if e = client.Mail(from.Address); e != nil {
		return fmt.Errorf("SMTP 拒绝发件地址")
	}
	if e = client.Rcpt(to.Address); e != nil {
		return fmt.Errorf("SMTP 拒绝接收地址")
	}
	writer, e := client.Data()
	if e != nil {
		return fmt.Errorf("SMTP 不接受邮件内容")
	}
	subject := strings.ReplaceAll(strings.ReplaceAll(p.Title, "\r", " "), "\n", " ")
	headers := "From: " + from.String() + "\r\nTo: " + to.String() + "\r\nSubject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\nDate: " + time.Now().Format(time.RFC1123Z) + "\r\nMessage-ID: <" + id + "@tingfeng.local>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n"
	if _, e = writer.Write([]byte(headers)); e != nil {
		return fmt.Errorf("SMTP 写入失败")
	}
	qp := quotedprintable.NewWriter(writer)
	if _, e = qp.Write([]byte(p.Text)); e != nil {
		return fmt.Errorf("SMTP 写入失败")
	}
	if e = qp.Close(); e != nil {
		return fmt.Errorf("SMTP 写入失败")
	}
	if e = writer.Close(); e != nil {
		return fmt.Errorf("SMTP 未确认接收邮件")
	}
	_ = client.Quit()
	return nil
}
