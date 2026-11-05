package notify

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"
	"tingfeng/internal/domain"
	"tingfeng/internal/secure"
)

func TestSMTPWithTLSAndSTARTTLS(t *testing.T) {
	certificate := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer certificate.Close()
	roots := x509.NewCertPool()
	roots.AddCert(certificate.Certificate())
	for _, mode := range []string{"tls", "starttls"} {
		t.Run(mode, func(t *testing.T) {
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer listener.Close()
			result := make(chan error, 1)
			received := make(chan string, 1)
			go func() {
				conn, e := listener.Accept()
				if e != nil {
					result <- e
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				encrypted := mode == "tls"
				if encrypted {
					tc := tls.Server(conn, certificate.TLS)
					if e = tc.Handshake(); e != nil {
						result <- e
						return
					}
					conn = tc
				}
				reader := textproto.NewReader(bufio.NewReader(conn))
				fmt.Fprint(conn, "220 fixture ESMTP\r\n")
				for {
					line, e := reader.ReadLine()
					if e != nil {
						result <- e
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"):
						if encrypted {
							fmt.Fprint(conn, "250-fixture\r\n250 AUTH PLAIN\r\n")
						} else {
							fmt.Fprint(conn, "250-fixture\r\n250 STARTTLS\r\n")
						}
					case line == "STARTTLS":
						fmt.Fprint(conn, "220 Start TLS\r\n")
						tc := tls.Server(conn, certificate.TLS)
						if e = tc.Handshake(); e != nil {
							result <- e
							return
						}
						conn = tc
						reader = textproto.NewReader(bufio.NewReader(conn))
						encrypted = true
					case strings.HasPrefix(line, "AUTH"):
						if !encrypted {
							result <- fmt.Errorf("plaintext auth")
							return
						}
						fmt.Fprint(conn, "235 accepted\r\n")
					case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
						fmt.Fprint(conn, "250 accepted\r\n")
					case line == "DATA":
						fmt.Fprint(conn, "354 send data\r\n")
						b, e := io.ReadAll(reader.DotReader())
						if e != nil {
							result <- e
							return
						}
						received <- string(b)
						fmt.Fprint(conn, "250 queued\r\n")
					case line == "QUIT":
						fmt.Fprint(conn, "221 bye\r\n")
						result <- nil
						return
					default:
						result <- fmt.Errorf("unexpected SMTP command")
						return
					}
				}
			}()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			p, _ := strconv.Atoi(port)
			ch := domain.Channel{Name: "fixture", Type: "email", Host: "127.0.0.1", Port: p, TLS: mode, Sender: "sender@example.com", User: "fixture", Password: "test-password"}
			sender := New(secure.Network{AllowPrivate: true, Timeout: 5 * time.Second})
			sender.TLSConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			payload := domain.Payload{Title: "听风测试", Text: "价格 123.45\n第二行"}
			if e = sender.Send(context.Background(), ch, "receiver@example.com", payload, "delivery-fixture"); e != nil {
				t.Fatal(e)
			}
			if e = <-result; e != nil {
				t.Fatal(e)
			}
			message, e := mail.ReadMessage(strings.NewReader(<-received))
			if e != nil {
				t.Fatal(e)
			}
			subject, e := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
			if e != nil || subject != payload.Title {
				t.Fatal("encoded subject mismatch", e)
			}
			b, e := io.ReadAll(quotedprintable.NewReader(message.Body))
			if e != nil || strings.TrimSuffix(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") != payload.Text {
				t.Fatal("body mismatch", e)
			}
			if message.Header.Get("Message-ID") != "<delivery-fixture@tingfeng.local>" {
				t.Fatal("unstable message ID")
			}
		})
	}
}
