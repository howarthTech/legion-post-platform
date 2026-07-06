package billing

import (
	"fmt"

	"github.com/howarthTech/legion-post-platform/internal/spec"
)

// CheckoutLink is what the operator sends the post: pay this link once and
// the annual subscription starts with the card kept on file.
type CheckoutLink struct {
	Plan EnsuredPlan
	URL  string
}

// EnsureCustomer finds the Square customer for this client (by reference_id =
// client slug) or creates one from the spec.
func (c *Client) EnsureCustomer(s *spec.Spec) (string, error) {
	var search struct {
		Customers []struct {
			ID string `json:"id"`
		} `json:"customers"`
	}
	err := c.do("POST", "/v2/customers/search", map[string]any{
		"query": map[string]any{
			"filter": map[string]any{
				"reference_id": map[string]any{"exact": s.Client},
			},
		},
	}, &search)
	if err != nil {
		return "", fmt.Errorf("search customer: %w", err)
	}
	if len(search.Customers) > 0 {
		return search.Customers[0].ID, nil
	}

	var created struct {
		Customer struct {
			ID string `json:"id"`
		} `json:"customer"`
	}
	err = c.do("POST", "/v2/customers", map[string]any{
		"idempotency_key": "legion-customer-" + s.Client,
		"company_name":    s.PostName,
		"email_address":   s.Email,
		"phone_number":    s.Phone,
		"reference_id":    s.Client,
		"note":            "Legion Post Platform client — provisioned from clients/" + s.Client + ".yaml",
	}, &created)
	if err != nil {
		return "", fmt.Errorf("create customer: %w", err)
	}
	return created.Customer.ID, nil
}

// CreateCheckoutLink makes a Square-hosted payment link that starts the given
// subscription plan variation when the post pays it.
func (c *Client) CreateCheckoutLink(s *spec.Spec, plan EnsuredPlan) (CheckoutLink, error) {
	if c.LocationID == "" {
		return CheckoutLink{}, fmt.Errorf("SQUARE_LOCATION_ID not set — required for payment links")
	}
	var resp struct {
		PaymentLink struct {
			URL string `json:"url"`
		} `json:"payment_link"`
	}
	err := c.do("POST", "/v2/online-checkout/payment-links", map[string]any{
		"idempotency_key": fmt.Sprintf("legion-link-%s-%s-v1", s.Client, plan.Key),
		"description":     fmt.Sprintf("%s — %s (annual)", s.PostName, plan.Name),
		"quick_pay": map[string]any{
			"name":        plan.Name,
			"location_id": c.LocationID,
			"price_money": map[string]any{"amount": plan.PriceCents, "currency": "USD"},
		},
		"checkout_options": map[string]any{
			// The plan VARIATION id per Square-Version >= 2023-11-15.
			"subscription_plan_id": plan.VariationID,
			"ask_for_shipping_address": false,
		},
		"pre_populated_data": map[string]any{
			"buyer_email": s.Email,
		},
	}, &resp)
	if err != nil {
		return CheckoutLink{}, fmt.Errorf("create payment link (%s): %w", plan.Key, err)
	}
	return CheckoutLink{Plan: plan, URL: resp.PaymentLink.URL}, nil
}

// Subscription is a row in the status report.
type Subscription struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	PlanVarID string `json:"plan_variation_id"`
	Customer  string `json:"customer_id"`
	StartDate string `json:"start_date"`
	ChargedTo string `json:"charged_through_date"`
}

// ListSubscriptions returns every subscription visible to the account
// (the platform's client base is small; no filtering needed yet).
func (c *Client) ListSubscriptions() ([]Subscription, error) {
	var out []Subscription
	cursor := ""
	for {
		body := map[string]any{}
		if cursor != "" {
			body["cursor"] = cursor
		}
		var resp struct {
			Subscriptions []Subscription `json:"subscriptions"`
			Cursor        string         `json:"cursor"`
		}
		if err := c.do("POST", "/v2/subscriptions/search", body, &resp); err != nil {
			return nil, fmt.Errorf("search subscriptions: %w", err)
		}
		out = append(out, resp.Subscriptions...)
		if resp.Cursor == "" {
			return out, nil
		}
		cursor = resp.Cursor
	}
}
