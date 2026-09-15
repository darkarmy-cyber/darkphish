package models

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/gophish/gomail"
)

// boundedSMTPSender limits entire SMTP operations, including greeting, TLS,
// authentication and DATA acknowledgement. A peer cannot keep an operation
// alive indefinitely by trickling individual bytes.
type boundedSMTPSender struct {
	conn    net.Conn
	client  *smtp.Client
	timeout time.Duration
}

func newBoundedSMTPSender(conn net.Conn, d *gomail.Dialer, timeout time.Duration) (_ *boundedSMTPSender, err error) {
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	if err = conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	tlsConfig := d.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: d.Host}
	}
	if d.SSL {
		conn = tls.Client(conn, tlsConfig)
	}
	c, err := smtp.NewClient(conn, d.Host)
	if err != nil {
		return nil, err
	}
	if d.LocalName != "" {
		if err = c.Hello(d.LocalName); err != nil {
			return nil, err
		}
	}
	if !d.SSL {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err = c.StartTLS(tlsConfig); err != nil {
				return nil, err
			}
		}
	}
	auth := d.Auth
	if auth == nil && d.Username != "" {
		if ok, mechanisms := c.Extension("AUTH"); ok {
			switch {
			case strings.Contains(mechanisms, "CRAM-MD5"):
				auth = smtp.CRAMMD5Auth(d.Username, d.Password)
			case strings.Contains(mechanisms, "LOGIN") && !strings.Contains(mechanisms, "PLAIN"):
				auth = &smtpLoginAuth{username: d.Username, password: d.Password, host: d.Host}
			default:
				auth = smtp.PlainAuth("", d.Username, d.Password, d.Host)
			}
		}
	}
	if auth != nil {
		if err = c.Auth(auth); err != nil {
			return nil, err
		}
	}
	return &boundedSMTPSender{conn: conn, client: c, timeout: timeout}, nil
}

func (s *boundedSMTPSender) Send(from string, to []string, msg io.WriterTo) error {
	if err := s.conn.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return err
	}
	if err := s.client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := s.client.Rcpt(recipient); err != nil {
			return err
		}
	}
	w, err := s.client.Data()
	if err != nil {
		return err
	}
	if _, err = msg.WriteTo(w); err != nil {
		_ = s.conn.Close()
		return err
	}
	return w.Close()
}

func (s *boundedSMTPSender) Reset() error {
	if err := s.conn.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return err
	}
	return s.client.Reset()
}

func (s *boundedSMTPSender) Close() error {
	defer s.conn.Close()
	if err := s.conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return s.client.Quit()
}

type smtpLoginAuth struct {
	username string
	password string
	host     string
}

func (a *smtpLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server.Name != a.host || (!server.TLS && a.host != "localhost" && a.host != "127.0.0.1" && a.host != "::1") {
		return "", nil, errors.New("SMTP LOGIN requires a trusted TLS connection")
	}
	return "LOGIN", nil, nil
}

func (a *smtpLoginAuth) Next(challenge []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(challenge))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	default:
		return nil, errors.New("unexpected SMTP LOGIN challenge")
	}
}
