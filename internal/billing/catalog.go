package billing

import (
	"fmt"
)

// The two platform offerings. Prices decided 2026-07-06; annual cadence
// because posts budget annually (and it matches how Legionsites bills).
var Plans = []PlanDef{
	{Key: "website", Name: "Legion Post Website", PriceCents: 14900},
	{Key: "sms", Name: "Legion Post SMS CRM Add-on", PriceCents: 5000},
}

type PlanDef struct {
	Key        string // stable slug used in CLI flags
	Name       string // catalog display name (also the idempotent identity)
	PriceCents int64  // annual price, USD cents
}

// EnsuredPlan is a PlanDef resolved to live catalog IDs.
type EnsuredPlan struct {
	PlanDef
	PlanID      string
	VariationID string
}

type catalogObject struct {
	Type                 string          `json:"type"`
	ID                   string          `json:"id"`
	SubscriptionPlanData *subPlanData    `json:"subscription_plan_data,omitempty"`
	SubscriptionVarData  *subPlanVarData `json:"subscription_plan_variation_data,omitempty"`
}

type subPlanData struct {
	Name string `json:"name"`
	// Populated by Square on reads; on SUBSCRIPTION_PLAN upserts we leave it
	// empty and create variations as separate catalog objects.
	SubscriptionPlanVariations []catalogObject `json:"subscription_plan_variations,omitempty"`
}

type subPlanVarData struct {
	Name               string  `json:"name"`
	SubscriptionPlanID string  `json:"subscription_plan_id,omitempty"`
	Phases             []phase `json:"phases,omitempty"`
}

type phase struct {
	Cadence string   `json:"cadence"` // ANNUAL
	Ordinal int      `json:"ordinal"`
	Pricing *pricing `json:"pricing,omitempty"`
}

type pricing struct {
	Type       string `json:"type"` // STATIC
	PriceMoney *money `json:"price_money,omitempty"`
}

type money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// EnsurePlans makes sure both subscription plans (and one annual variation
// each) exist in the Square catalog, creating whatever is missing. Identity
// is the plan display name — safe to run any number of times.
func (c *Client) EnsurePlans() ([]EnsuredPlan, error) {
	existing, err := c.listSubscriptionPlans()
	if err != nil {
		return nil, err
	}

	var out []EnsuredPlan
	for _, def := range Plans {
		ep := EnsuredPlan{PlanDef: def}

		if found, ok := existing[def.Name]; ok {
			ep.PlanID = found.ID
			if len(found.SubscriptionPlanData.SubscriptionPlanVariations) > 0 {
				ep.VariationID = found.SubscriptionPlanData.SubscriptionPlanVariations[0].ID
			}
		} else {
			var resp struct {
				CatalogObject catalogObject `json:"catalog_object"`
			}
			err := c.do("POST", "/v2/catalog/object", map[string]any{
				"idempotency_key": fmt.Sprintf("legion-plan-%s-v1", def.Key),
				"object": catalogObject{
					Type:                 "SUBSCRIPTION_PLAN",
					ID:                   "#" + def.Key,
					SubscriptionPlanData: &subPlanData{Name: def.Name},
				},
			}, &resp)
			if err != nil {
				return nil, fmt.Errorf("create plan %q: %w", def.Name, err)
			}
			ep.PlanID = resp.CatalogObject.ID
		}

		if ep.VariationID == "" {
			var resp struct {
				CatalogObject catalogObject `json:"catalog_object"`
			}
			err := c.do("POST", "/v2/catalog/object", map[string]any{
				"idempotency_key": fmt.Sprintf("legion-planvar-%s-v1", def.Key),
				"object": catalogObject{
					Type: "SUBSCRIPTION_PLAN_VARIATION",
					ID:   "#" + def.Key + "-annual",
					SubscriptionVarData: &subPlanVarData{
						Name:               def.Name + " — Annual",
						SubscriptionPlanID: ep.PlanID,
						Phases: []phase{{
							Cadence: "ANNUAL",
							Ordinal: 0,
							Pricing: &pricing{
								Type:       "STATIC",
								PriceMoney: &money{Amount: def.PriceCents, Currency: "USD"},
							},
						}},
					},
				},
			}, &resp)
			if err != nil {
				return nil, fmt.Errorf("create variation for %q: %w", def.Name, err)
			}
			ep.VariationID = resp.CatalogObject.ID
		}

		out = append(out, ep)
	}
	return out, nil
}

// listSubscriptionPlans returns existing plans keyed by display name.
func (c *Client) listSubscriptionPlans() (map[string]catalogObject, error) {
	plans := map[string]catalogObject{}
	cursor := ""
	for {
		path := "/v2/catalog/list?types=SUBSCRIPTION_PLAN"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		var resp struct {
			Objects []catalogObject `json:"objects"`
			Cursor  string          `json:"cursor"`
		}
		if err := c.do("GET", path, nil, &resp); err != nil {
			return nil, fmt.Errorf("list plans: %w", err)
		}
		for _, o := range resp.Objects {
			if o.SubscriptionPlanData != nil {
				plans[o.SubscriptionPlanData.Name] = o
			}
		}
		if resp.Cursor == "" {
			return plans, nil
		}
		cursor = resp.Cursor
	}
}
