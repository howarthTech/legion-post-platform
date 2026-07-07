package billing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// This file drives the self-service embedded checkout: create a Square
// customer, store the card the browser tokenized (Web Payments SDK) as a card
// on file, and start a recurring annual subscription per selected plan.

// Buyer is the post/officer completing checkout.
type Buyer struct {
	PostName    string
	ContactName string
	Email       string
	Phone       string
}

// CheckoutResult reports the outcome of an embedded checkout.
type CheckoutResult struct {
	CustomerID      string   `json:"customerId"`
	CardID          string   `json:"cardId"`
	SubscriptionIDs []string `json:"subscriptionIds"`
	Plans           []string `json:"plans"`
}

// idemKey returns a random idempotency key with the given prefix. Square
// requires ≤45 chars; prefix + 24 hex chars stays within that.
func idemKey(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	k := prefix + hex.EncodeToString(b)
	if len(k) > 45 {
		k = k[:45]
	}
	return k
}

// CreateCustomer creates a Square customer for the buyer and returns its id.
func (c *Client) CreateCustomer(b Buyer) (string, error) {
	var resp struct {
		Customer struct {
			ID string `json:"id"`
		} `json:"customer"`
	}
	body := map[string]any{
		"idempotency_key": idemKey("lpw-cust-"),
		"given_name":      b.ContactName,
		"company_name":    b.PostName,
		"email_address":   b.Email,
		"note":            "Legion Post Websites — " + b.PostName,
	}
	if b.Phone != "" {
		body["phone_number"] = b.Phone
	}
	if err := c.do("POST", "/v2/customers", body, &resp); err != nil {
		return "", fmt.Errorf("create customer: %w", err)
	}
	return resp.Customer.ID, nil
}

// StoreCard turns a Web Payments SDK token (source_id) into a card on file for
// the customer and returns the card id.
func (c *Client) StoreCard(customerID, sourceID string, b Buyer) (string, error) {
	var resp struct {
		Card struct {
			ID string `json:"id"`
		} `json:"card"`
	}
	card := map[string]any{"customer_id": customerID}
	if b.ContactName != "" {
		card["cardholder_name"] = b.ContactName
	}
	body := map[string]any{
		"idempotency_key": idemKey("lpw-card-"),
		"source_id":       sourceID,
		"card":            card,
	}
	if err := c.do("POST", "/v2/cards", body, &resp); err != nil {
		return "", fmt.Errorf("store card: %w", err)
	}
	return resp.Card.ID, nil
}

// StartSubscription begins a recurring subscription for the customer on the
// given plan variation, charged to the stored card. Returns the subscription id.
func (c *Client) StartSubscription(customerID, cardID, planVariationID string) (string, error) {
	if c.LocationID == "" {
		return "", fmt.Errorf("SQUARE_LOCATION_ID not set")
	}
	var resp struct {
		Subscription struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"subscription"`
	}
	body := map[string]any{
		"idempotency_key":   idemKey("lpw-sub-"),
		"location_id":       c.LocationID,
		"customer_id":       customerID,
		"plan_variation_id": planVariationID,
		"card_id":           cardID,
	}
	if err := c.do("POST", "/v2/subscriptions", body, &resp); err != nil {
		return "", fmt.Errorf("start subscription: %w", err)
	}
	return resp.Subscription.ID, nil
}

// Checkout runs the full embedded flow: ensure the plans exist, create the
// customer, store the card, and start a subscription for each requested plan
// key ("website" and optionally "sms"). "website" is required.
func (c *Client) Checkout(sourceID string, b Buyer, planKeys []string) (*CheckoutResult, error) {
	// Resolve plan keys to live variation ids.
	plans, err := c.EnsurePlans()
	if err != nil {
		return nil, err
	}
	byKey := map[string]EnsuredPlan{}
	for _, p := range plans {
		byKey[p.Key] = p
	}
	// Normalize + validate selection.
	want := map[string]bool{}
	for _, k := range planKeys {
		k = strings.TrimSpace(strings.ToLower(k))
		if k == "website" || k == "sms" {
			want[k] = true
		}
	}
	if !want["website"] {
		return nil, fmt.Errorf("the Website plan is required")
	}

	customerID, err := c.CreateCustomer(b)
	if err != nil {
		return nil, err
	}
	cardID, err := c.StoreCard(customerID, sourceID, b)
	if err != nil {
		return nil, err
	}

	res := &CheckoutResult{CustomerID: customerID, CardID: cardID}
	// Deterministic order: website first, then sms.
	for _, k := range []string{"website", "sms"} {
		if !want[k] {
			continue
		}
		p, ok := byKey[k]
		if !ok || p.VariationID == "" {
			return res, fmt.Errorf("plan %q not available in Square catalog", k)
		}
		subID, err := c.StartSubscription(customerID, cardID, p.VariationID)
		if err != nil {
			// Partial success: the customer/card exist and earlier subs may
			// have started. Surface the error with what did succeed.
			return res, fmt.Errorf("subscription for %q: %w", k, err)
		}
		res.SubscriptionIDs = append(res.SubscriptionIDs, subID)
		res.Plans = append(res.Plans, k)
	}
	return res, nil
}
