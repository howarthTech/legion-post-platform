// Package billing talks to the Square API for platform billing: the two
// subscription plans (Website $149/yr, SMS CRM add-on $50/yr), one Square
// Customer per client post, and subscription checkout links the operator
// sends to the post.
//
// Raw REST like the CRM's Twilio client — the surface we use is small enough
// that an SDK isn't worth the dependency.
package billing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// squareVersion pins the API behavior we coded against.
const squareVersion = "2025-01-23"

type Client struct {
	baseURL       string
	token         string
	LocationID    string // public — used by the browser SDK
	ApplicationID string // public — used by the browser SDK
	Env           string // "sandbox" or "production" — selects the SDK CDN
	hc            *http.Client
}

// NewClientFromEnvFile builds a client from a key=value env file (see
// secrets/README.md: SQUARE_ENV, SQUARE_ACCESS_TOKEN, SQUARE_LOCATION_ID).
// Process env vars win over file values so CI can override.
func NewClientFromEnvFile(path string) (*Client, error) {
	vals := map[string]string{}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read secrets file: %w", err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if ok {
				vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	get := func(k string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return vals[k]
	}

	token := get("SQUARE_ACCESS_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("SQUARE_ACCESS_TOKEN not set (file %q or env)", path)
	}
	env := get("SQUARE_ENV")
	base := "https://connect.squareupsandbox.com"
	if env == "production" {
		base = "https://connect.squareup.com"
	}
	return &Client{
		baseURL:       base,
		token:         token,
		LocationID:    get("SQUARE_LOCATION_ID"),
		ApplicationID: get("SQUARE_APPLICATION_ID"),
		Env:           env,
		hc:            &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// do sends a JSON request and decodes the JSON response into out (if non-nil).
// Square error payloads are surfaced with their detail strings.
func (c *Client) do(method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Square-Version", squareVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("square %s %s: HTTP %d: %s", method, path, resp.StatusCode, summarizeErrors(respBody))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

// summarizeErrors pulls the detail strings out of a Square error envelope so
// failures read like sentences, not JSON dumps.
func summarizeErrors(body []byte) string {
	var env struct {
		Errors []struct {
			Category string `json:"category"`
			Code     string `json:"code"`
			Detail   string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err != nil || len(env.Errors) == 0 {
		return string(body)
	}
	var parts []string
	for _, e := range env.Errors {
		parts = append(parts, fmt.Sprintf("%s/%s: %s", e.Category, e.Code, e.Detail))
	}
	return strings.Join(parts, "; ")
}
