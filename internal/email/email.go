// Package email sends transactional mail through an SMTP server (MXroute).
// Because the domain's SPF/DKIM are configured and we authenticate as a real
// mailbox, MXroute signs outgoing mail — so these land in inboxes, not spam.
//
// Config (env): SMTP_HOST, SMTP_PORT (default 465, implicit TLS), SMTP_USER,
// SMTP_PASS, EMAIL_FROM ("Name <addr>"), EMAIL_REPLYTO (optional).
package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

type Sender struct {
	host, port, user, pass string
	from, replyTo          string
}

// FromEnv builds a Sender from environment variables. Returns a Sender that
// reports Enabled()=false when SMTP isn't configured, so callers degrade
// gracefully instead of failing.
func FromEnv() *Sender {
	user := os.Getenv("SMTP_USER")
	from := os.Getenv("EMAIL_FROM")
	if from == "" {
		from = user
	}
	return &Sender{
		host:    os.Getenv("SMTP_HOST"),
		port:    envOr("SMTP_PORT", "465"),
		user:    user,
		pass:    os.Getenv("SMTP_PASS"),
		from:    from,
		replyTo: os.Getenv("EMAIL_REPLYTO"),
	}
}

// Enabled reports whether outgoing mail is configured.
func (s *Sender) Enabled() bool {
	return s.host != "" && s.user != "" && s.pass != ""
}

// Send delivers one HTML email. Blocking; callers typically run it in a
// goroutine so it never delays an HTTP response.
func (s *Sender) Send(to, subject, htmlBody string) error {
	if !s.Enabled() {
		return fmt.Errorf("email not configured")
	}
	fromAddr := extractAddr(s.from)
	msg := s.buildMessage(to, subject, htmlBody)

	addr := net.JoinHostPort(s.host, s.port)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.host})
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()
	if err := c.Auth(smtp.PlainAuth("", s.user, s.pass, s.host)); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := c.Mail(fromAddr); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func (s *Sender) buildMessage(to, subject, htmlBody string) string {
	var h strings.Builder
	h.WriteString("From: " + s.from + "\r\n")
	h.WriteString("To: " + to + "\r\n")
	if s.replyTo != "" {
		h.WriteString("Reply-To: " + s.replyTo + "\r\n")
	}
	h.WriteString("Subject: " + subject + "\r\n")
	h.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	h.WriteString("MIME-Version: 1.0\r\n")
	h.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	h.WriteString("\r\n")
	h.WriteString(htmlBody)
	return h.String()
}

// extractAddr pulls the bare address out of "Name <addr@x>" for the SMTP
// envelope; returns the input unchanged if there's no angle-bracket form.
func extractAddr(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.LastIndex(from, ">"); j > i {
			return from[i+1 : j]
		}
	}
	return strings.TrimSpace(from)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
