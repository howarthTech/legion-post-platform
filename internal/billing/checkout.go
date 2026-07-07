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
	Rank        string // optional rank/title, e.g. "Commander"
	Email       string
	Phone       string
}

// WhoDisplay returns the contact with rank prefixed when present
// ("Commander Hollis").
func (b Buyer) WhoDisplay() string {
	if b.Rank != "" {
		return b.Rank + " " + b.ContactName
	}
	return b.ContactName
}

// PriceForPlans sums the annual price (cents) of the selected plan keys.
func PriceForPlans(planKeys []string) int64 {
	want := map[string]bool{}
	for _, k := range planKeys {
		want[strings.TrimSpace(strings.ToLower(k))] = true
	}
	var total int64
	for _, p := range Plans {
		if want[p.Key] {
			total += p.PriceCents
		}
	}
	return total
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
		"note":            "Legion Post Websites — " + b.PostName + " (" + b.WhoDisplay() + ")",
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

// Checkout runs the full embedded flow: create the customer, store the card,
// and start ONE subscription on the tier that matches the selection — so the
// post is billed a single combined charge (Website, or Website + SMS), not a
// separate charge per add-on. "website" is required.
func (c *Client) Checkout(sourceID string, b Buyer, planKeys []string) (*CheckoutResult, error) {
	// Validate selection: website is required.
	hasWebsite := false
	for _, k := range planKeys {
		if strings.EqualFold(strings.TrimSpace(k), "website") {
			hasWebsite = true
		}
	}
	if !hasWebsite {
		return nil, fmt.Errorf("the Website plan is required")
	}
	tier := TierForSelection(planKeys)
	if tier == nil {
		return nil, fmt.Errorf("no billing tier matches the selection")
	}

	// Resolve the tier to a live Square variation id.
	tiers, err := c.EnsureTiers()
	if err != nil {
		return nil, err
	}
	var variationID string
	for _, t := range tiers {
		if t.Key == tier.Key {
			variationID = t.VariationID
		}
	}
	if variationID == "" {
		return nil, fmt.Errorf("tier %q not available in Square catalog", tier.Key)
	}

	customerID, err := c.CreateCustomer(b)
	if err != nil {
		return nil, err
	}
	cardID, err := c.StoreCard(customerID, sourceID, b)
	if err != nil {
		return nil, err
	}
	subID, err := c.StartSubscription(customerID, cardID, variationID)
	if err != nil {
		// The customer + card exist; the single subscription charge failed.
		return &CheckoutResult{CustomerID: customerID, CardID: cardID},
			fmt.Errorf("subscription: %w", err)
	}

	return &CheckoutResult{
		CustomerID:      customerID,
		CardID:          cardID,
		SubscriptionIDs: []string{subID},
		Plans:           tier.Includes, // the itemized products this charge covers
	}, nil
}
