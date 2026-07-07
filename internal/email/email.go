// Package email sends transactional mail through Resend's HTTP API. The
// sending domain must be verified in Resend (SPF/DKIM records) so mail is
// authenticated and lands in inboxes.
//
// Config (env): RESEND_API_KEY, EMAIL_FROM ("Name <addr@domain>"),
// EMAIL_REPLYTO (optional). Degrades gracefully when unset.
package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const resendEndpoint = "https://api.resend.com/emails"

type Sender struct {
	apiKey  string
	from    string
	replyTo string
	hc      *http.Client
}

// FromEnv builds a Sender from environment variables. Enabled()=false when
// unset, so callers degrade gracefully instead of failing.
func FromEnv() *Sender {
	return &Sender{
		apiKey:  os.Getenv("RESEND_API_KEY"),
		from:    os.Getenv("EMAIL_FROM"),
		replyTo: os.Getenv("EMAIL_REPLYTO"),
		hc:      &http.Client{Timeout: 20 * time.Second},
	}
}

// Enabled reports whether outgoing mail is configured.
func (s *Sender) Enabled() bool {
	return s.apiKey != "" && s.from != ""
}

// Send delivers one HTML email via Resend. Blocking; callers typically run it
// in a goroutine so it never delays an HTTP response.
func (s *Sender) Send(to, subject, htmlBody string) error {
	if !s.Enabled() {
		return fmt.Errorf("email not configured")
	}
	payload := map[string]any{
		"from":    s.from,
		"to":      []string{to},
		"subject": subject,
		"html":    htmlBody,
	}
	if s.replyTo != "" {
		payload["reply_to"] = s.replyTo
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, resendEndpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("resend request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("resend HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
