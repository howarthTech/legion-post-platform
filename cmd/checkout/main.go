// checkout — the self-service billing backend for Legion Post Websites.
//
// It holds the Square access token (never sent to the browser) and exposes two
// endpoints the static checkout page calls:
//
//	GET  /api/config    -> public Square app id + location + env + plan prices,
//	                       so the browser can render the Web Payments SDK card form.
//	POST /api/checkout  -> { sourceId, postName, contactName, email, phone, plans }
//	                       creates a Square customer, stores the tokenized card,
//	                       and starts the recurring annual subscription(s).
//
// The card is tokenized in the browser (Web Payments SDK) — card data never
// reaches this service (PCI SAQ-A). Runs behind Caddy on a loopback port; the
// checkout page and this API share the legionpostwebsites.com origin, so no
// CORS is needed.
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/howarthTech/legion-post-platform/internal/billing"
)

func main() {
	// Square creds come from the container's env_file (/srv/secrets/square.env).
	client, err := billing.NewClientFromEnvFile("")
	if err != nil {
		log.Fatalf("square config: %v (set SQUARE_* in the env file)", err)
	}
	if client.ApplicationID == "" || client.LocationID == "" {
		log.Fatal("SQUARE_APPLICATION_ID and SQUARE_LOCATION_ID are required (the browser SDK needs them)")
	}
	notifyURL := os.Getenv("NOTIFY_URL") // optional: POST a note here on each sale
	listen := envOr("LISTEN_ADDR", "0.0.0.0:8092")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", configHandler(client))
	mux.HandleFunc("POST /api/checkout", checkoutHandler(client, notifyURL))
	mux.HandleFunc("POST /api/check-signup", checkSignupHandler(client, notifyURL))
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("checkout listening on %s (Square env: %s)", listen, orDefault(client.Env, "sandbox"))
	srv := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      40 * time.Second, // Square calls can take a moment
	}
	log.Fatal(srv.ListenAndServe())
}

func configHandler(c *billing.Client) http.HandlerFunc {
	type plan struct {
		Key        string `json:"key"`
		Name       string `json:"name"`
		PriceCents int64  `json:"priceCents"`
	}
	plans := make([]plan, 0, len(billing.Plans))
	for _, p := range billing.Plans {
		plans = append(plans, plan{Key: p.Key, Name: p.Name, PriceCents: p.PriceCents})
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"applicationId": c.ApplicationID,
			"locationId":    c.LocationID,
			"env":           orDefault(c.Env, "sandbox"),
			"plans":         plans,
		})
	}
}

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func checkoutHandler(c *billing.Client, notifyURL string) http.HandlerFunc {
	type req struct {
		SourceID    string   `json:"sourceId"`
		PostName    string   `json:"postName"`
		ContactName string   `json:"contactName"`
		Rank        string   `json:"rank"`
		Email       string   `json:"email"`
		Phone       string   `json:"phone"`
		Plans       []string `json:"plans"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Could not read the form. Please try again.")
			return
		}
		buyer, msg := parseBuyer(in.PostName, in.ContactName, in.Rank, in.Email, in.Phone)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		if in.SourceID == "" {
			writeErr(w, http.StatusBadRequest, "Card details are missing — please re-enter your card.")
			return
		}

		res, err := c.Checkout(in.SourceID, buyer, in.Plans)
		if err != nil {
			// Log the full error server-side; give the buyer a safe message.
			log.Printf("checkout failed for %q <%s>: %v", buyer.PostName, buyer.Email, err)
			m := "We couldn't complete the payment. Your card was not charged. " +
				"Please check your card details and try again, or contact us."
			if strings.Contains(err.Error(), "Website plan is required") {
				m = "Please keep the Website plan selected — it's required."
			}
			writeErr(w, http.StatusPaymentRequired, m)
			return
		}

		log.Printf("✓ checkout (card): %q <%s> plans=%v customer=%s subs=%v",
			buyer.PostName, buyer.Email, res.Plans, res.CustomerID, res.SubscriptionIDs)
		notify(notifyURL, "card", buyer, res.Plans)

		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plans": res.Plans})
	}
}

// checkSignupHandler records a "pay by check" signup: no card is processed.
// The operator is notified and, after verifying the post, takes the site live
// once the mailed check arrives. Returns the amount to mail.
func checkSignupHandler(c *billing.Client, notifyURL string) http.HandlerFunc {
	type req struct {
		PostName    string   `json:"postName"`
		ContactName string   `json:"contactName"`
		Rank        string   `json:"rank"`
		Email       string   `json:"email"`
		Phone       string   `json:"phone"`
		Plans       []string `json:"plans"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Could not read the form. Please try again.")
			return
		}
		buyer, msg := parseBuyer(in.PostName, in.ContactName, in.Rank, in.Email, in.Phone)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		// Normalize plan selection (website required).
		plans := []string{}
		for _, k := range in.Plans {
			k = strings.ToLower(strings.TrimSpace(k))
			if k == "website" || k == "sms" {
				plans = append(plans, k)
			}
		}
		hasWebsite := false
		for _, k := range plans {
			if k == "website" {
				hasWebsite = true
			}
		}
		if !hasWebsite {
			writeErr(w, http.StatusBadRequest, "Please keep the Website plan selected — it's required.")
			return
		}
		totalCents := billing.PriceForPlans(plans)

		// Best-effort: record the post as a Square customer so there's a
		// ledger entry to attach the payment to when the check arrives.
		if custID, err := c.CreateCustomer(buyer); err != nil {
			log.Printf("check-signup: customer record failed (non-fatal) for %q: %v", buyer.PostName, err)
		} else {
			log.Printf("check-signup customer=%s", custID)
		}

		log.Printf("✓ check-signup: %q <%s> plans=%v total=$%d — awaiting mailed check",
			buyer.PostName, buyer.Email, plans, totalCents/100)
		notify(notifyURL, "check", buyer, plans)

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"plans":      plans,
			"totalCents": totalCents,
		})
	}
}

// parseBuyer validates the shared buyer fields; returns a user-facing message
// (non-empty) on failure.
func parseBuyer(postName, contactName, rank, email, phone string) (billing.Buyer, string) {
	b := billing.Buyer{
		PostName:    strings.TrimSpace(postName),
		ContactName: strings.TrimSpace(contactName),
		Rank:        strings.TrimSpace(rank),
		Email:       strings.TrimSpace(email),
		Phone:       strings.TrimSpace(phone),
	}
	if b.PostName == "" || b.ContactName == "" {
		return b, "Please enter the post name and your name."
	}
	if !emailRE.MatchString(b.Email) {
		return b, "Please enter a valid email address."
	}
	return b, ""
}

// notify fires a best-effort POST so the operator learns a post has signed up
// (by card or check) and the site can be taken live. No-op when NOTIFY_URL is
// unset. method is "card" or "check".
func notify(url, method string, b billing.Buyer, plans []string) {
	if url == "" {
		return
	}
	subject := "LPW — new paid subscription (card): " + b.PostName
	if method == "check" {
		subject = "LPW — new signup (pay by CHECK, awaiting payment): " + b.PostName
	}
	go func() {
		contact := b.ContactName
		if b.Rank != "" {
			contact = b.Rank + " " + b.ContactName
		}
		payload, _ := json.Marshal(map[string]any{
			"_subject": subject,
			"method":   method,
			"post":     b.PostName,
			"contact":  contact,
			"email":    b.Email,
			"phone":    b.Phone,
			"plans":    strings.Join(plans, ", "),
		})
		hc := &http.Client{Timeout: 15 * time.Second}
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if resp, err := hc.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
