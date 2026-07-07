package billing

import "fmt"

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
